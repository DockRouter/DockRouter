package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTimeout(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	timedHandler := Timeout(5 * time.Second)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	timedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestTimeoutWithDeadline(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Error("Expected deadline to be set on context")
			return
		}
		if time.Until(deadline) > 5*time.Second {
			t.Errorf("Deadline too far in the future: %v", time.Until(deadline))
		}
		w.WriteHeader(http.StatusOK)
	})

	timedHandler := Timeout(5 * time.Second)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	timedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestTimeoutCanceledContext(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Context should be done after timeout
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	})

	timedHandler := Timeout(50 * time.Millisecond)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	timedHandler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed < 40*time.Millisecond {
		t.Errorf("Handler returned too quickly: %v", elapsed)
	}
}
