// ===================================================================================
// TOLEREX – LEADER SERVER (CONTROL & DATA PLANE ORCHESTRATION)
// ===================================================================================
//
// This file defines the Leader node in the Tolerex fault-tolerant storage cluster.
//
// Responsibilities of Leader:
// - Manage membership (register new members, track heartbeat/liveness).
// - Decide replication targets for storing messages (prefer least-loaded members).
// - Coordinate Store and Retrieve operations via gRPC with cluster members.
// - Maintain cluster state (members, message replicas) and persist it to disk for recovery.
// - Provide a local TCP control interface for operators (simple SET/GET commands).
//
// Key Implementation Details:
// 1. **RPC Handlers**: Implements RegisterMember, Heartbeat, Store, Retrieve using gRPC.
// 2. **Replication Strategy**: Stores each message on `Tolerance` members. Selects least-loaded live members to replicate. Attempts replication with timeouts; updates an in-memory message map on success.
// 3. **Fault Detection**: A heartbeat watcher marks members as DOWN if heartbeat messages stop (beyond timeout).
// 4. **State Persistence**: Cluster state (membership and message map) is saved to a JSON file (`leader_state.json`) for crash recovery.
// 5. **Operator Interface**: A simple TCP console (telnet style) provides admin commands (SET to store a message, GET to retrieve).
//
// **Concurrency Model**:
// - A single mutex `s.mu` protects all shared state (Members, MemberLog, MsgMap).
// - Critical sections are kept short to reduce lock contention:contentReference[oaicite:2]{index=2}; expensive operations (network calls, disk I/O) occur outside the lock by snapshotting state as needed.
// - The heartbeat watcher and RPC handlers run in separate goroutines; state changes are synchronized via the mutex.
//
// **Security**:
// - All Leader-to-Member gRPC calls use mutual TLS for authentication (`security.NewMTLSClientCreds`).
// - The operator TCP interface is intended for local use only, with limited commands.
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
// Helper struct used to sort Members by stored message count when selecting targets.
type memberStat struct {
	addr  string
	count int
}

// --- MEMBER RUNTIME STATE ---
// Metadata tracked by the Leader for each Member (for liveness and load).
type MemberInfo struct {
	Address    string
	AddedAt    time.Time
	MessageCnt int
	LastSeen   time.Time
	Alive      bool
}

// --- LEADER SERVER ---
// Implements the gRPC service and holds cluster coordination state.
type LeaderServer struct {
	pb.UnimplementedStorageServiceServer

	// --- CLUSTER MEMBERSHIP ---
	Members   []string
	Tolerance int

	// --- MESSAGE → REPLICA ADDRESSES MAP ---
	MsgMap map[int][]string

	// --- MEMBER METADATA REGISTRY ---
	MemberLog map[string]*MemberInfo

	// --- SHARED STATE LOCK ---
	mu sync.Mutex

	// --- mTLS DIAL OPTION FOR LEADER→MEMBER RPC ---
	memberDialOpt grpc.DialOption

	// Optional function hooks for testing overrides.
	dialFn      func(addr string) (*grpc.ClientConn, error)
	replicateFn func(ctx context.Context, addr string, msg *pb.StoredMessage) bool

	// --- RUNTIME LISTENERS ---
	TcpListener  net.Listener // Operator TCP console
	GrpcListener net.Listener // gRPC service listener
}

// --- PERSISTED LEADER STATE ---
// Struct for JSON serialization of Leader state (Members, MemberLog, MsgMap).
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
// Updates a Member's liveness timestamp and marks it alive.
func (s *LeaderServer) Heartbeat(ctx context.Context, hb *pb.HeartbeatRequest) (*emptypb.Empty, error) {
	log := logger.WithContext(ctx, logger.Leader)

	// Mutex-protected update of LastSeen and Alive status.
	s.mu.Lock()
	defer s.mu.Unlock()

	member, ok := s.MemberLog[hb.Address]
	if !ok {
		// Ignore heartbeat from unknown member (must register first).
		return &emptypb.Empty{}, nil
	}
	member.LastSeen = time.Now()
	member.Alive = true
	logger.Debug(log, "Heartbeat received from %s", hb.Address)
	return &emptypb.Empty{}, nil
}

// cleanupMemberUnsafe removes all replicas belonging to a DOWN member
// and resets its load counters.
// MUST be called with s.mu held.

// ===================================================================================
// LIVENESS SUPERVISION
// ===================================================================================

