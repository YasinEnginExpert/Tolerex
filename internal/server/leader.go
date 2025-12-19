package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"tolerex/internal/config"
	"tolerex/internal/logger"
	"tolerex/internal/security"
	pb "tolerex/proto/gen"

	"google.golang.org/grpc"
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

	memberDialOpt grpc.DialOption
}

// --- Kaydeilecek bilgiler
type LeaderState struct {
	Members   []string
	MemberLog map[string]*MemberInfo
	MsgMap    map[int][]string
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
	logger.Debug(log, "Heartbeat received from %s", hb.Address)

	return &emptypb.Empty{}, nil
}

// -------- Üyeleri periyodik kontrol eder
func (s *LeaderServer) StartHeartbeatWatcher() {
	logger.Info(logger.Leader, "Heartbeat watcher started (timeout=15s, interval=5s)")

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
						logger.Warn(logger.Leader,
							"Member marked DOWN: addr=%s lastSeen=%s",
							m.Address,
							m.LastSeen.Format(time.RFC3339),
						)
						m.Alive = false
						_ = s.saveStateUnsafe()
					}
				}
			}
			s.mu.Unlock()
		}
	}()
}

// -------- Leader oluşturma
func NewLeaderServer(members []string, toleranceFile string) (*LeaderServer, error) {
	logger.Info(logger.Leader, "Tolerance file path: %s", toleranceFile)

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

	leader := &LeaderServer{
		Members:   members,
		Tolerance: tolerance,
		MsgMap:    make(map[int][]string),
		MemberLog: memberLog,
	}

	creds, err := security.NewMTLSClientCreds(
		"config/tls/leader.crt",
		"config/tls/leader.key",
		"config/tls/ca.crt",
		"member",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to init leader->member mTLS creds: %w", err)
	}
	leader.memberDialOpt = grpc.WithTransportCredentials(creds)

	if err := leader.loadState(); err != nil {
		logger.Warn(logger.Leader, "Failed to load leader state: %v", err)
	}

	return leader, nil
}

// -------- Yeni üye kaydı
func (s *LeaderServer) RegisterMember(ctx context.Context, req *pb.MemberInfo) (*pb.RegisterReply, error) {
	log := logger.WithContext(ctx, logger.Leader)

	s.mu.Lock()
	defer s.mu.Unlock()

	addr := req.Address
	if m, exists := s.MemberLog[addr]; exists {
		if !m.Alive {
			logger.Info(log, "Member recovered: %s", addr)
		}
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

	_ = s.saveStateUnsafe()

	logger.Info(log, "New member registered: %s", addr)
	return &pb.RegisterReply{Ok: true}, nil
}

func (s *LeaderServer) dialMember(addr string) (*grpc.ClientConn, error) {
	logger.Debug(logger.Leader, "Dialing member %s with mTLS", addr)

	// Dial ayrı timeout ile
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	return grpc.DialContext(ctx, addr, s.memberDialOpt)
}

// -------- SET (Store)
func (s *LeaderServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	log := logger.WithContext(ctx, logger.Leader)

	logger.Info(log, "Store request received: msg_id=%d tolerance=%d", msg.Id, s.Tolerance)

	var stats []memberStat

	// snapshot: members + messageCnt
	s.mu.Lock()
	for _, addr := range s.Members {
		info := s.MemberLog[addr]
		if info == nil || !info.Alive {
			continue
		}
		stats = append(stats, memberStat{addr: addr, count: info.MessageCnt})
	}
	tol := s.Tolerance
	s.mu.Unlock()

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count < stats[j].count
	})

	var selected []string
	for _, st := range stats {
		if len(selected) == tol {
			break
		}
		selected = append(selected, st.addr)
	}
	if len(selected) < tol {
		return &pb.StoreResult{Ok: false, Err: "Not enough alive members"}, nil
	}

	var successful []string

	for _, addr := range selected {
		if len(successful) == tol {
			break
		}

		conn, err := s.dialMember(addr)
		if err != nil {
			logger.Warn(log, "Connection error to %s: %v", addr, err)
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
			logger.Error(log, "Failed to store on member %s: %v", addr, callErr)
			s.mu.Lock()
			if s.MemberLog[addr] != nil {
				s.MemberLog[addr].Alive = false
			}
			s.mu.Unlock()
		}
	}

	if len(successful) == tol {
		s.mu.Lock()
		s.MsgMap[int(msg.Id)] = successful
		s.mu.Unlock()

		logger.Info(log, "Store replication successful: msg_id=%d replicas=%v", msg.Id, successful)

		_ = s.saveState()
		return &pb.StoreResult{Ok: true}, nil
	}

	return &pb.StoreResult{Ok: false, Err: "Replication incomplete"}, nil
}

