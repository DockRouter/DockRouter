// Package metrics provides Prometheus-compatible metrics collection
package metrics

import (
	"fmt"
	"io"
	"strings"
)

// PrometheusFormat writes metrics in Prometheus text format
func (c *Collector) PrometheusFormat(w io.Writer) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Write counters
	for name, counter := range c.counters {
		fmt.Fprintf(w, "# TYPE %s counter\n", sanitizeName(name))
		fmt.Fprintf(w, "%s %d\n\n", sanitizeName(name), counter.Value())
	}

	// Write gauges
	for name, gauge := range c.gauges {
		fmt.Fprintf(w, "# TYPE %s gauge\n", sanitizeName(name))
		fmt.Fprintf(w, "%s %g\n\n", sanitizeName(name), gauge.value)
	}

	// Write histograms
	for name, hist := range c.histograms {
		fmt.Fprintf(w, "# TYPE %s histogram\n", sanitizeName(name))
		fmt.Fprintf(w, "%s_count %d\n", sanitizeName(name), hist.count)
		fmt.Fprintf(w, "%s_sum %g\n\n", sanitizeName(name), hist.sum)
	}
}

func sanitizeName(name string) string {
	// Replace invalid characters
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return "dockrouter_" + name
}
