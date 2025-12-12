package server

import (
	"bufio"
	"context"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"tolerex/internal/config"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ---Uyelerin messaj sayısi
type memberStat struct {
	addr  string
	count int
}

// -------- Üyeye ait bilgileri tutan yapı: adres, eklenme zamanı ve aldığı mesaj sayısı
type MemberInfo struct {
	Address    string
	AddedAt    time.Time
	MessageCnt int
	LastSeen   time.Time
	Alive      bool
}

// -------- Lider sunucunun sahip olduğu yapılar
type LeaderServer struct {
	pb.UnimplementedStorageServiceServer
	Members   []string               // Üyelerin adres listesi
	Tolerance int                    // Hata toleransı: her mesaj kaç üyeye yazılacak
	MsgMap    map[int][]string       // Her mesaj hangi üyelere yazıldı
	MemberLog map[string]*MemberInfo // Üyeler hakkında detaylı bilgi
	mu        sync.Mutex
	DataDir   string
}

// Üyenin hayatta olduğunu lidere bildirir
func (s *LeaderServer) Heartbeat(ctx context.Context, hb *pb.HeartbeatRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	member, ok := s.MemberLog[hb.Address]
	if !ok {
		return &emptypb.Empty{}, nil
	}

	member.LastSeen = time.Now()
	member.Alive = true
	return &emptypb.Empty{}, nil
}

// Üyeleri periyodik kontrol eder, timeout olanı Alive=false yapar
func (s *LeaderServer) StartHeartbeatWatcher() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			s.mu.Lock()
			now := time.Now()
			for _, m := range s.MemberLog {
				if m.LastSeen.IsZero() {
					m.LastSeen = m.AddedAt
				}
				if now.Sub(m.LastSeen) > 15*time.Second {
					if m.Alive {
						log.Printf("Üye düştü: %s", m.Address)
					}
					m.Alive = false
				}
			}
			s.mu.Unlock()
		}
	}()
}

// -------- Lider sunucunun oluşturulması, üyeler ve tolerans ayarlarını yükler
func NewLeaderServer(members []string, toleranceFile string) (*LeaderServer, error) {
	log.Printf(">> Tolerans dosyası yolu: '%s'\n", toleranceFile)
	tolerance, err := config.ReadTolerance(toleranceFile)
	if err != nil {
		return nil, err
	}

	memberLog := make(map[string]*MemberInfo)
	now := time.Now()
	for _, addr := range members {
		memberLog[addr] = &MemberInfo{
			Address:    addr,
			AddedAt:    now,
			MessageCnt: 0,
			LastSeen:   now,
			Alive:      true,
		}
	}

	return &LeaderServer{
		Members:   members,
		Tolerance: tolerance,
		MsgMap:    make(map[int][]string),
		MemberLog: memberLog,
	}, nil
}

// ---Yeni Memberların eklenmesi---
func (s *LeaderServer) RegisterMember(ctx context.Context, req *pb.MemberInfo) (*pb.RegisterReply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := req.Address
	if _, exists := s.MemberLog[addr]; exists {
		s.MemberLog[addr].Alive = true
		s.MemberLog[addr].LastSeen = time.Now()
		return &pb.RegisterReply{Ok: true}, nil
	}

	// Yeni uye ekleme
	now := time.Now()
	s.Members = append(s.Members, addr)
	s.MemberLog[addr] = &MemberInfo{
		Address:    addr,
		AddedAt:    now,
		MessageCnt: 0,
		LastSeen:   now,
		Alive:      true,
	}

	log.Printf("Yeni üye eklendi: %s", addr)
	return &pb.RegisterReply{Ok: true}, nil
}

