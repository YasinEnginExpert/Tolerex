package balancer

import (
	"fmt"
	"testing"
)

// Helper to generate N dummy members
func generateMembers(n int) []MemberLoad {
	m := make([]MemberLoad, n)
	for i := 0; i < n; i++ {
		m[i] = MemberLoad{
			Addr:  fmt.Sprintf("member-%d", i),
			Count: i * 10, // Deterministic load
		}
	}
	return m
}

func BenchmarkLeastLoaded_100_Members(b *testing.B) {
	balancer := &LeastLoadedBalancer{}
	members := generateMembers(100)
	tolerance := 3
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.Pick(members, tolerance)
	}
}

func BenchmarkLeastLoaded_10000_Members(b *testing.B) {
	// O(N log N) - This should get slower as N grows
	balancer := &LeastLoadedBalancer{}
	members := generateMembers(10000)
	tolerance := 3
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.Pick(members, tolerance)
	}
}

func BenchmarkP2C_100_Members(b *testing.B) {
	balancer := &PowerOfTwoChoicesBalancer{}
	members := generateMembers(100)
	tolerance := 3
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.Pick(members, tolerance)
	}
}

func BenchmarkP2C_10000_Members(b *testing.B) {
	// O(1) - This should remain fast regardless of N
	balancer := &PowerOfTwoChoicesBalancer{}
	members := generateMembers(10000)
	tolerance := 3
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		balancer.Pick(members, tolerance)
	}
}
