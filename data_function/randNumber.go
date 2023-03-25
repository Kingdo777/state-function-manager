package data_function

import (
	"math/rand"
	"sync"
	"time"
)

const FirstShmKey int = 0x77770000

const ShmKeyMaxCount int = 0xFFFF

type safeRand struct {
	mu sync.Mutex
	r  *rand.Rand
}

func newSafeRand() *safeRand {
	return &safeRand{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (sr *safeRand) ShmKeyGen() int {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return sr.r.Intn(ShmKeyMaxCount) + FirstShmKey
}
