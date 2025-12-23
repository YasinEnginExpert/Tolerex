package server

import (
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
)

func TestLeader_CPUProfile(t *testing.T) {
	// --- TEST MODE ---
	tmp := t.TempDir()
	t.Setenv("TOLEREX_TEST_MODE", "1")
	t.Setenv("TOLEREX_TEST_DIR", tmp)

	// --- TOLERANCE CONFIG ---
	conf := filepath.Join(tmp, "tolerance.conf")
	if err := os.WriteFile(conf, []byte("TOLERANCE=1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	leader, err := NewLeaderServer(nil, conf)
	if err != nil {
		t.Fatal(err)
	}

	// --- CPU PROFILE FILE ---
	f, err := os.Create("cpu_leader.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()

	// --- WORKLOAD (HOT PATH) ---
	for i := 0; i < 10000; i++ {
		leader.Members = []string{"m1", "m2", "m3"}
		leader.MemberLog["m1"] = &MemberInfo{Address: "m1", Alive: true}
		leader.MemberLog["m2"] = &MemberInfo{Address: "m2", Alive: true}
		leader.MemberLog["m3"] = &MemberInfo{Address: "m3", Alive: true}

		leader.Members = leader.Members // force selection logic
	}
}