// --- HEARTBEAT WATCHER ---
// Periodically checks liveness and marks Members DOWN on timeout.
//
// IMPORTANT DESIGN DECISION:
// --------------------------
// When a Member is marked DOWN, its data is NOT removed from the cluster state.
// Only the liveness flag (Alive=false) is updated.
//
// Rationale:
// - Leader remains the single source of truth.
// - Message ownership is preserved to allow recovery if the Member comes back.
// - Physical data may still exist on disk even if the process is temporarily down.
// - Logical deletion is avoided unless explicitly triggered by an operator.
//
// This results in a non-destructive failure model:
// - Failure = temporary unavailability
// - Not equal to data loss
func (s *LeaderServer) StartHeartbeatWatcher() {
	logger.Info(logger.Leader, "Heartbeat watcher started (timeout=15s, interval=5s)")

	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for range ticker.C {
			logger.Debug(logger.Leader, "Heartbeat watcher tick")
			s.mu.Lock()
			now := time.Now()
			changed := false

			for _, m := range s.MemberLog {
				if m.LastSeen.IsZero() {
					m.LastSeen = m.AddedAt
				}
				if now.Sub(m.LastSeen) > heartbeatTimeout && m.Alive {
					logger.Warn(logger.Leader,
						"Member marked DOWN: addr=%s lastSeen=%s",
						m.Address,
						m.LastSeen.Format(time.RFC3339),
					)
					m.Alive = false
					changed = true
				}
			}
			s.mu.Unlock()
			if changed {
				_ = s.saveState()
			}
		}
	}()
}

// ===================================================================================
// LEADER INITIALIZATION & SECURITY
// ===================================================================================

// --- NEW LEADER SERVER ---
// Loads config, initializes registries and mTLS, and restores any persisted state.
func NewLeaderServer(members []string, toleranceFile string) (*LeaderServer, error) {
	logger.Info(logger.Leader, "LeaderServer initialization started")
	logger.Info(logger.Leader, "Tolerance file path: %s", toleranceFile)

	// Load tolerance configuration value.
	tolerance, err := config.ReadTolerance(toleranceFile)
	if err != nil {
		return nil, err
	}

	// Initialize member registry.
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

	// Prepare mTLS credentials for Leader→Member RPC (if not in test mode).
	if os.Getenv("TOLEREX_TEST_MODE") != "1" {
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
	}

	// Load persisted state from disk (best-effort).
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
//
// RECOVERY SEMANTICS:
// -------------------
// If a previously known Member re-registers:
// - Its Alive flag is set to true
// - Its historical message ownership is preserved
// - No data reconciliation is performed automatically
//
// The Leader assumes:
// "If the Member is alive again, its previously stored data may still be valid."
//
// This keeps recovery explicit and predictable.
func (s *LeaderServer) RegisterMember(ctx context.Context, req *pb.MemberInfo) (*pb.RegisterReply, error) {
	log := logger.WithContext(ctx, logger.Leader)

	s.mu.Lock()

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
		s.mu.Unlock()
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
	s.mu.Unlock()
	if err := s.saveState(); err != nil {
		logger.Warn(log, "Failed to persist new member state: %v", err)
	}
	logger.Info(log, "New member registered: %s", addr)
	return &pb.RegisterReply{Ok: true}, nil
}

// ===================================================================================
// LEADER → MEMBER CONNECTIVITY
// ===================================================================================

// --- DIAL MEMBER (mTLS) ---
// Establishes a short-lived mTLS gRPC connection to a Member.
func (s *LeaderServer) dialMember(addr string) (*grpc.ClientConn, error) {
	if s.dialFn != nil {
		return s.dialFn(addr) // use test override if provided
	}
	logger.Debug(logger.Leader, "Dialing member %s with mTLS", addr)
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()
	return grpc.DialContext(ctx, addr, s.memberDialOpt, grpc.WithBlock())
}

// ===================================================================================
// gRPC: STORE (REPLICATION COORDINATION)
// ===================================================================================

// --- STORE ---
// Replicates the message to TOLERANCE number of alive members.
//
// REPLICATION MODEL:
// ------------------
// - Only members with Alive=true are considered.
// - Members are selected based on lowest MessageCnt (simple load balancing).
// - Replication succeeds only if TOLERANCE writes succeed.
//
// GUARANTEES:
// - Successful Store implies at least TOLERANCE replicas were written.
// - Partial replication does NOT update MsgMap.
// - Leader state is persisted only after full success.
//
// NON-GUARANTEES:
// - No rebalancing if membership changes later.
// - No automatic healing of under-replicated messages.
func (s *LeaderServer) Store(ctx context.Context, msg *pb.StoredMessage) (*pb.StoreResult, error) {
	log := logger.WithContext(ctx, logger.Leader)

	logger.Info(log, "Store request received: msg_id=%d tolerance=%d", msg.Id, s.Tolerance)

	var stats []memberStat

	// Snapshot members and load counts under lock.
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

	// Select up to TOLERANCE least-loaded members.
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count < stats[j].count
	})
	selected := make([]string, 0, tol)
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

	successful := make([]string, 0, tol)
	for _, addr := range selected {
		var success bool
		if s.replicateFn != nil {
			// Use injected replication function (for testing).
			success = s.replicateFn(ctx, addr, msg)
			// (No logging on failure in test mode)
		} else {
			conn, err := s.dialMember(addr)
			if err != nil {
				logger.Warn(log, "Connection error to %s: %v", addr, err)
				continue
			}
			client := pb.NewStorageServiceClient(conn)
			ctx2, cancel := context.WithTimeout(ctx, rpcTimeout)
			res, callErr := client.Store(ctx2, msg)
			cancel()
			conn.Close()
			if callErr == nil && res != nil && res.Ok {
				success = true
			} else {
				logger.Error(log, "Replication failed: msg_id=%d member=%s err=%v", msg.Id, addr, callErr)
			}
		}
		if success {
			successful = append(successful, addr)
			if len(successful) == tol {
				break // achieved required replication count
			}
		}
	}

	// Update state based on replication results.
	if len(successful) > 0 {
		s.mu.Lock()
		for _, addr := range successful {
			if s.MemberLog[addr] != nil {
				s.MemberLog[addr].MessageCnt++
			}
		}
		full := len(successful) >= tol
		if full {
			// Store a copy of the replica list for this message ID.
			s.MsgMap[int(msg.Id)] = append([]string(nil), successful...)
		}
		s.mu.Unlock()
		if full {
			logger.Info(log, "Store replication successful: msg_id=%d replicas=%v", msg.Id, successful)
			_ = s.saveState()
			return &pb.StoreResult{Ok: true}, nil
		}
	}
	return &pb.StoreResult{Ok: false, Err: "Replication incomplete"}, nil
}

