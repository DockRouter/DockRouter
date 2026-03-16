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
	count uint64
	sum   float64
	// TODO: Add bucket tracking
}

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
	hist := &Histogram{}
	c.histograms[name] = hist
	return hist
}

// Observe records an observation
func (h *Histogram) Observe(v float64) {
	atomic.AddUint64(&h.count, 1)
	// Note: not thread-safe for sum, would need mutex
	h.sum += v
}
