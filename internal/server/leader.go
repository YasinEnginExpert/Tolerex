// ===================================================================================
// TOLEREX – LEADER SERVER (CONTROL PLANE + DATA PLANE ORCHESTRATION)
// ===================================================================================
//
// This file implements the core Leader-side server logic for the Tolerex
// distributed, fault-tolerant storage system.
//
// From a distributed systems perspective, the Leader is responsible for:
//
// - Membership management (dynamic registration + liveness tracking via heartbeats)
// - Replica placement decisions (selecting target Members for replication)
// - Coordinating Store/Retrieve operations across Members over mTLS-protected gRPC
// - Maintaining and persisting cluster metadata (member state + message replica map)
// - Providing an operator-facing TCP control interface (SET/GET/FS-like commands)
// - Exposing a live cluster view for operational visibility (stdout dashboard)
//
// Key subsystems implemented here:
//
// 1) gRPC RPC Surface:
//    - RegisterMember: Adds/recovers Members in the cluster registry
//    - Heartbeat     : Updates liveness timestamps and status
//    - Store         : Replicates a message to TOLERANCE number of Members
//    - Retrieve      : Fetches a message from known replicas or any alive Member
//
// 2) Replication Strategy:
//    - Chooses least-loaded Members based on per-Member MessageCnt
//    - Attempts replication with per-call timeouts and failure isolation
//    - Updates MsgMap (message → replica addresses) on success
//
// 3) Fault Detection:
//    - Heartbeat watcher periodically marks Members DOWN on timeout
//
// 4) State Persistence:
//    - Saves leader_state.json containing Members, MemberLog, MsgMap
//    - Loads persisted state at startup if available
//
// 5) Local TCP Control Plane:
//    - Exposes an interactive command interface for local operator usage
//    - Reuses Store/Retrieve logic for SET/GET commands
//
// Concurrency Model:
//
// - Shared state (Members, MemberLog, MsgMap) is protected by s.mu
// - Snapshotting is used where appropriate to reduce lock holding time
//
// Security Model:
//
// - Leader → Member RPC uses mTLS client credentials
// - Member dialing uses a bounded context timeout per connection attempt
//
// ===================================================================================

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// --- MEMBER LOAD SNAPSHOT ITEM ---
// Helper struct used to sort Members by stored message count
// when selecting replication targets.
type memberStat struct {
	addr  string
	count int
}

// --- MEMBER RUNTIME STATE ---
// Tracks Member metadata used by the Leader for:
// - liveness supervision
// - load-aware replica placement
// - operational visibility (live status view)
type MemberInfo struct {
	Address    string
	AddedAt    time.Time
	MessageCnt int
	LastSeen   time.Time
	Alive      bool
}

// --- LEADER SERVER ---
// Implements the generated gRPC service interface and also hosts
// Leader-specific coordination state.
type LeaderServer struct {
	pb.UnimplementedStorageServiceServer

	// --- CLUSTER MEMBERSHIP ---
	Members   []string
	Tolerance int

	// --- MESSAGE → REPLICA ADDRESSES ---
	MsgMap map[int][]string

	// --- MEMBER METADATA REGISTRY ---
	MemberLog map[string]*MemberInfo

	// --- SHARED STATE LOCK ---
	mu sync.Mutex

	// --- DIAL OPTION FOR LEADER → MEMBER (mTLS) ---
	memberDialOpt grpc.DialOption

	dialFn      func(addr string) (*grpc.ClientConn, error)
	replicateFn func(ctx context.Context, addr string, msg *pb.StoredMessage) bool

	// --- Runtime listeners (introspection only) ---
	TcpListener  net.Listener // TCP CLI (telnet-style console)
	GrpcListener net.Listener // gRPC service listener

}

// --- PERSISTED LEADER STATE ---
// Serialized to disk for restart recovery.
// (Members + MemberLog + MsgMap)
type LeaderState struct {
	Members   []string
	MemberLog map[string]*MemberInfo
	MsgMap    map[int][]string
}

func stateFilePath() string {
	if os.Getenv("TOLEREX_TEST_MODE") == "1" {
		if d := os.Getenv("TOLEREX_TEST_DIR"); d != "" {
			return filepath.Join(d, "leader_state.json")
		}
	}
	return "internal/data/leader_state.json"
}

const (
	heartbeatTimeout  = 15 * time.Second
	heartbeatInterval = 5 * time.Second
	rpcTimeout        = 2 * time.Second
)

// ===================================================================================
// gRPC: HEARTBEAT
// ===================================================================================

