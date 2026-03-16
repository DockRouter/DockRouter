// Package admin provides the admin API and dashboard
package admin

import (
	"context"
	"net/http"
)

// Server runs the admin HTTP server
type Server struct {
	addr    string
	handler http.Handler
}

// NewServer creates a new admin server
func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		addr:    addr,
		handler: handler,
	}
}

// Start begins listening for admin requests
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.addr,
		Handler: s.handler,
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	return srv.ListenAndServe()
}
