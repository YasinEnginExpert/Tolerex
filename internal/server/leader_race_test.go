package server

import (
	"context"
	"os"
	"sync"
	"testing"

	"tolerex/internal/logger"
	pb "tolerex/proto/gen"
)

func init() {
	_ = os.MkdirAll("test_race", 0755)

	// Test modu aktif
	_ = os.Setenv("TOLEREX_TEST_MODE", "1")
	_ = os.Setenv("TOLEREX_TEST_DIR", "test_race")

	logger.Init()
}

func TestLeaderServer_ConcurrentStore(t *testing.T) {

	s := &LeaderServer{
		Members:   []string{"m1", "m2", "m3"},
		Tolerance: 2,
		MsgMap:    make(map[int][]string),
		MemberLog: map[string]*MemberInfo{
			"m1": {Address: "m1", Alive: true},
			"m2": {Address: "m2", Alive: true},
			"m3": {Address: "m3", Alive: true},
		},

		//NETWORK + gRPC COMPLETELY DISABLED
		replicateFn: func(ctx context.Context, addr string, msg *pb.StoredMessage) bool {
			return true // pretend replication succeeded
		},
	}

	ctx := context.Background()

	const goroutines = 10
	const writesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < writesPerGoroutine; i++ {
				id := int32(gid*1000 + i)
				_, _ = s.Store(ctx, &pb.StoredMessage{
					Id:   id,
					Text: "HELLO",
				})
			}
		}(g)
	}

	wg.Wait()
}