// ===================================================================================
// gRPC: RETRIEVE (READ PATH)
// ===================================================================================

// --- RETRIEVE ---
// Attempts to fetch the message from known replicas; falls back to any alive member.
//
// READ STRATEGY:
// --------------
// 1. Prefer known replica addresses from MsgMap.
// 2. Skip members that are currently marked as DOWN.
// 3. Return the first successful response.
//
// DESIGN NOTES:
// - Replica lists are logical, not guaranteed to be physically correct.
// - If a Member lost its disk but rejoined, retrieval may fail gracefully.
// - Leader does NOT attempt automatic repair or replica validation.
//
// This keeps the read path simple and deterministic.
func (s *LeaderServer) Retrieve(ctx context.Context, req *pb.MessageID) (*pb.StoredMessage, error) {
	log := logger.WithContext(ctx, logger.Leader)

	logger.Info(log, "Retrieve request received: msg_id=%d", req.Id)

	// Snapshot the replica map and members list under lock.
	s.mu.Lock()
	replicas, ok := s.MsgMap[int(req.Id)]
	members := append([]string(nil), s.Members...)
	s.mu.Unlock()

	// Decide which targets to try.
	var targets []string
	if ok && len(replicas) > 0 {
		targets = replicas
	} else {
		logger.Warn(log, "Replica map empty for msg_id=%d, trying all members", req.Id)
		targets = members
	}

	// Try each target in order until the message is found.
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
			logger.Warn(log, "Retrieve connect failed to %s: %v", addr, err)
			continue
		}
		client := pb.NewStorageServiceClient(conn)
		ctx2, cancel := context.WithTimeout(ctx, rpcTimeout)
		res, callErr := client.Retrieve(ctx2, req)
		cancel()
		conn.Close()
		if callErr == nil && res != nil {
			return res, nil // return the first successful retrieval
		}
	}
	// If not found on any candidate, return NotFound.
	return nil, status.Error(codes.NotFound, "message not found")
}

// ===================================================================================
// TCP CONTROL PLANE (LOCAL OPERATOR INTERFACE)
// ===================================================================================

