package server

import "sort"

// MemberLoad: Leader'ın seçim yapması için ihtiyaç duyduğu "yük" snapshot'ı.
// (MemberInfo'nun tamamını taşımıyoruz; sadece seçim için gerekenler.)
type MemberLoad struct {
	Addr  string
	Count int // MessageCnt
}

// LoadBalancer: farklı seçim stratejilerini tak-çıkar yapabilmek için arayüz.
type LoadBalancer interface {
	// Pick: candidates içinden tolerance kadar hedef seçer.
	// candidates zaten "Alive=true" filtrelenmiş olmalı.
	Pick(candidates []MemberLoad, tolerance int) []MemberLoad
}

// LeastLoadedBalancer: mevcut leader.go içindeki mantığın taşınmış hali.
// "en az MessageCnt" olanları seçer.
type LeastLoadedBalancer struct{}

func (b *LeastLoadedBalancer) Pick(candidates []MemberLoad, tolerance int) []MemberLoad {
	if tolerance <= 0 || len(candidates) == 0 {
		return nil
	}

	// Deterministik sıralama:
	// 1) Count artan
	// 2) Count eşitse Addr alfabetik (tie-break)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Count != candidates[j].Count {
			return candidates[i].Count < candidates[j].Count
		}
		return candidates[i].Addr < candidates[j].Addr
	})

	if tolerance > len(candidates) {
		tolerance = len(candidates)
	}

	out := make([]MemberLoad, 0, tolerance)
	for i := 0; i < tolerance; i++ {
		out = append(out, candidates[i])
	}
	return out
}
