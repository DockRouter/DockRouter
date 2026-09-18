package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// countingBackend serves 200 and counts how often it was probed.
func countingBackend(t *testing.T, hits *int64) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

// dr.healthcheck.interval is documented per container. Before the scheduler
// honoured it, every target was probed on the checker's global tick and the
// label had no effect at all.
func TestCheckerHonoursPerTargetInterval(t *testing.T) {
	var fastHits, slowHits int64
	fast := countingBackend(t, &fastHits)
	slow := countingBackend(t, &slowHits)

	c := NewChecker(20*time.Millisecond, time.Second)
	c.Register(fast, HealthCheck{Path: "/", Interval: 20 * time.Millisecond})
	c.Register(slow, HealthCheck{Path: "/", Interval: 10 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	go c.Start(ctx)
	time.Sleep(300 * time.Millisecond)
	cancel()

	gotFast := atomic.LoadInt64(&fastHits)
	gotSlow := atomic.LoadInt64(&slowHits)

	if gotFast < 3 {
		t.Errorf("fast target probed %d times, want at least 3", gotFast)
	}
	if gotSlow > 1 {
		t.Errorf("target with a 10s interval probed %d times in 300ms, want at most 1", gotSlow)
	}
}

// A target that declares no interval keeps using the checker's default.
func TestCheckerDefaultsToCheckerInterval(t *testing.T) {
	var hits int64
	target := countingBackend(t, &hits)

	c := NewChecker(20*time.Millisecond, time.Second)
	c.Register(target, HealthCheck{Path: "/"})

	ctx, cancel := context.WithCancel(context.Background())
	go c.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()

	if got := atomic.LoadInt64(&hits); got < 2 {
		t.Errorf("target probed %d times, want at least 2", got)
	}
}
