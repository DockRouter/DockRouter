// Package proxy handles reverse proxying
package proxy

import (
	"testing"
)

func TestBufferPool(t *testing.T) {
	pool := newBufferPool()

	// Get a buffer
	buf := pool.Get()
	if buf == nil {
		t.Error("Buffer should not be nil")
	}
	if cap(buf) != 32*1024 {
		t.Errorf("Buffer capacity = %d, want %d", cap(buf), 32*1024)
	}

	// Put it back
	pool.Put(buf)

	// Get again - should reuse
	buf2 := pool.Get()
	if buf2 == nil {
		t.Error("Second buffer should not be nil")
	}

	pool.Put(buf2)
}

func TestBufferPoolMultiple(t *testing.T) {
	pool := newBufferPool()

	buffers := make([][]byte, 5)
	for i := 0; i < 5; i++ {
		buffers[i] = pool.Get()
	}

	for i, buf := range buffers {
		if buf == nil {
			t.Errorf("Buffer %d should not be nil", i)
		}
	}

	for _, buf := range buffers {
		pool.Put(buf)
	}
}