// -------- GET (Retrieve)
func (s *LeaderServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	log := logger.WithContext(ctx, logger.Leader)

	logger.Info(log, "Retrieve request received: msg_id=%d", req.Id)

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

		conn, err := s.dialMember(addr)
		if err != nil {
			logger.Warn(log, "GET connect failed %s: %v", addr, err)
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

	logger.Info(log, "TCP client connected: %s", conn.RemoteAddr())

	reader := bufio.NewScanner(conn)
	writer := bufio.NewWriter(conn)

	// -------- Banner --------
	banner := []string{
		"--------------------------------------------------",
		"TOLEREX - Distributed Storage Leader Node",
		"--------------------------------------------------",
		"",
		"Commands:",
		"",
		"  SET <id> <message>   Store a message",
		"  GET <id>             Retrieve a message",
		"  PWD                  Show current directory",
		"  CD <dir>             Change directory",
		"  DIR                  List directory contents",
		"  HELP                 Show this help",
		"  QUIT | EXIT          Close connection",
		"",
		"--------------------------------------------------",
	}

	for _, line := range banner {
		fmt.Fprint(writer, line+"\r\n")
	}
	fmt.Fprint(writer, "tolerex> ")
	writer.Flush()

	// -------- Command loop --------
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			fmt.Fprint(writer, "tolerex> ")
			writer.Flush()
			continue
		}

		parts := strings.Fields(line)
		cmd := strings.ToUpper(parts[0])

		switch cmd {

		// -------- SET --------
		case "SET":
			if len(parts) < 3 {
				fmt.Fprint(writer, "ERROR: SET <id> <message>\r\n")
				break
			}

			id, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Fprint(writer, "ERROR: id must be numeric\r\n")
				break
			}

			msg := strings.Join(parts[2:], " ")
			res, _ := s.Store(ctx, &pb.StoredMessage{
				Id:   int32(id),
				Text: msg,
			})

			if res != nil && res.Ok {
				fmt.Fprint(writer, "OK\r\n")
			} else {
				fmt.Fprint(writer, "ERROR: store failed\r\n")
			}

		// -------- GET --------
		case "GET":
			if len(parts) != 2 {
				fmt.Fprint(writer, "ERROR: GET <id>\r\n")
				break
			}

			id, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Fprint(writer, "ERROR: id must be numeric\r\n")
				break
			}

			res, _ := s.Retrieve(ctx, &pb.MessageID{Id: int32(id)})
			if res != nil && res.Text != "" {
				fmt.Fprint(writer, res.Text+"\r\n")
			} else {
				fmt.Fprint(writer, "NOT_FOUND\r\n")
			}

		// -------- PWD --------
		case "PWD":
			dir, err := os.Getwd()
			if err != nil {
				fmt.Fprint(writer, "ERROR: cannot get pwd\r\n")
			} else {
				fmt.Fprint(writer, dir+"\r\n")
			}

		// -------- CD --------
		case "CD":
			if len(parts) != 2 {
				fmt.Fprint(writer, "ERROR: CD <dir>\r\n")
				break
			}

			if err := os.Chdir(parts[1]); err != nil {
				fmt.Fprint(writer, "ERROR: cannot change directory\r\n")
			} else {
				fmt.Fprint(writer, "OK\r\n")
			}

		// -------- DIR --------
		case "DIR":
			files, err := os.ReadDir(".")
			if err != nil {
				fmt.Fprint(writer, "ERROR: cannot read directory\r\n")
				break
			}

			for _, f := range files {
				fmt.Fprint(writer, f.Name()+"\r\n")
			}
			fmt.Fprint(writer, "END\r\n")

		// -------- HELP --------
		case "HELP":
			for _, line := range banner {
				fmt.Fprint(writer, line+"\r\n")
			}

		// -------- QUIT / EXIT --------
		case "QUIT", "EXIT":
			fmt.Fprint(writer, "Bye \r\n")
			writer.Flush()
			logger.Info(log, "TCP client disconnected: %s", conn.RemoteAddr())
			return

		// -------- UNKNOWN --------
		default:
			fmt.Fprint(writer, "ERROR: unknown command\r\n")
		}

		fmt.Fprint(writer, "tolerex> ")
		writer.Flush()
	}

	if err := reader.Err(); err != nil {
		logger.Warn(log, "TCP client error: %v", err)
	}
}

func (s *LeaderServer) saveState() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveStateUnsafe()
}

// s.mu kilidi tutulmuşken çağrılmalı
func (s *LeaderServer) saveStateUnsafe() error {
	members := append([]string(nil), s.Members...)

	memberLog := make(map[string]*MemberInfo, len(s.MemberLog))
	for k, v := range s.MemberLog {
		if v == nil {
			continue
		}
		cp := *v
		memberLog[k] = &cp
	}

	msgMap := make(map[int][]string, len(s.MsgMap))
	for id, addrs := range s.MsgMap {
		msgMap[id] = append([]string(nil), addrs...)
	}

	data, err := json.MarshalIndent(LeaderState{
		Members:   members,
		MemberLog: memberLog,
		MsgMap:    msgMap,
	}, "", "  ")
	if err != nil {
		return err
	}

	_ = os.MkdirAll("internal/data", 0755)
	return os.WriteFile("internal/data/leader_state.json", data, 0644)
}

func (s *LeaderServer) loadState() error {
	data, err := os.ReadFile("internal/data/leader_state.json")
	if err != nil {
		return nil
	}

	var state LeaderState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.Members = state.Members
	s.MemberLog = state.MemberLog
	s.MsgMap = state.MsgMap
	return nil
}

// -------- Üye istatistikleri
func (s *LeaderServer) PrintMemberStats() {
	s.mu.Lock()
	snapshot := make([]MemberInfo, 0, len(s.MemberLog))
	for _, info := range s.MemberLog {
		if info == nil {
			continue
		}
		snapshot = append(snapshot, *info)
	}
	s.mu.Unlock()

	fmt.Print("\033[2J\033[H")
	fmt.Println("==== MEMBER STATUS (LIVE VIEW) ====")
	for _, info := range snapshot {
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
