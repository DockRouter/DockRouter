// Package health provides backend health checking
package health

import (
	"context"
	"sync"
	"time"
)

// Checker orchestrates health checks for all backends
type Checker struct {
	mu       sync.RWMutex
	checks   map[string]*HealthCheck
	interval time.Duration
	timeout  time.Duration
}

// HealthCheck represents a single backend health check
type HealthCheck struct {
	Target     string
	Path       string
	Interval   time.Duration
	Timeout    time.Duration
	Threshold  int
	Recovery   int
	State      HealthState
	ConsecFail int
	ConsecPass int
}

// NewChecker creates a new health checker
func NewChecker(interval, timeout time.Duration) *Checker {
	return &Checker{
		checks:   make(map[string]*HealthCheck),
		interval: interval,
		timeout:  timeout,
	}
}

// Register adds a backend for health checking
func (c *Checker) Register(target string, config HealthCheck) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks[target] = &config
}

// Unregister removes a backend from health checking
func (c *Checker) Unregister(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.checks, target)
}

// Start begins the health check loop
func (c *Checker) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkAll()
		}
	}
}

func (c *Checker) checkAll() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for target, check := range c.checks {
		go c.checkOne(target, check)
	}
}

func (c *Checker) checkOne(target string, check *HealthCheck) {
	healthy, err := HTTPCheck(target, check.Path, check.Timeout)
	if err != nil {
		healthy = false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if healthy {
		check.ConsecFail = 0
		check.ConsecPass++

		// Recovery logic
		if check.State == StateUnhealthy || check.State == StateRecovering {
			if check.ConsecPass >= check.Recovery {
				check.State = StateHealthy
			} else {
				check.State = StateRecovering
			}
		} else if check.State == StateDegraded && check.ConsecPass >= 2 {
			check.State = StateHealthy
		}
	} else {
		check.ConsecPass = 0
		check.ConsecFail++

		// Degradation logic
		if check.ConsecFail >= check.Threshold {
			check.State = StateUnhealthy
		} else if check.ConsecFail >= check.Threshold/2 {
			check.State = StateDegraded
		}
	}
}

// GetState returns the health state for a target
func (c *Checker) GetState(target string) HealthState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if check, ok := c.checks[target]; ok {
		return check.State
	}
	return StateUnknown
}
