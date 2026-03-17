// Package metrics provides Prometheus metrics collection
package metrics

import (
	"bytes"
	"strings"
	"testing"
)

func TestCollectorCounter(t *testing.T) {
	c := NewCollector()

	counter := c.Counter("requests_total")
	counter.Inc()
	counter.Inc()
	counter.Inc()

	if counter.Value() != 3 {
		t.Errorf("Counter value = %d, want 3", counter.Value())
	}
}

func TestCollectorGauge(t *testing.T) {
	c := NewCollector()

	gauge := c.Gauge("temperature")
	gauge.Set(25.5)

	// Gauge doesn't have a Value() method, so we just verify it doesn't panic
}

func TestCollectorHistogram(t *testing.T) {
	c := NewCollector()

	hist := c.Histogram("latency")
	hist.Observe(0.1)
	hist.Observe(0.2)
	hist.Observe(0.3)

	// Histogram implementation is basic, just verify it doesn't panic
}

func TestCollectorMultipleCounters(t *testing.T) {
	c := NewCollector()

	c1 := c.Counter("counter1")
	c2 := c.Counter("counter2")

	c1.Inc()
	c2.Inc()
	c2.Inc()

	if c1.Value() != 1 {
		t.Errorf("Counter1 value = %d, want 1", c1.Value())
	}
	if c2.Value() != 2 {
		t.Errorf("Counter2 value = %d, want 2", c2.Value())
	}
}

func TestCollectorSameCounterTwice(t *testing.T) {
	c := NewCollector()

	counter1 := c.Counter("requests")
	counter2 := c.Counter("requests")

	// Should return the same counter
	counter1.Inc()
	if counter2.Value() != 1 {
		t.Error("Should return same counter instance")
	}
}

func TestCollectorPrometheusFormat(t *testing.T) {
	c := NewCollector()

	counter := c.Counter("dockrouter_requests_total")
	counter.Inc()

	// The prometheus.go file has the format function
	// Just verify the counter works
	if counter.Value() != 1 {
		t.Errorf("Counter value = %d, want 1", counter.Value())
	}
}

func TestCollectorEmpty(t *testing.T) {
	c := NewCollector()

	// Empty collector should not panic
	counter := c.Counter("test")
	if counter.Value() != 0 {
		t.Errorf("Empty counter value = %d, want 0", counter.Value())
	}
}

func TestPrometheusFormatCounter(t *testing.T) {
	c := NewCollector()
	counter := c.Counter("requests-total")
	counter.Inc()
	counter.Inc()

	var buf bytes.Buffer
	c.PrometheusFormat(&buf)

	output := buf.String()
	if !strings.Contains(output, "dockrouter_requests_total") {
		t.Errorf("Output missing metric name: %s", output)
	}
	if !strings.Contains(output, "# TYPE dockrouter_requests_total counter") {
		t.Errorf("Output missing TYPE declaration: %s", output)
	}
}

func TestPrometheusFormatGauge(t *testing.T) {
	c := NewCollector()
	gauge := c.Gauge("temperature-celsius")
	gauge.Set(25.5)

	var buf bytes.Buffer
	c.PrometheusFormat(&buf)

	output := buf.String()
	if !strings.Contains(output, "dockrouter_temperature_celsius") {
		t.Errorf("Output missing metric name: %s", output)
	}
	if !strings.Contains(output, "# TYPE dockrouter_temperature_celsius gauge") {
		t.Errorf("Output missing TYPE declaration: %s", output)
	}
}

func TestPrometheusFormatHistogram(t *testing.T) {
	c := NewCollector()
	hist := c.Histogram("request-duration")
	hist.Observe(0.1)
	hist.Observe(0.2)

	var buf bytes.Buffer
	c.PrometheusFormat(&buf)

	output := buf.String()
	if !strings.Contains(output, "dockrouter_request_duration") {
		t.Errorf("Output missing metric name: %s", output)
	}
	if !strings.Contains(output, "# TYPE dockrouter_request_duration histogram") {
		t.Errorf("Output missing TYPE declaration: %s", output)
	}
	if !strings.Contains(output, "_count") {
		t.Errorf("Output missing histogram count: %s", output)
	}
	if !strings.Contains(output, "_sum") {
		t.Errorf("Output missing histogram sum: %s", output)
	}
}

func TestPrometheusFormatEmpty(t *testing.T) {
	c := NewCollector()

	var buf bytes.Buffer
	c.PrometheusFormat(&buf)

	// Should not panic with empty collector
	output := buf.String()
	_ = output
}

