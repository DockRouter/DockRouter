// Package metrics provides Prometheus-compatible metrics collection
package metrics

import (
	"sync"
	"sync/atomic"
)

// Collector holds all metrics
type Collector struct {
	mu sync.RWMutex

	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

// Counter is a monotonically increasing value
type Counter struct {
	value uint64
}

// Gauge is a point-in-time value
type Gauge struct {
	value float64
}

// Histogram tracks distribution of values
type Histogram struct {
	mu      sync.RWMutex
	count   uint64
	sum     float64
	buckets []HistogramBucket
}

// HistogramBucket holds a single bucket
type HistogramBucket struct {
	upperBound float64
	count      uint64
}

// DefaultBuckets are default histogram buckets (in seconds for latency)
var DefaultBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

// Counter gets or creates a counter
func (c *Collector) Counter(name string) *Counter {
	c.mu.Lock()
	defer c.mu.Unlock()
	if counter, ok := c.counters[name]; ok {
		return counter
	}
	counter := &Counter{}
	c.counters[name] = counter
	return counter
}

// Inc increments the counter
func (c *Counter) Inc() {
	atomic.AddUint64(&c.value, 1)
}

// Value returns the counter value
func (c *Counter) Value() uint64 {
	return atomic.LoadUint64(&c.value)
}

// Gauge gets or creates a gauge
func (c *Collector) Gauge(name string) *Gauge {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gauge, ok := c.gauges[name]; ok {
		return gauge
	}
	gauge := &Gauge{}
	c.gauges[name] = gauge
	return gauge
}

// Set sets the gauge value
func (g *Gauge) Set(v float64) {
	g.value = v
}

// Histogram gets or creates a histogram
func (c *Collector) Histogram(name string) *Histogram {
	c.mu.Lock()
	defer c.mu.Unlock()
	if hist, ok := c.histograms[name]; ok {
		return hist
	}
	hist := &Histogram{
		buckets: make([]HistogramBucket, len(DefaultBuckets)),
	}
	for i, bound := range DefaultBuckets {
		hist.buckets[i].upperBound = bound
	}
	c.histograms[name] = hist
	return hist
}

// Observe records an observation
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.count++
	h.sum += v

	// Increment matching buckets
	for i := range h.buckets {
		if v <= h.buckets[i].upperBound {
			h.buckets[i].count++
		}
	}
}

// Count returns the number of observations
func (h *Histogram) Count() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.count
}

// Sum returns the sum of all observations
func (h *Histogram) Sum() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sum
}

// Buckets returns bucket boundaries and counts
func (h *Histogram) Buckets() []HistogramBucket {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]HistogramBucket, len(h.buckets))
	copy(result, h.buckets)
	return result
}

// IncCounter is a convenience method to increment a counter by name
func (c *Collector) IncCounter(name string) {
	c.Counter(name).Inc()
}

// SetGauge is a convenience method to set a gauge by name
func (c *Collector) SetGauge(name string, value float64) {
	c.Gauge(name).Set(value)
}

// ObserveHistogram is a convenience method to observe a histogram by name
func (c *Collector) ObserveHistogram(name string, value float64) {
	c.Histogram(name).Observe(value)
}
