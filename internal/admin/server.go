// Package admin provides the admin API and dashboard
package admin

import (
	"context"
	"net/http"
	"time"
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
		Addr:              s.addr,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	return srv.ListenAndServe()
}
