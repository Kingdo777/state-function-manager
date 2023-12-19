package state_function

import "sync"

type KeyGenerator struct {
	min   int
	max   int
	next  int
	mutex sync.Mutex
	pool  []int
}

func NewKeyGenerator(min, max int) *KeyGenerator {
	return &KeyGenerator{
		min:  min,
		max:  max,
		next: min,
		pool: make([]int, 0, max-min+1),
	}
}

func (g *KeyGenerator) GetKey() (int, bool) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	if len(g.pool) > 0 {
		key := g.pool[len(g.pool)-1]
		g.pool = g.pool[:len(g.pool)-1]
		return key, true
	}

	if g.next > g.max {
		return 0, false
	}

	key := g.next
	g.next++
	return key, true
}

func (g *KeyGenerator) ReturnKey(key int) bool {
	if key < g.min || key > g.max {
		return false
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()

	for _, k := range g.pool {
		if k == key {
			return false
		}
	}

	g.pool = append(g.pool, key)
	return true
}
