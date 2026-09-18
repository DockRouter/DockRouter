// Package admin provides the admin API and dashboard
package admin

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// sseKeepaliveInterval is how often an idle event stream emits a comment line
// so intermediaries do not treat the connection as dead.
const sseKeepaliveInterval = 25 * time.Second

// SSEHub manages Server-Sent Events connections
type SSEHub struct {
	mu         sync.RWMutex
	clients    map[*sseClient]struct{}
	broadcast  chan Event
	register   chan *sseClient
	unregister chan *sseClient
	done       chan struct{}
	stopOnce   sync.Once
	dropped    atomic.Int64
}

type sseClient struct {
	ch    chan Event
	flush chan struct{}
}

// Event represents an SSE event
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// NewSSEHub creates a new SSE hub
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients:    make(map[*sseClient]struct{}),
		broadcast:  make(chan Event, 100),
		register:   make(chan *sseClient),
		unregister: make(chan *sseClient),
		done:       make(chan struct{}),
	}
}

// Run starts the SSE hub. It blocks until Stop is called.
func (h *SSEHub) Run() {
	for {
		select {
		case <-h.done:
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.ch)
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.ch <- event:
				default:
					// Client too slow, drop event
					h.dropped.Add(1)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Stop shuts down the SSE hub and closes all client channels. It is safe to
// call more than once, which matters now that it sits on a shutdown path that
// can be reached from both a signal and a fatal server error.
func (h *SSEHub) Stop() {
	h.stopOnce.Do(func() {
		close(h.done)

		// Let Run() handle client cleanup via the unregister path
		// Just clear the map to prevent new registrations
		h.mu.Lock()
		for client := range h.clients {
			close(client.ch)
			delete(h.clients, client)
		}
		h.mu.Unlock()
	})
}

// Send broadcasts an event to all clients
func (h *SSEHub) Send(event Event) {
	select {
	case h.broadcast <- event:
	default:
		// Broadcast channel full, drop event
		h.dropped.Add(1)
	}
}

// Handler returns an HTTP handler for SSE connections
func (h *SSEHub) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// Proxies in front of the dashboard buffer streamed responses unless
		// told not to, which makes events arrive in bursts or not at all.
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		client := &sseClient{
			ch:    make(chan Event, 10),
			flush: make(chan struct{}),
		}

		select {
		case h.register <- client:
		case <-h.done:
			return
		case <-r.Context().Done():
			return
		}
		defer func() {
			select {
			case h.unregister <- client:
			case <-h.done:
			case <-r.Context().Done():
			}
		}()

		// An idle event stream sends nothing for as long as the cluster is
		// quiet, and an intermediary will eventually close it. A comment line
		// keeps the connection alive and is ignored by EventSource.
		keepalive := time.NewTicker(sseKeepaliveInterval)
		defer keepalive.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-h.done:
				return
			case <-keepalive.C:
				if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
					return
				}
				flusher.Flush()
			case event, ok := <-client.ch:
				if !ok {
					// Channel closed, client was unregistered
					return
				}
				// Write SSE format with JSON encoded event
				w.Write([]byte("data: "))
				data, err := json.Marshal(event)
				if err != nil {
					continue
				}
				w.Write(data)
				w.Write([]byte("\n\n"))
				flusher.Flush()
			}
		}
	}
}