// --- TCP CLIENT HANDLER ---
// Simple interactive console for operator commands (SET/GET).
func (s *LeaderServer) HandleClient(conn net.Conn) {
	defer conn.Close()
	reqID := fmt.Sprintf("tcp-%d", time.Now().UnixNano())
	ctx := context.WithValue(context.Background(), logger.RequestIDKey, reqID)
	log := logger.WithContext(ctx, logger.Leader)
	logger.Info(log, "TCP client connected: %s", conn.RemoteAddr())

	reader := bufio.NewScanner(conn)
	writer := bufio.NewWriter(conn)

	// Command reference banner.
	banner := []string{
		"---------------------------------------",
		"TOLEREX - Leader Node",
		"---------------------------------------",
		"",
		"Commands:",
		"  SET <id> <message>",
		"  <N> SET <message>",
		"  GET <id>",
		"  HELP",
		"  QUIT | EXIT",
		"",
		"Examples:",
		"  SET 10 HELLO",
		"  1000 SET HELLO",
		"---------------------------------------",
	}
	for _, line := range banner {
		fmt.Fprint(writer, line+"\r\n")
	}
	fmt.Fprint(writer, "tolerex> ")
	writer.Flush()

	// Command loop.
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
			res, _ := s.Store(ctx, &pb.StoredMessage{Id: int32(id), Text: msg})
			if res != nil && res.Ok {
				fmt.Fprint(writer, "OK\r\n")
			} else {
				fmt.Fprint(writer, "ERROR: store failed\r\n")
			}
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
		case "HELP":
			for _, line := range banner {
				fmt.Fprint(writer, line+"\r\n")
			}
		case "QUIT", "EXIT":
			fmt.Fprint(writer, "Bye\r\n")
			writer.Flush()
			logger.Info(log, "TCP client disconnected: %s", conn.RemoteAddr())
			return
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
// STATE PERSISTENCE (SAVE/LOAD)
// ===================================================================================

// --- SAVE STATE (THREAD-SAFE) ---
// Acquires the lock and writes Leader state to disk.
func (s *LeaderServer) saveState() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveStateUnsafe()
}

// --- SAVE STATE (UNSAFE) ---
// Must be called with s.mu already held. Saves state to leader_state.json.
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
	// Consider using a faster JSON library (e.g., jsoniter) for performance:contentReference[oaicite:3]{index=3}
	data, err := json.MarshalIndent(LeaderState{
		Members:   members,
		MemberLog: memberLog,
		MsgMap:    msgMap,
	}, "", "  ")
	if err != nil {
		return err
	}

	// --- WRITE TO FILE ---
	path := stateFilePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	err = os.WriteFile(path, data, 0644)
	if err == nil {
		logger.Info(logger.Leader, "Leader state persisted successfully")
	}
	return err
}

// --- LOAD STATE ---
// Loads saved state from disk at startup (if file exists).
func (s *LeaderServer) loadState() error {
	data, err := os.ReadFile(stateFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no state file, start fresh
		}
		return err
	}
	var state LeaderState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.MemberLog == nil {
		state.MemberLog = make(map[string]*MemberInfo)
	}
	if state.MsgMap == nil {
		state.MsgMap = make(map[int][]string)
	}
	s.Members = state.Members
	s.MemberLog = state.MemberLog
	s.MsgMap = state.MsgMap
	logger.Info(logger.Leader, "Leader state loaded: members=%d, msgMap entries=%d", len(s.Members), len(s.MsgMap))
	return nil
}

// ===================================================================================
// OPERATOR VISIBILITY
// ===================================================================================

// --- MEMBER STATUS DISPLAY ---
// Prints a snapshot of Member state to stdout (for operator use).
//
// DISPLAY SEMANTICS:
// ------------------
// - Alive=false means "temporarily unreachable", not "data lost".
// - MsgCnt reflects logical replica ownership tracked by the Leader.
// - Values are NOT recomputed from Member disks.
//
// This view is informational, not authoritative for data correctness.
func (s *LeaderServer) PrintMemberStats() {
	// Snapshot member info under lock.
	s.mu.Lock()
	snapshot := make([]MemberInfo, 0, len(s.MemberLog))
	for _, info := range s.MemberLog {
		if info != nil {
			snapshot = append(snapshot, *info)
		}
	}
	s.mu.Unlock()

	// Render the snapshot.
	fmt.Print("\033[2J\033[H") // clear screen
	fmt.Println("==== MEMBER STATUS (LIVE VIEW) ====")
	for _, info := range snapshot {
		fmt.Printf("Addr=%s | Alive=%v | LastSeen=%s | MsgCnt=%d\n",
			info.Address, info.Alive, info.LastSeen.Format("15:04:05"), info.MessageCnt)
	}
	fmt.Println("==================================")
}
