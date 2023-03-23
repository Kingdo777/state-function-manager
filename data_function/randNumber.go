package data_function

import (
	"math/rand"
	"sync"
	"time"
)

const FirstShmKey int64 = 0x77770000

const ShmKeyMaxCount int64 = 0xFFFF

type safeRand struct {
	mu sync.Mutex
	r  *rand.Rand
}

func newSafeRand() *safeRand {
	return &safeRand{
		r: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (sr *safeRand) ShmKeyGen() int64 {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return sr.r.Int63n(ShmKeyMaxCount) + FirstShmKey
}
