package server

import (
	"reflect"
	"testing"
)

func TestLeastLoadedBalancer_Pick(t *testing.T) {
	balancer := &LeastLoadedBalancer{}

	candidates := []MemberLoad{
		{Addr: "A", Count: 10},
		{Addr: "B", Count: 5},
		{Addr: "C", Count: 20},
		{Addr: "D", Count: 2},
	}

	// Case 1: Tolerance = 2 (Should pick D=2 and B=5)
	got := balancer.Pick(candidates, 2)
	want := []MemberLoad{
		{Addr: "D", Count: 2},
		{Addr: "B", Count: 5},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Pick(2) = %v, want %v", got, want)
	}

	// Case 2: Tolerance > Candidates (Should pick all sorted)
	gotAll := balancer.Pick(candidates, 100)
	wantAll := []MemberLoad{
		{Addr: "D", Count: 2},
		{Addr: "B", Count: 5},
		{Addr: "A", Count: 10},
		{Addr: "C", Count: 20},
	}

	if !reflect.DeepEqual(gotAll, wantAll) {
		t.Errorf("Pick(100) = %v, want %v", gotAll, wantAll)
	}
}

func TestPowerOfTwoChoicesBalancer_Pick(t *testing.T) {
	balancer := &PowerOfTwoChoicesBalancer{}

	candidates := []MemberLoad{
		{Addr: "A", Count: 10},
		{Addr: "B", Count: 5},
		{Addr: "C", Count: 20},
		{Addr: "D", Count: 2},
	}

	// Since P2C is probabilistic, we can generally check:
	// 1. Length is correct
	// 2. Returned items are from the original set

	tolerance := 2
	got := balancer.Pick(candidates, tolerance)

	if len(got) != tolerance {
		t.Errorf("Pick(%d) returned %d items", tolerance, len(got))
	}

	for _, m := range got {
		found := false
		for _, c := range candidates {
			if m.Addr == c.Addr {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Pick returned unknown member: %v", m)
		}
	}
}
