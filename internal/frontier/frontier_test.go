package frontier

import (
	"testing"
)

func TestFrontier(t *testing.T) {
	f := New()

	url1 := "http://example.com/1"
	url2 := "http://example.com/2"

	f.Enqueue(url1, 0)
	f.Enqueue(url2, 1)

	// Test Pop order
	u, d, ok := f.Pop()
	if !ok {
		t.Fatal("expected item, got none")
	}
	if u != url1 {
		t.Errorf("expected %s, got %s", url1, u)
	}
	if d != 0 {
		t.Errorf("expected depth 0, got %d", d)
	}

	u, d, ok = f.Pop()
	if !ok {
		t.Fatal("expected item, got none")
	}
	if u != url2 {
		t.Errorf("expected %s, got %s", url2, u)
	}
	if d != 1 {
		t.Errorf("expected depth 1, got %d", d)
	}

	// Test Empty
	_, _, ok = f.Pop()
	if ok {
		t.Error("expected empty queue, got item")
	}
}

func TestMemoryLeak(t *testing.T) {
	f := New()
	f.Enqueue("http://example.com", 0)
	f.Pop()

	// Check underlying array capacity vs length (heuristic, mostly checking if we crash or logic holds)
	// We can't easily check for memory leak in a unit test without complex heap analysis,
	// but we can ensure the logic doesn't panic and behaves correctly.
	if len(f.queue) != 0 {
		t.Errorf("expected queue length 0, got %d", len(f.queue))
	}
}
