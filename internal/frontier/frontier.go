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

func (f *Frontier) Pop() (string, int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queue) == 0 {
		return "", 0, false
	}
	it := f.queue[0]
	//f.queue = f.queue[1:] keeps a reference to the underlying array, which can prevent GC of removed elements in long-running programs.
	//To avoid that, set the popped slot to the zero value before slicing (e.g. f.queue[0] = item{})
	//or use a ring buffer / index-based queue to avoid repeated allocations and enable GC.
	// Allow GC to reclaim the item
	f.queue[0] = item{}
	f.queue = f.queue[1:]
	return it.url, it.depth, true
}

// Size returns the current number of items in the queue.
func (f *Frontier) Size() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queue)
}

// Peek returns up to n items from the front of the queue without removing them.
func (f *Frontier) Peek(n int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := n
	if count > len(f.queue) {
		count = len(f.queue)
	}
	urls := make([]string, 0, count)
	for i := 0; i < count; i++ {
		urls = append(urls, f.queue[i].url)
	}
	return urls
}
