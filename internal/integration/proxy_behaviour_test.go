package integration

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DockRouter/dockrouter/internal/health"
	"github.com/DockRouter/dockrouter/internal/log"
	"github.com/DockRouter/dockrouter/internal/middleware"
	"github.com/DockRouter/dockrouter/internal/proxy"
	"github.com/DockRouter/dockrouter/internal/router"
)

func newLogger() *log.Logger { return log.NewLogger(io.Discard, log.LevelError) }

func newTestContext(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

func addrOf(ts *httptest.Server) string { return strings.TrimPrefix(ts.URL, "http://") }

// register mirrors what cmd/dockrouter's appRouteSink.AddRoute does.
func register(t *router.Table, containerID, host, addr string, lb router.LoadBalanceStrategy) {
	pool := router.NewBackendPool(lb)
	route := &router.Route{
		ID: router.RouteKey(host, "/"), Host: host, PathPrefix: "/", Backend: pool,
		ContainerID: containerID, ContainerName: containerID, CreatedAt: time.Now(),
	}
	t.Upsert(route, &router.BackendTarget{
		Address: addr, ContainerID: containerID, Weight: 1, Healthy: true,
	})
}

// productionChain mirrors App.buildMiddlewareChain in cmd/dockrouter/main.go.
func productionChain(h http.Handler) http.Handler {
	chain := middleware.Chain(middleware.Recovery, middleware.RequestID)
	chain = middleware.Chain(chain, middleware.AccessLog)
	chain = middleware.Chain(chain, middleware.Timeout(30*time.Second))
	chain = middleware.Chain(chain, middleware.SecurityHeaders)
	return chain(h)
}

func frontend(t *router.Table) *httptest.Server {
	rt := router.NewRouter(t, proxy.NewProxy(newLogger()), newLogger())
	return httptest.NewServer(productionChain(rt))
}

// ---------------------------------------------------------------
// WebSocket proxying, through the full production middleware chain.
// ---------------------------------------------------------------
func TestWebSocketThroughFullChain(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		buf.Flush()
		// Echo one frame so we can prove bytes really flow both ways.
		line, _ := buf.ReadString('\n')
		buf.WriteString("echo:" + line)
		buf.Flush()
		time.Sleep(150 * time.Millisecond)
	}))
	defer backend.Close()

	table := router.NewTable()
	register(table, "ws1", "ws.example.com", addrOf(backend), router.RoundRobin)
	front := frontend(table)
	defer front.Close()

	conn, err := net.Dial("tcp", addrOf(front))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	fmt.Fprintf(conn, "GET /socket HTTP/1.1\r\nHost: ws.example.com\r\n"+
		"Upgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n")

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("upgrade failed: got %q, want 101 Switching Protocols", strings.TrimSpace(status))
	}
	// Drain the remaining handshake headers.
	for {
		line, err := br.ReadString('\n')
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}

	// Prove the tunnel carries data after the upgrade.
	fmt.Fprintf(conn, "ping\n")
	echo, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if strings.TrimSpace(echo) != "echo:ping" {
		t.Errorf("tunnel did not relay data: got %q, want %q", strings.TrimSpace(echo), "echo:ping")
	}

	// A WebSocket request must not knock the backend out of rotation.
	if r := table.Match("ws.example.com", "/socket"); r != nil && r.Backend.HealthyCount() == 0 {
		t.Error("a WebSocket request marked the backend unhealthy")
	}
}

// ---------------------------------------------------------------
// SSE / streaming, through the full production middleware chain.
// ---------------------------------------------------------------
func TestStreamingThroughFullChain(t *testing.T) {
	const delay = 700 * time.Millisecond
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		time.Sleep(delay)
		w.Write([]byte("data: second\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer backend.Close()

	table := router.NewTable()
	register(table, "sse", "sse.example.com", addrOf(backend), router.RoundRobin)
	front := frontend(table)
	defer front.Close()

	req, _ := http.NewRequest("GET", front.URL+"/", nil)
	req.Host = "sse.example.com"
	req.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	firstByte := time.Since(start)

	if firstByte >= delay {
		t.Errorf("first SSE event arrived after %v; response is being buffered instead of streamed",
			firstByte.Round(time.Millisecond))
	}
	if got := string(buf[:n]); !strings.Contains(got, "first") {
		t.Errorf("first read = %q, want the first event", got)
	}
	t.Logf("first event in %v: %q", firstByte.Round(time.Millisecond), buf[:n])
}

// ---------------------------------------------------------------
// Replicas of one service share a route and are load balanced.
// ---------------------------------------------------------------
func TestReplicasAreLoadBalanced(t *testing.T) {
	mk := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(name))
		}))
	}
	b1, b2, b3 := mk("replica-1"), mk("replica-2"), mk("replica-3")
	defer b1.Close()
	defer b2.Close()
	defer b3.Close()

	table := router.NewTable()
	for i, b := range []*httptest.Server{b1, b2, b3} {
		register(table, fmt.Sprintf("c%d", i+1), "app.example.com", addrOf(b), router.RoundRobin)
	}
	front := frontend(table)
	defer front.Close()

	seen := map[string]int{}
	for i := 0; i < 30; i++ {
		req, _ := http.NewRequest("GET", front.URL+"/", nil)
		req.Host = "app.example.com"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("req %d: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		seen[string(body)]++
	}

	if len(seen) != 3 {
		t.Errorf("3 replicas registered but %d received traffic: %v", len(seen), seen)
	}
	for name, n := range seen {
		if n != 10 {
			t.Errorf("round-robin uneven: %s got %d of 30, want 10", name, n)
		}
	}
	t.Logf("distribution: %v (table holds %d route(s))", seen, table.Count())
}

