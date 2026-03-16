// Package admin provides the admin API and dashboard
package admin

import (
	"net/http"
	"sync"
)

// SSEHub manages Server-Sent Events connections
type SSEHub struct {
	mu         sync.RWMutex
	clients    map[*sseClient]struct{}
	broadcast  chan Event
	register   chan *sseClient
	unregister chan *sseClient
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
	}
}

// Run starts the SSE hub
func (h *SSEHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			delete(h.clients, client)
			h.mu.Unlock()
			close(client.ch)

		case event := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.ch <- event:
				default:
					// Client too slow, drop event
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Send broadcasts an event to all clients
func (h *SSEHub) Send(event Event) {
	h.broadcast <- event
}

// Handler returns an HTTP handler for SSE connections
func (h *SSEHub) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		client := &sseClient{
			ch:    make(chan Event, 10),
			flush: make(chan struct{}),
		}

		h.register <- client
		defer func() { h.unregister <- client }()

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-client.ch:
				// Write SSE format
				w.Write([]byte("data: "))
				// TODO: JSON encode event - for now just send type
				w.Write([]byte(event.Type))
				w.Write([]byte("\n\n"))
				flusher.Flush()
			}
		}
	}
}
