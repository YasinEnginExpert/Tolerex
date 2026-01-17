package balancer

import (
	"math/rand"
	"sort"
)

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

// PowerOfTwoChoicesBalancer: "The Power of Two Choices" algoritması.
// N eleman arasından rastgele 2 tane seçer, yükü az olanı alır.
// Büyük ölçekli sistemlerde tüm listeyi sıralamak (sort) pahalı olduğunda kullanılır.
type PowerOfTwoChoicesBalancer struct{}

func (b *PowerOfTwoChoicesBalancer) Pick(candidates []MemberLoad, tolerance int) []MemberLoad {
	if tolerance <= 0 || len(candidates) == 0 {
		return nil
	}

	// Yeterli aday yoksa veya tam sayı isteniyorsa hepsini dön
	if len(candidates) <= tolerance {
		return candidates
	}

	out := make([]MemberLoad, 0, tolerance)

	// Aday havuzunun kopyası üzerinde çalışalım ki orijinali bozulmasın
	pool := make([]MemberLoad, len(candidates))
	copy(pool, candidates)

	// tolerance kadar seçim yapacağız
	for i := 0; i < tolerance; i++ {
		// Havuzda tek eleman kaldıysa mecbur onu seç
		if len(pool) == 1 {
			out = append(out, pool[0])
			break
		}

		// Rastgele 2 indeks seç
		idx1 := rand.Intn(len(pool))
		idx2 := rand.Intn(len(pool))

		// Eğer aynı indeksi seçtiysek, farklı olana kadar dene (retry limit koymak iyi olabilir ama len>1 ise sorun olmaz)
		for idx1 == idx2 {
			idx2 = rand.Intn(len(pool))
		}

		// Karşılaştır
		winnerIdx := idx1
		if pool[idx2].Count < pool[idx1].Count {
			winnerIdx = idx2
		}

		// Kazananı listeye ekle
		out = append(out, pool[winnerIdx])

		// Kazananı havuzdan çıkar (swap with last & shrink)
		lastIdx := len(pool) - 1
		pool[winnerIdx] = pool[lastIdx]
		pool = pool[:lastIdx]
	}

	return out
}