// ---------------------------------------------------------------
// Scaling down one replica must not take the service offline.
// ---------------------------------------------------------------
func TestScaleDownKeepsServiceUp(t *testing.T) {
	b1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("replica-1"))
	}))
	b2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("replica-2"))
	}))
	defer b1.Close()
	defer b2.Close()

	table := router.NewTable()
	register(table, "c1", "app.example.com", addrOf(b1), router.RoundRobin)
	register(table, "c2", "app.example.com", addrOf(b2), router.RoundRobin)

	front := frontend(table)
	defer front.Close()

	// Replica c1 stops.
	table.RemoveByContainer("c1")

	route := table.Match("app.example.com", "/")
	if route == nil {
		t.Fatal("stopping one replica removed the whole route; surviving replica is unreachable")
	}

	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", front.URL+"/", nil)
		req.Host = "app.example.com"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("req %d: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != "replica-2" {
			t.Errorf("request %d served by %q, want the surviving replica-2", i, body)
		}
	}

	// The last replica going away does remove the route.
	table.RemoveByContainer("c2")
	if table.Match("app.example.com", "/") != nil {
		t.Error("route survived after its last backend was removed")
	}
}

// ---------------------------------------------------------------
// Failover replays the request body against the next backend.
// ---------------------------------------------------------------
func TestFailoverPreservesRequestBody(t *testing.T) {
	var hits int32
	var lastBody atomic.Value
	lastBody.Store("")

	// A backend that accepts the connection, reads the body, then dies.
	broken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer broken.Close()
	go func() {
		for {
			c, err := broken.Accept()
			if err != nil {
				return
			}
			io.CopyN(io.Discard, c, 64) // consume part of the request
			c.Close()                   // then fail
		}
	}()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		b, _ := io.ReadAll(r.Body)
		lastBody.Store(string(b))
		w.Write([]byte("ok"))
	}))
	defer good.Close()

	pool := router.NewBackendPool(router.RoundRobin)
	table := router.NewTable()
	table.Upsert(&router.Route{
		ID: router.RouteKey("retry.example.com", "/"), Host: "retry.example.com",
		PathPrefix: "/", Backend: pool, CreatedAt: time.Now(),
	}, &router.BackendTarget{Address: broken.Addr().String(), ContainerID: "broken", Weight: 1, Healthy: true})
	table.Upsert(&router.Route{
		ID: router.RouteKey("retry.example.com", "/"), Host: "retry.example.com",
		PathPrefix: "/", Backend: pool, CreatedAt: time.Now(),
	}, &router.BackendTarget{Address: addrOf(good), ContainerID: "good", Weight: 1, Healthy: true})

	front := frontend(table)
	defer front.Close()

	payload := strings.Repeat("PAYLOAD-", 32)
	req, _ := http.NewRequest("POST", front.URL+"/", strings.NewReader(payload))
	req.Host = "retry.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("failover never reached the healthy backend")
	}
	if got := lastBody.Load().(string); got != payload {
		t.Errorf("healthy backend received %d bytes, want %d — request body was not replayed",
			len(got), len(payload))
	}
}

// ---------------------------------------------------------------
// A backend that fails and recovers returns to rotation.
// ---------------------------------------------------------------
func TestBackendRecoversAfterFailure(t *testing.T) {
	var up atomic.Bool
	up.Store(false)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	table := router.NewTable()
	register(table, "c1", "app.example.com", addrOf(backend), router.RoundRobin)

	// Wire the checker exactly as cmd/dockrouter does.
	checker := health.NewChecker(100*time.Millisecond, 500*time.Millisecond)
	checker.OnStateChange(func(target string, state health.HealthState) {
		table.SetBackendHealth(target, state == health.StateHealthy || state == health.StateDegraded)
	})
	checker.Register(addrOf(backend), health.HealthCheck{
		Path: "/", Interval: 100 * time.Millisecond,
		Timeout: 500 * time.Millisecond, Threshold: 2, Recovery: 1,
	})

	ctx, cancel := newTestContext(3 * time.Second)
	defer cancel()
	go checker.Start(ctx)

	// The backend is down: the checker should eject it.
	if !waitFor(2*time.Second, func() bool {
		r := table.Match("app.example.com", "/")
		return r != nil && r.Backend.HealthyCount() == 0
	}) {
		t.Fatal("health checker never took the failing backend out of rotation")
	}

	// The backend comes back: it must return to rotation on its own.
	up.Store(true)
	if !waitFor(2*time.Second, func() bool {
		r := table.Match("app.example.com", "/")
		return r != nil && r.Backend.HealthyCount() == 1
	}) {
		t.Fatal("recovered backend was never returned to rotation")
	}
}
