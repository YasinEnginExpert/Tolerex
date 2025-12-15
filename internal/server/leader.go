package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"tolerex/internal/config"
	"tolerex/internal/logger"
	pb "tolerex/proto/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// --- Üyelerin mesaj sayısı için yardımcı struct
type memberStat struct {
	addr  string
	count int
}

// -------- Üyeye ait bilgiler
type MemberInfo struct {
	Address    string
	AddedAt    time.Time
	MessageCnt int
	LastSeen   time.Time
	Alive      bool
}

// -------- Leader Server
type LeaderServer struct {
	pb.UnimplementedStorageServiceServer
	Members   []string
	Tolerance int
	MsgMap    map[int][]string
	MemberLog map[string]*MemberInfo
	mu        sync.Mutex
	DataDir   string
}

// -------- Heartbeat RPC
func (s *LeaderServer) Heartbeat(ctx context.Context, hb *pb.HeartbeatRequest) (*emptypb.Empty, error) {
	log := logger.WithContext(ctx, logger.Leader)

	s.mu.Lock()
	defer s.mu.Unlock()

	member, ok := s.MemberLog[hb.Address]
	if !ok {
		return &emptypb.Empty{}, nil
	}

	member.LastSeen = time.Now()
	member.Alive = true
	log.Printf("Heartbeat received from %s", hb.Address)

	return &emptypb.Empty{}, nil
}

// -------- Üyeleri periyodik kontrol eder
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
						logger.Leader.Printf("Member down: %s", m.Address)
					}
					m.Alive = false
				}
			}
			s.mu.Unlock()
		}
	}()
}

// -------- Leader oluşturma
func NewLeaderServer(members []string, toleranceFile string) (*LeaderServer, error) {
	logger.Leader.Printf("Tolerance file path: %s", toleranceFile)

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

// -------- Yeni üye kaydı
func (s *LeaderServer) RegisterMember(ctx context.Context, req *pb.MemberInfo) (*pb.RegisterReply, error) {
	log := logger.WithContext(ctx, logger.Leader)

	s.mu.Lock()
	defer s.mu.Unlock()

	addr := req.Address
	if m, exists := s.MemberLog[addr]; exists {
		m.Alive = true
		m.LastSeen = time.Now()
		return &pb.RegisterReply{Ok: true}, nil
	}

	now := time.Now()
	s.Members = append(s.Members, addr)
	s.MemberLog[addr] = &MemberInfo{
		Address:    addr,
		AddedAt:    now,
		MessageCnt: 0,
		LastSeen:   now,
		Alive:      true,
	}

	log.Printf("New member registered: %s", addr)
	return &pb.RegisterReply{Ok: true}, nil
}

// -------- SET (Store)
func (s *LeaderServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	log := logger.WithContext(ctx, logger.Leader)

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

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count < stats[j].count
	})

	var selected []string
	for i := 0; i < s.Tolerance && i < len(stats); i++ {
		selected = append(selected, stats[i].addr)
	}

	if len(selected) < s.Tolerance {
		return &pb.StoreResult{Ok: false, Err: "Not enough alive members"}, nil
	}

	var successful []string

	for _, addr := range selected {
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Connection error to %s: %v", addr, err)
			continue
		}

		client := pb.NewStorageServiceClient(conn)

		ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
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
			log.Printf("Failed to store on member %s: %v", addr, callErr)
		}
	}

	if len(successful) == s.Tolerance {
		s.mu.Lock()
		s.MsgMap[int(msg.Id)] = successful
		s.mu.Unlock()
		return &pb.StoreResult{Ok: true}, nil
	}

	return &pb.StoreResult{Ok: false, Err: "Replication incomplete"}, nil
}

// -------- GET (Retrieve)
func (s *LeaderServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	log := logger.WithContext(ctx, logger.Leader)

	s.mu.Lock()
	replicas, ok := s.MsgMap[int(req.Id)]
	members := append([]string(nil), s.Members...)
	s.mu.Unlock()

	targets := replicas
	if !ok || len(replicas) == 0 {
		targets = members
	}

	for _, addr := range targets {
		s.mu.Lock()
		info := s.MemberLog[addr]
		alive := info != nil && info.Alive
		s.mu.Unlock()

		if !alive {
			continue
		}

		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("GET connect failed %s: %v", addr, err)
			continue
		}

		client := pb.NewStorageServiceClient(conn)

		ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
		res, callErr := client.Retrieve(ctx2, req)
		cancel()
		conn.Close()

		if callErr == nil && res != nil {
			return res, nil
		}
	}

	return nil, nil
}

// -------- TCP Client Handler
func (s *LeaderServer) HandleClient(conn net.Conn) {
	defer conn.Close()

	reqID := fmt.Sprintf("tcp-%d", time.Now().UnixNano())
	ctx := context.WithValue(context.Background(), logger.RequestIDKey, reqID)
	log := logger.WithContext(ctx, logger.Leader)

	log.Printf("TCP client connected: %s", conn.RemoteAddr())

	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, " ", 3)

		if len(parts) < 2 {
			conn.Write([]byte("ERROR: invalid command\n"))
			continue
		}

		cmd := strings.ToUpper(parts[0])
		id, err := strconv.Atoi(parts[1])
		if err != nil {
			conn.Write([]byte("ERROR: id must be numeric\n"))
			continue
		}

		switch cmd {
		case "SET":
			if len(parts) != 3 {
				conn.Write([]byte("ERROR: SET needs message\n"))
				continue
			}
			msg := &pb.StoredMessage{Id: int32(id), Text: parts[2]}
			res, _ := s.Store(ctx, msg)
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
			res, _ := s.Retrieve(ctx, &pb.MessageID{Id: int32(id)})
			if res != nil && res.Text != "" {
				conn.Write([]byte(res.Text + "\n"))
			} else {
				conn.Write([]byte("NOT_FOUND\n"))
			}

		default:
			conn.Write([]byte("ERROR: unknown command\n"))
		}
	}
}

// -------- Üye istatistikleri
func (s *LeaderServer) PrintMemberStats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Terminali temizle
	fmt.Print("\033[2J\033[H")

	fmt.Println("==== MEMBER STATUS (LIVE VIEW) ====")
	for _, info := range s.MemberLog {
		fmt.Printf(
			"Addr=%s | Alive=%v | LastSeen=%s | MsgCnt=%d\n",
			info.Address,
			info.Alive,
			info.LastSeen.Format("15:04:05"),
			info.MessageCnt,
		)
	}
	fmt.Println("==================================")
}
