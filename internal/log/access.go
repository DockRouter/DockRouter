// Package log provides structured logging
package log

import (
	"net/http"
	"time"
)

// AccessLogEntry represents an HTTP access log entry
type AccessLogEntry struct {
	Timestamp   string  `json:"ts"`
	Method      string  `json:"method"`
	Host        string  `json:"host"`
	Path        string  `json:"path"`
	Query       string  `json:"query,omitempty"`
	Status      int     `json:"status"`
	DurationMs  float64 `json:"duration_ms"`
	ClientIP    string  `json:"client_ip"`
	RequestID   string  `json:"request_id,omitempty"`
	Backend     string  `json:"backend,omitempty"`
	ContainerID string  `json:"container,omitempty"`
	UserAgent   string  `json:"user_agent,omitempty"`
}

// AccessLogger logs HTTP access
func (l *Logger) AccessLog(r *http.Request, status int, duration time.Duration, backend string) {
	entry := AccessLogEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Method:     r.Method,
		Host:       r.Host,
		Path:       r.URL.Path,
		Query:      r.URL.RawQuery,
		Status:     status,
		DurationMs: float64(duration.Microseconds()) / 1000,
		ClientIP:   r.RemoteAddr,
		RequestID:  r.Header.Get("X-Request-Id"),
		Backend:    backend,
		UserAgent:  r.Header.Get("User-Agent"),
	}

	l.Info("request completed",
		"method", entry.Method,
		"host", entry.Host,
		"path", entry.Path,
		"status", entry.Status,
		"duration_ms", entry.DurationMs,
		"client_ip", entry.ClientIP,
		"request_id", entry.RequestID,
		"backend", entry.Backend,
	)
}