// --- HEARTBEAT RPC ---
// Updates a Member's liveness timestamps and marks it alive.
func (s *LeaderServer) Heartbeat(ctx context.Context, hb *pb.HeartbeatRequest) (*emptypb.Empty, error) {
	log := logger.WithContext(ctx, logger.Leader)

	// --- MUTEX-PROTECTED STATE UPDATE ---
	s.mu.Lock()
	defer s.mu.Unlock()

	member, ok := s.MemberLog[hb.Address]
	if !ok {
		// Unknown Member: ignore heartbeat (Member must register first)
		return &emptypb.Empty{}, nil
	}

	member.LastSeen = time.Now()
	member.Alive = true
	logger.Debug(log, "Heartbeat received from %s", hb.Address)

	return &emptypb.Empty{}, nil
}

// ===================================================================================
// LIVENESS SUPERVISION
// ===================================================================================

// --- HEARTBEAT WATCHER ---
// Periodically checks Member liveness and marks Members DOWN on timeout.
func (s *LeaderServer) StartHeartbeatWatcher() {
	logger.Info(logger.Leader, "Heartbeat watcher started (timeout=15s, interval=5s)")

	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for range ticker.C {
			logger.Debug(logger.Leader, "Heartbeat watcher tick")
			s.mu.Lock()
			now := time.Now()

			for _, m := range s.MemberLog {
				if m.LastSeen.IsZero() {
					m.LastSeen = m.AddedAt
				}
				if now.Sub(m.LastSeen) > heartbeatTimeout {
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

// ===================================================================================
// LEADER CONSTRUCTION & SECURITY BOOTSTRAP
// ===================================================================================

// --- NEW LEADER SERVER ---
// Loads tolerance config, initializes registries, prepares mTLS dial option,
// and attempts to load persisted leader state.
func NewLeaderServer(members []string, toleranceFile string) (*LeaderServer, error) {
	logger.Info(logger.Leader, "LeaderServer initialization started")
	logger.Info(logger.Leader, "Tolerance file path: %s", toleranceFile)

	// --- TOLERANCE LOAD ---
	tolerance, err := config.ReadTolerance(toleranceFile)
	if err != nil {
		return nil, err
	}

	// --- INITIAL MEMBER REGISTRY ---
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

	// --- mTLS CLIENT CREDS: LEADER → MEMBER ---
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

	// --- LOAD PERSISTED STATE (BEST-EFFORT) ---
	if err := leader.loadState(); err != nil {
		logger.Warn(logger.Leader, "Failed to load leader state: %v", err)
	}

	return leader, nil
}

// ===================================================================================
// gRPC: MEMBER REGISTRATION
// ===================================================================================

// --- REGISTER MEMBER ---
// Adds a new Member or marks an existing one as recovered.
func (s *LeaderServer) RegisterMember(ctx context.Context, req *pb.MemberInfo) (*pb.RegisterReply, error) {
	log := logger.WithContext(ctx, logger.Leader)

	s.mu.Lock()
	defer s.mu.Unlock()

	addr := req.Address

	// --- RE-REGISTRATION / RECOVERY PATH ---
	if m, exists := s.MemberLog[addr]; exists {
		if !m.Alive {
			logger.Info(log, "Member recovered: %s", addr)
		} else {
			logger.Debug(log, "Member already registered and alive: %s", addr)
		}

		m.Alive = true
		m.LastSeen = time.Now()
		return &pb.RegisterReply{Ok: true}, nil
	}

	// --- NEW MEMBER PATH ---
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

// ===================================================================================
// LEADER → MEMBER CONNECTIVITY
// ===================================================================================

// --- DIAL MEMBER (mTLS) ---
// Establishes a bounded-timeout mTLS gRPC connection to a Member.
func (s *LeaderServer) dialMember(addr string) (*grpc.ClientConn, error) {

	if s.dialFn != nil {
		return s.dialFn(addr)
	}

	logger.Debug(logger.Leader, "Dialing member %s with mTLS", addr)

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	return grpc.DialContext(
		ctx,
		addr,
		s.memberDialOpt,
		grpc.WithBlock(),
	)
}

// ===================================================================================
// gRPC: STORE (REPLICATION COORDINATION)
// ===================================================================================

// --- STORE ---
// Replicates the message to TOLERANCE number of alive Members.
// Chooses least-loaded Members first using MessageCnt as a heuristic.
func (s *LeaderServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	log := logger.WithContext(ctx, logger.Leader)

	logger.Info(log, "Store request received: msg_id=%d tolerance=%d", msg.Id, s.Tolerance)

	var stats []memberStat

	// --- SNAPSHOT MEMBERS + LOAD COUNTS ---
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

	// --- SORT BY LEAST-LOADED ---
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count < stats[j].count
	})

	// --- SELECT TARGETS UP TO TOLERANCE ---
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

	logger.Debug(log, "Replication targets selected: %v", selected)

	// --- ATTEMPT REPLICATION ---
	var successful []string

	for _, addr := range selected {
		if len(successful) == tol {
			s.mu.Lock()
			s.MsgMap[int(msg.Id)] = successful
			s.mu.Unlock()
			_ = s.saveState()
			return &pb.StoreResult{Ok: true}, nil
		}

		if s.replicateFn != nil {
			if s.replicateFn(ctx, addr, msg) {
				successful = append(successful, addr)

				s.mu.Lock()
				if s.MemberLog[addr] != nil {
					s.MemberLog[addr].MessageCnt++
				}
				s.mu.Unlock()
			}
			continue
		}

		conn, err := s.dialMember(addr)
		if err != nil {
			logger.Warn(log, "Connection error to %s: %v", addr, err)
			continue
		}

		client := pb.NewStorageServiceClient(conn)

		// --- PER-CALL TIMEOUT ---
		ctx2, cancel := context.WithTimeout(ctx, rpcTimeout)
		res, callErr := client.Store(ctx2, msg)
		cancel()
		conn.Close()

		if callErr == nil && res != nil && res.Ok {
			successful = append(successful, addr)

			// --- UPDATE LOAD COUNTER ---
			s.mu.Lock()
			if s.MemberLog[addr] != nil {
				s.MemberLog[addr].MessageCnt++
			}
			s.mu.Unlock()
		} else {
			// Replication failure does NOT imply member failure.
			// Liveness is decided exclusively by the heartbeat watcher.
			logger.Error(
				log,
				"Replication failed: msg_id=%d member=%s err=%v",
				msg.Id,
				addr,
				callErr,
			)
		}
	}

	// --- COMMIT REPLICA MAP ON FULL SUCCESS ---
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

// ===================================================================================
// gRPC: RETRIEVE (READ PATH)
// ===================================================================================

// --- RETRIEVE ---
// Attempts to retrieve the message from known replicas.
// If replicas are unknown, falls back to trying any alive Member.
func (s *LeaderServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	log := logger.WithContext(ctx, logger.Leader)

	logger.Info(log, "Retrieve request received: msg_id=%d", req.Id)

	// --- SNAPSHOT REPLICA MAP + MEMBERS ---
	s.mu.Lock()
	replicas, ok := s.MsgMap[int(req.Id)]
	members := append([]string(nil), s.Members...)
	s.mu.Unlock()

	targets := replicas
	if !ok || len(replicas) == 0 {
		logger.Warn(log,
			"Replica map empty for msg_id=%d, falling back to all members",
			req.Id,
		)
		targets = members
	}

	// --- TRY TARGETS IN ORDER ---
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

		ctx2, cancel := context.WithTimeout(ctx, rpcTimeout)
		res, callErr := client.Retrieve(ctx2, req)
		cancel()
		conn.Close()

		if callErr == nil && res != nil {
			return res, nil
		}
	}

	// If not found on any candidate, return NotFound.
	return nil, status.Error(codes.NotFound, "message not found")
}

// leaderListeningPorts returns TCP listening sockets owned
// by the current leader process.
//
// This is strictly read-only introspection.
// No user-controlled command execution is performed.

// ===================================================================================
// TCP CONTROL PLANE (LOCAL OPERATOR INTERFACE)
// ===================================================================================

// --- TCP CLIENT HANDLER ---
// Provides a simple interactive command protocol over TCP.
// This is intended for localhost-only operator usage (as configured by main).
func (s *LeaderServer) HandleClient(conn net.Conn) {
	defer conn.Close()

	// --- SYNTHETIC REQUEST ID FOR TCP SESSION ---
	reqID := fmt.Sprintf("tcp-%d", time.Now().UnixNano())
	ctx := context.WithValue(context.Background(), logger.RequestIDKey, reqID)
	log := logger.WithContext(ctx, logger.Leader)

	logger.Info(log, "TCP client connected: %s", conn.RemoteAddr())

	reader := bufio.NewScanner(conn)
	writer := bufio.NewWriter(conn)

	// --- BANNER / HELP TEXT ---
	banner := []string{
		"---------------------------------------",
		"TOLEREX - Leader Node",
		"---------------------------------------",
		"",
		"Commands:",
		"",
		"  SET <id> <message>",
		"  <N> SET <message>",
		"  GET <id>",
		"  HELP",
		"  QUIT | EXIT",
		"",
		"Examples:",
		"  SET 10 HELLO",
		"  1000 SET HELLO",
		"",
		"---------------------------------------",
	}

	for _, line := range banner {
		fmt.Fprint(writer, line+"\r\n")
	}
	fmt.Fprint(writer, "tolerex> ")
	writer.Flush()

	// --- COMMAND LOOP ---
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		logger.Debug(log, "TCP command received: %s", line)
		if line == "" {
			fmt.Fprint(writer, "tolerex> ")
			writer.Flush()
			continue
		}

		parts := strings.Fields(line)
		cmd := strings.ToUpper(parts[0])

		switch cmd {
		// --- SET (STORE) ---
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

		// --- GET (RETRIEVE) ---
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

			res, err := s.Retrieve(ctx, &pb.MessageID{Id: int32(id)})
			if err != nil {
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.NotFound {
					fmt.Fprint(writer, "NOT_FOUND\r\n")
				} else {
					fmt.Fprint(writer, "ERROR: retrieve failed\r\n")
				}
				break
			}
			fmt.Fprint(writer, res.Text+"\r\n")
		// --- HELP ---
		case "HELP":
			for _, line := range banner {
				fmt.Fprint(writer, line+"\r\n")
			}

		// --- QUIT / EXIT ---
		case "QUIT", "EXIT":
			fmt.Fprint(writer, "Bye \r\n")
			writer.Flush()
			logger.Info(log, "TCP client disconnected: %s", conn.RemoteAddr())
			return

		// --- UNKNOWN COMMAND ---
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

// ===================================================================================
// STATE PERSISTENCE (SAVE / LOAD)
// ===================================================================================

// --- SAVE STATE (LOCKING WRAPPER) ---
func (s *LeaderServer) saveState() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveStateUnsafe()
}

// --- SAVE STATE (UNSAFE) ---
// Must be called while s.mu is already held.
func (s *LeaderServer) saveStateUnsafe() error {
	// --- SNAPSHOT MEMBERS ---
	members := append([]string(nil), s.Members...)

	// --- DEEP COPY MEMBER LOG ---
	memberLog := make(map[string]*MemberInfo, len(s.MemberLog))

	for k, v := range s.MemberLog {
		if v == nil {
			continue
		}
		cp := *v
		memberLog[k] = &cp
	}

	// --- DEEP COPY MESSAGE MAP ---
	msgMap := make(map[int][]string, len(s.MsgMap))
	for id, addrs := range s.MsgMap {
		msgMap[id] = append([]string(nil), addrs...)
	}

	// --- JSON SERIALIZATION ---
	data, err := json.MarshalIndent(LeaderState{
		Members:   members,
		MemberLog: memberLog,
		MsgMap:    msgMap,
	}, "", "  ")
	if err != nil {
		return err
	}

	// --- ENSURE DIRECTORY + WRITE FILE ---
	path := stateFilePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	err = os.WriteFile(path, data, 0644)
	if err == nil {
		logger.Info(logger.Leader, "Leader state persisted successfully")
	}
	return err
}

// --- LOAD STATE ---
// Best-effort load from disk. Missing file is treated as empty state.
func (s *LeaderServer) loadState() error {
	data, err := os.ReadFile(stateFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var state LeaderState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.Members = state.Members
	s.MemberLog = state.MemberLog
	if state.MemberLog == nil {
		state.MemberLog = make(map[string]*MemberInfo)
	}
	if state.MsgMap == nil {
		state.MsgMap = make(map[int][]string)
	}
	s.MsgMap = state.MsgMap

	logger.Info(logger.Leader,
		"Leader state loaded: members=%d msgMap=%d",
		len(s.Members),
		len(s.MsgMap),
	)
	return nil
}

// ===================================================================================
// LIVE OPERATOR VIEW
// ===================================================================================

// --- MEMBER STATUS DISPLAY ---
// Prints a live snapshot of Member state to stdout.
// Intended for operator visibility (not persistent logging).
func (s *LeaderServer) PrintMemberStats() {
	// --- SNAPSHOT MEMBER INFO UNDER LOCK ---
	s.mu.Lock()
	snapshot := make([]MemberInfo, 0, len(s.MemberLog))
	for _, info := range s.MemberLog {
		if info == nil {
			continue
		}
		snapshot = append(snapshot, *info)
	}
	s.mu.Unlock()

	// --- CLEAR SCREEN + RENDER ---
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
