package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLeaderServer_InitState(t *testing.T) {
	// --- TEST ISOLATION ---
	tmp := t.TempDir()
	t.Setenv("TOLEREX_TEST_MODE", "1")
	t.Setenv("TOLEREX_TEST_DIR", tmp)

	// --- CREATE TEMP TOLERANCE FILE ---
	toleranceFile := filepath.Join(tmp, "tolerance.conf")
	if err := os.WriteFile(
		toleranceFile,
		[]byte("TOLERANCE=1\n"),
		0644,
	); err != nil {
		t.Fatalf("failed to write tolerance file: %v", err)
	}

	// --- INPUT MEMBERS ---
	members := []string{
		"127.0.0.1:7001",
		"127.0.0.1:7002",
	}

	// --- CREATE LEADER ---
	leader, err := NewLeaderServer(members, toleranceFile)
	if err != nil {
		t.Fatalf("NewLeaderServer failed: %v", err)
	}

	// --- BASIC ASSERTIONS ---
	if leader == nil {
		t.Fatal("leader is nil")
	}

	if leader.Tolerance != 1 {
		t.Errorf("expected tolerance=1, got %d", leader.Tolerance)
	}

	// --- MEMBERS ---
	if len(leader.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(leader.Members))
	}

	// --- MEMBER LOG ---
	if len(leader.MemberLog) != 2 {
		t.Errorf("expected 2 memberLog entries, got %d", len(leader.MemberLog))
	}

	for _, addr := range members {
		info, ok := leader.MemberLog[addr]
		if !ok {
			t.Fatalf("member %s missing from MemberLog", addr)
		}
		if !info.Alive {
			t.Errorf("member %s should be alive", addr)
		}
	}

	// --- MESSAGE MAP ---
	if leader.MsgMap == nil {
		t.Fatal("MsgMap is nil")
	}
	if len(leader.MsgMap) != 0 {
		t.Errorf("expected empty MsgMap, got %d entries", len(leader.MsgMap))
	}
}