// -------- İstemciden gelen mesajları alır ve üyeler arasında dağıtır
func (s *LeaderServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	// 1) Leader kendi diskine yazar
	if err := storage.WriteMessage(s.DataDir, int(msg.Id), msg.Text); err != nil {
		return &pb.StoreResult{Ok: false, Err: "Lider diske yazamadı"}, nil
	}

	// 2) Alive üyeler arasından en az yükte olanları seç
	var stats []memberStat

	s.mu.Lock()
	for _, addr := range s.Members {
		info := s.MemberLog[addr]
		if info == nil || !info.Alive {
			continue
		}
		stats = append(stats, memberStat{addr: addr, count: info.MessageCnt})
	}
	s.mu.Unlock()

	sort.Slice(stats, func(i, j int) bool { return stats[i].count < stats[j].count })

	var selected []string
	for i := 0; i < s.Tolerance && i < len(stats); i++ {
		selected = append(selected, stats[i].addr)
	}

	// eğer yeterli alive üye yoksa
	if len(selected) < s.Tolerance {
		return &pb.StoreResult{Ok: false, Err: "Yeterli sayıda alive üye yok"}, nil
	}

	// Seçilen üyelere yaz
	var successful []string

	for _, addr := range selected {
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Bağlantı hatası (%s): %v", addr, err)
			continue
		}

		client := pb.NewStorageServiceClient(conn)

		ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		res, callErr := client.Store(ctx2, msg)
		cancel()
		conn.Close()

		if callErr == nil && res != nil && res.Ok {
			successful = append(successful, addr)

			s.mu.Lock()
			if s.MemberLog[addr] != nil {
				s.MemberLog[addr].MessageCnt++
			}
			s.mu.Unlock()
		} else {
			log.Printf("Üyeye gönderilemedi (%s): %v", addr, callErr)
		}
	}

	// tolerance kadar başarılı mı
	if len(successful) == s.Tolerance {
		s.mu.Lock()
		s.MsgMap[int(msg.Id)] = successful
		s.mu.Unlock()
		return &pb.StoreResult{Ok: true}, nil
	}

	return &pb.StoreResult{Ok: false, Err: "Üyelere tam yazılamadı"}, nil
}

// -------- Gelen Get istegine göre sunuculara tek tek bakılır----
func (s *LeaderServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	// Önce leader disk
	text, err := storage.ReadMessage(s.DataDir, int(req.Id))
	if err == nil {
		return &pb.StoredMessage{Id: req.Id, Text: text}, nil
	}

	// Replika listesi
	s.mu.Lock()
	replicas, ok := s.MsgMap[int(req.Id)]
	s.mu.Unlock()

	if !ok {
		return nil, nil
	}

	// sırayla alive olanlara sor
	for _, addr := range replicas {
		s.mu.Lock()
		info := s.MemberLog[addr]
		alive := info != nil && info.Alive
		s.mu.Unlock()

		if !alive {
			continue
		}

		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("GET bağlanamadı (%s): %v", addr, err)
			continue
		}

		client := pb.NewStorageServiceClient(conn)

		ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		res, callErr := client.Retrieve(ctx2, req)
		cancel()
		conn.Close()

		if callErr == nil && res != nil {
			return res, nil
		}
	}

	return nil, nil
}

// -------- İstemciden gelen TCP bağlantılarını işler (SET, GET komutları)
func (s *LeaderServer) HandleClient(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 2 {
			conn.Write([]byte("ERROR: Geçersiz komut\n"))
			continue
		}
		cmd, idStr := strings.ToUpper(parts[0]), parts[1]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			conn.Write([]byte("ERROR: ID sayısal olmalı\n"))
			continue
		}

		switch cmd {
		case "SET":
			if len(parts) != 3 {
				conn.Write([]byte("ERROR: SET için mesaj eksik\n"))
				continue
			}
			msg := &pb.StoredMessage{Id: int32(id), Text: parts[2]}
			res, _ := s.Store(context.Background(), msg)
			if res != nil && res.Ok {
				conn.Write([]byte("OK\n"))
			} else {
				errMsg := "unknown"
				if res != nil {
					errMsg = res.Err
				}
				conn.Write([]byte("ERROR: " + errMsg + "\n"))
			}

		case "GET":
			res, _ := s.Retrieve(context.Background(), &pb.MessageID{Id: int32(id)})
			if res != nil {
				conn.Write([]byte(res.Text + "\n"))
			} else {
				conn.Write([]byte("NOT_FOUND\n"))
			}

		default:
			conn.Write([]byte("ERROR: Bilinmeyen komut\n"))
		}
	}
}

// -------- Periyodik olarak liderin tüm üyeleri ve durumlarını log'lar
func (s *LeaderServer) PrintMemberStats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Print("\033[2J\033[H")

	log.Println("==== Üye Durum Tablosu ====")
	for _, info := range s.MemberLog {
		log.Printf("Adres: %s | Alive: %v | LastSeen: %s | Mesaj Sayısı: %d",
			info.Address,
			info.Alive,
			info.LastSeen.Format("15:04:05"),
			info.MessageCnt,
		)
	}
	log.Println("============================")
}
