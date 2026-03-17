// Package middleware provides HTTP middleware components
package middleware

import (
	"net/http"
	"time"
)

// MetricsCollector interface for recording metrics
type MetricsCollector interface {
	IncCounter(name string)
	ObserveHistogram(name string, value float64)
	SetGauge(name string, value float64)
}

// Metrics records HTTP request metrics
func Metrics(collector MetricsCollector) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status
			wrapped := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}

			// Increment active requests
			collector.IncCounter("http_requests_total")
			collector.SetGauge("http_requests_active", 1)

			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			collector.ObserveHistogram("http_request_duration_seconds", duration)
			collector.SetGauge("http_requests_active", 0)

			// Record status code
			if wrapped.status >= 400 {
				collector.IncCounter("http_errors_total")
			}
		})
	}
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
