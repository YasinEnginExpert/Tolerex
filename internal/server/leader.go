package server

import (
	"bufio"
	"context"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"tolerex/internal/config"
	"tolerex/internal/storage"
	pb "tolerex/proto/gen"

	"google.golang.org/grpc"
)

type MemberInfo struct {
	Address    string
	AddedAt    time.Time
	MessageCnt int
}

type LeaderServer struct {
	pb.UnimplementedStorageServiceServer
	Members   []string
	Tolerance int
	MsgMap    map[int][]string
	MemberLog map[string]*MemberInfo
}

func NewLeaderServer(members []string, toleranceFile string) (*LeaderServer, error) {
	tolerance, err := config.ReadTolerance(toleranceFile)
	if err != nil {
		return nil, err
	}

	memberLog := make(map[string]*MemberInfo)
	for _, addr := range members {
		memberLog[addr] = &MemberInfo{
			Address:    addr,
			AddedAt:    time.Now(),
			MessageCnt: 0,
		}
	}

	return &LeaderServer{
		Members:   members,
		Tolerance: tolerance,
		MsgMap:    make(map[int][]string),
		MemberLog: memberLog,
	}, nil
}

func (s *LeaderServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	// 1. Lider kendi diskine yazıyor
	err := storage.WriteMessage(int(msg.Id), msg.Text)
	if err != nil {
		return &pb.StoreResult{Ok: false, Err: "Lider diske yazamadı"}, nil
	}

	// 2. Rastgele tolerance kadar üye seç
	rand.Seed(time.Now().UnixNano())
	shuffled := append([]string{}, s.Members...)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	selected := shuffled[:s.Tolerance]

	var successful []string

	// 3. Üyelere Store çağrısı yap
	for _, addr := range selected {
		conn, err := grpc.Dial(addr, grpc.WithInsecure())
		if err != nil {
			log.Printf("Bağlantı hatası (%s): %v", addr, err)
			continue
		}
		defer conn.Close()

		client := pb.NewStorageServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		res, err := client.Store(ctx, msg)
		if err == nil && res.Ok {
			successful = append(successful, addr)
			s.MemberLog[addr].MessageCnt++ // başarılıya eklenince sayaç artar
		} else {
			log.Printf("Üyeye gönderilemedi (%s): %v", addr, err)
		}
	}

	// 4. Hepsi başarılı mı?
	if len(successful) == s.Tolerance {
		s.MsgMap[int(msg.Id)] = successful
		return &pb.StoreResult{Ok: true}, nil
	}

	return &pb.StoreResult{Ok: false, Err: "Üyelere tam yazılamadı"}, nil
}

// İstemciden gelen TCP bağlantılarını işleyen fonksiyon
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
			if res.Ok {
				conn.Write([]byte("OK\n"))
			} else {
				conn.Write([]byte("ERROR: " + res.Err + "\n"))
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

func (s *LeaderServer) PrintMemberStats() {
	log.Println("==== Üye Durum Tablosu ====")
	for _, info := range s.MemberLog {
		log.Printf("Adres: %s | Eklenme: %s | Mesaj Sayısı: %d",
			info.Address,
			info.AddedAt.Format("15:04:05"),
			info.MessageCnt)
	}
	log.Println("============================")
}
