package main

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DockRouter/dockrouter/internal/admin"
	"github.com/DockRouter/dockrouter/internal/config"
	"github.com/DockRouter/dockrouter/internal/health"
	"github.com/DockRouter/dockrouter/internal/log"
	"github.com/DockRouter/dockrouter/internal/metrics"
	"github.com/DockRouter/dockrouter/internal/router"
	tlspkg "github.com/DockRouter/dockrouter/internal/tls"
)

func newEventsApp(t *testing.T) *App {
	t.Helper()
	return &App{
		logger:          log.NewLogger(nil, log.LevelInfo),
		config:          &config.Config{Admin: true, DefaultTLS: "off"},
		routeTable:      router.NewTable(),
		challengeSolver: tlspkg.NewChallengeSolver(),
		healthChecker:   health.NewChecker(10*time.Second, 5*time.Second),
		metrics:         metrics.NewCollector(),
		events:          admin.NewSSEHub(),
		startTime:       time.Now(),
	}
}

// The dashboard has always opened /api/v1/events. Until it was registered the
// request fell through to the dashboard catch-all and came back as HTML, so
// EventSource failed and live updates never worked.
func TestAdminHandlerServesEventStream(t *testing.T) {
	app := newEventsApp(t)
	go app.events.Run()
	defer app.events.Stop()

	srv := httptest.NewServer(app.buildAdminHandler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("requesting event stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// The stream must actually carry published events through to the client.
	go func() {
		for i := 0; i < 20; i++ {
			app.publishEvent("route.added", map[string]string{"host": "app.example.com"})
			time.Sleep(25 * time.Millisecond)
		}
	}()

	reader := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading event stream: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			if !strings.Contains(line, "route.added") {
				t.Fatalf("unexpected event payload: %q", line)
			}
			if !strings.Contains(line, "app.example.com") {
				t.Fatalf("event lost its data: %q", line)
			}
			return
		}
	}
	t.Fatal("no event arrived on the stream")
}

// The admin server must not set a WriteTimeout: it is a wall-clock deadline for
// the whole exchange and would cut the event stream off mid-flight.
func TestAdminServerHasNoWriteTimeout(t *testing.T) {
	app := newEventsApp(t)
	app.config.AdminBind = "127.0.0.1"
	app.config.AdminPort = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.start(ctx)

	if app.adminServer == nil {
		t.Fatal("admin server was not created")
	}
	if app.adminServer.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 so SSE streams survive", app.adminServer.WriteTimeout)
	}
	if app.adminServer.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout should still bound slow request headers")
	}
}

// publishEvent is called from the route sink, which runs whether or not the
// hub exists. It must never panic on a nil hub.
func TestPublishEventWithoutHub(t *testing.T) {
	app := newEventsApp(t)
	app.events = nil
	app.publishEvent("route.added", nil)
}

// With no hub the endpoint reports that rather than serving an empty stream.
func TestHandleEventsWithoutHub(t *testing.T) {
	app := newEventsApp(t)
	app.events = nil

	w := httptest.NewRecorder()
	app.handleEvents(w, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// A listener that cannot bind used to be logged and then ignored, leaving a
// process that answered the health probe while serving no traffic. It now
// surfaces on serverErr so main can bring the process down.
func TestStartReportsListenerFailure(t *testing.T) {
	// Occupy a port so the HTTP listener is guaranteed to fail.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port

	app := newEventsApp(t)
	app.config.Admin = false
	app.config.HTTPPort = port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.start(ctx)
	defer app.events.Stop()

	select {
	case err := <-app.serverErr:
		if err == nil {
			t.Fatal("serverErr delivered a nil error")
		}
		if !strings.Contains(err.Error(), "http server") {
			t.Errorf("error = %v, want it to name the http server", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a listener that could not bind never reported an error")
	}
}

// serveErr must never block the goroutine that hit the error, even if nothing
// is draining the channel and several servers fail at once.
func TestServeErrNeverBlocks(t *testing.T) {
	app := newEventsApp(t)
	app.serverErr = make(chan error, 1)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			app.serveErr(errors.New("boom"))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("serveErr blocked on a full channel")
	}
}

// Before start() runs there is no channel to send on; that must not panic.
func TestServeErrBeforeStart(t *testing.T) {
	app := newEventsApp(t)
	app.serveErr(errors.New("boom"))
}
