// Package admin provides the admin API and dashboard
package admin

import (
	"encoding/json"
	"net/http"

	"github.com/DockRouter/dockrouter/internal/metrics"
)

// APIHandler handles REST API requests
type APIHandler struct {
	metricsCollector *metrics.Collector
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(metricsCollector *metrics.Collector) *APIHandler {
	return &APIHandler{
		metricsCollector: metricsCollector,
	}
}

// Routes returns the API routes
func (h *APIHandler) Routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/v1/status":       h.status,
		"/api/v1/routes":       h.routes,
		"/api/v1/containers":   h.containers,
		"/api/v1/certificates": h.certificates,
		"/api/v1/metrics":      h.metrics,
		"/api/v1/health":       h.health,
	}
}

func (h *APIHandler) status(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
	})
}

func (h *APIHandler) routes(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode([]interface{}{})
}

func (h *APIHandler) containers(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode([]interface{}{})
}

func (h *APIHandler) certificates(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode([]interface{}{})
}

func (h *APIHandler) metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	if h.metricsCollector != nil {
		h.metricsCollector.PrometheusFormat(w)
	}
}

func (h *APIHandler) health(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("OK"))
}
