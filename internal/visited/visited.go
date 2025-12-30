package visited

import "sync"

type InMemory struct {
	mu sync.Mutex
	m  map[string]struct{}
}

func NewInMemory() *InMemory {
	return &InMemory{
		m: make(map[string]struct{}),
	}
}

func (v *InMemory) Seen(url string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.m[url]
	return ok
}

func (v *InMemory) Mark(url string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m[url] = struct{}{}
}
