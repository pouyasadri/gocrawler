package frontier

import "sync"

type item struct {
	url   string
	depth int
}

type Frontier struct {
	mu    sync.Mutex
	queue []item
}

func New() *Frontier {
	return &Frontier{
		queue: make([]item, 0, 1024),
	}
}

func (f *Frontier) Enqueue(url string, depth int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, item{url: url, depth: depth})
}

func (f *Frontier) Pop() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queue) == 0 {
		return "", false
	}
	it := f.queue[0]
	//f.queue = f.queue[1:] keeps a reference to the underlying array, which can prevent GC of removed elements in long-running programs.
	//To avoid that, set the popped slot to the zero value before slicing (e.g. f.queue[0] = item{})
	//or use a ring buffer / index-based queue to avoid repeated allocations and enable GC.
	f.queue = f.queue[1:]
	return it.url, true
}