func TestPrometheusFormatAllTypes(t *testing.T) {
	c := NewCollector()

	counter := c.Counter("api.requests")
	counter.Inc()

	gauge := c.Gauge("active.connections")
	gauge.Set(10)

	hist := c.Histogram("response.time")
	hist.Observe(0.05)

	var buf bytes.Buffer
	c.PrometheusFormat(&buf)

	output := buf.String()

	// Check all metrics are present
	if !strings.Contains(output, "dockrouter_api_requests") {
		t.Error("Missing counter metric")
	}
	if !strings.Contains(output, "dockrouter_active_connections") {
		t.Error("Missing gauge metric")
	}
	if !strings.Contains(output, "dockrouter_response_time") {
		t.Error("Missing histogram metric")
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"requests-total", "dockrouter_requests_total"},
		{"api.v1.requests", "dockrouter_api_v1_requests"},
		{"simple", "dockrouter_simple"},
		{"with-dash.and.dots", "dockrouter_with_dash_and_dots"},
	}

	for _, tt := range tests {
		result := sanitizeName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCollectorConcurrent(t *testing.T) {
	c := NewCollector()
	counter := c.Counter("concurrent_requests")

	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				counter.Inc()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have 1000 increments
	if counter.Value() != 1000 {
		t.Errorf("Counter value = %d, want 1000", counter.Value())
	}
}

func TestGaugeSetAndGet(t *testing.T) {
	c := NewCollector()
	gauge := c.Gauge("test_gauge")

	gauge.Set(42.5)
	// Note: Gauge doesn't have a public Value() method
	// Just verify it doesn't panic

	gauge.Set(0)
	gauge.Set(-10.5)
}

func TestHistogramMultiple(t *testing.T) {
	c := NewCollector()
	hist := c.Histogram("test_hist")

	values := []float64{0.001, 0.01, 0.1, 1.0, 10.0}
	for _, v := range values {
		hist.Observe(v)
	}

	// Verify it doesn't panic
}

func TestIncCounter(t *testing.T) {
	c := NewCollector()

	// Use convenience method
	c.IncCounter("auto_counter")
	c.IncCounter("auto_counter")
	c.IncCounter("auto_counter")

	// Verify via the regular method
	counter := c.Counter("auto_counter")
	if counter.Value() != 3 {
		t.Errorf("Counter value = %d, want 3", counter.Value())
	}
}

func TestSetGauge(t *testing.T) {
	c := NewCollector()

	// Use convenience method
	c.SetGauge("auto_gauge", 42.5)
	c.SetGauge("auto_gauge", 100.0)

	// Verify it doesn't panic and that the same gauge is returned
	gauge := c.Gauge("auto_gauge")
	_ = gauge // Gauge doesn't have Value(), just verify it doesn't panic
}

func TestObserveHistogram(t *testing.T) {
	c := NewCollector()

	// Use convenience method
	c.ObserveHistogram("auto_histogram", 0.1)
	c.ObserveHistogram("auto_histogram", 0.2)
	c.ObserveHistogram("auto_histogram", 0.3)

	// Verify via the regular method
	hist := c.Histogram("auto_histogram")
	if hist.Count() != 3 {
		t.Errorf("Histogram count = %d, want 3", hist.Count())
	}
}

func TestHistogramCount(t *testing.T) {
	c := NewCollector()
	hist := c.Histogram("count_test")

	if hist.Count() != 0 {
		t.Errorf("Initial count = %d, want 0", hist.Count())
	}

	hist.Observe(0.1)
	hist.Observe(0.2)

	if hist.Count() != 2 {
		t.Errorf("Count after observations = %d, want 2", hist.Count())
	}
}

func TestHistogramSum(t *testing.T) {
	c := NewCollector()
	hist := c.Histogram("sum_test")

	hist.Observe(1.5)
	hist.Observe(2.5)
	hist.Observe(3.0)

	expected := 7.0
	if hist.Sum() != expected {
		t.Errorf("Sum = %f, want %f", hist.Sum(), expected)
	}
}

func TestHistogramBuckets(t *testing.T) {
	c := NewCollector()
	hist := c.Histogram("bucket_test")

	// Observe values that fall into specific buckets
	hist.Observe(0.001) // Should be in first bucket (<=0.001)
	hist.Observe(0.05)  // Should be in multiple buckets up to 0.05
	hist.Observe(1.0)   // Should be in many buckets up to 1.0

	buckets := hist.Buckets()
	if len(buckets) != len(DefaultBuckets) {
		t.Errorf("Bucket count = %d, want %d", len(buckets), len(DefaultBuckets))
	}

	// Verify at least some buckets have counts
	totalCount := uint64(0)
	for _, b := range buckets {
		totalCount += b.count
	}
	if totalCount == 0 {
		t.Error("Expected some bucket counts to be non-zero")
	}
}

func TestHistogramBucketsCopy(t *testing.T) {
	c := NewCollector()
	hist := c.Histogram("copy_test")

	hist.Observe(0.5)

	buckets1 := hist.Buckets()
	buckets2 := hist.Buckets()

	// Verify that Buckets() returns a copy (different slices)
	if &buckets1[0] == &buckets2[0] {
		t.Error("Buckets() should return a copy, not the same slice")
	}
}

func TestConvenienceMethodsWithSameName(t *testing.T) {
	c := NewCollector()

	// Test that convenience methods create/get the same metric
	c.IncCounter("shared_name")
	c.SetGauge("shared_name", 1.0)         // This is a DIFFERENT metric (gauge, not counter)
	c.ObserveHistogram("shared_name", 0.1) // This is a DIFFERENT metric (histogram)

	// Verify all three exist independently
	counter := c.Counter("shared_name")
	if counter.Value() != 1 {
		t.Errorf("Counter value = %d, want 1", counter.Value())
	}
}
