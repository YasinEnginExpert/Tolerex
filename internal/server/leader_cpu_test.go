package server

import (
	"context"
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"

	pb "tolerex/proto/gen"

	"google.golang.org/grpc"
)

func TestLeader_CPUProfile(t *testing.T) {

	// ------------------------------------------------------------
	// TEST MODE
	// ------------------------------------------------------------
	tmp := t.TempDir()
	t.Setenv("TOLEREX_TEST_MODE", "1")
	t.Setenv("TOLEREX_TEST_DIR", tmp)

	// ------------------------------------------------------------
	// TOLERANCE CONFIG
	// ------------------------------------------------------------
	conf := filepath.Join(tmp, "tolerance.conf")
	if err := os.WriteFile(conf, []byte("TOLERANCE=1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	leader, err := NewLeaderServer(nil, conf)
	if err != nil {
		t.Fatal(err)
	}

	// ------------------------------------------------------------
	// CLUSTER STATE
	// ------------------------------------------------------------
	leader.Members = []string{"m1", "m2", "m3"}
	leader.MemberLog = map[string]*MemberInfo{
		"m1": {Address: "m1", Alive: true},
		"m2": {Address: "m2", Alive: true},
		"m3": {Address: "m3", Alive: true},
	}

	// ------------------------------------------------------------
	// MOCK: REPLICATION (NO saveState)
	// ------------------------------------------------------------
	leader.replicateFn = func(ctx context.Context, addr string, msg *pb.StoredMessage) bool {
		return false
	}

	// ------------------------------------------------------------
	// MOCK: DIAL MEMBER (CRITICAL FIX)
	// ------------------------------------------------------------
	leader.dialFn = func(addr string) (*grpc.ClientConn, error) {
		return nil, grpc.ErrClientConnClosing
	}

	// ------------------------------------------------------------
	// CPU PROFILE
	// ------------------------------------------------------------
	f, err := os.Create("leader_cpu.prof")
	prof := f.Name()

	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	pprof.StartCPUProfile(f)

	defer pprof.StopCPUProfile()

	// ------------------------------------------------------------
	// HOT PATH
	// ------------------------------------------------------------
	ctx := context.Background()

	for i := 0; i < 30_000; i++ {
		_, _ = leader.Store(ctx, &pb.StoredMessage{
			Id:   int32(i),
			Text: "hello",
		})

		_, _ = leader.Retrieve(ctx, &pb.MessageID{
			Id: int32(i),
		})
	}

	t.Logf("CPU profile written to %s", prof)
}
