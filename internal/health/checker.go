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

	// onStateChange is invoked whenever a target's state changes. It is what
	// connects check results back to the route table; without it the checker
	// would observe failures but never act on them.
	onStateChange func(target string, state HealthState)
}

// HealthCheck represents a single backend health check
type HealthCheck struct {
	Target     string
	Path       string
	Type       string // "http" or "tcp", defaults to "http"
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

// OnStateChange registers a callback invoked when a target's health state
// changes. Must be set before Start.
func (c *Checker) OnStateChange(fn func(target string, state HealthState)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStateChange = fn
}

// Register adds a backend for health checking. Re-registering an existing
// target keeps its accumulated state so a config refresh does not reset it.
func (c *Checker) Register(target string, config HealthCheck) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.checks[target]; ok {
		existing.Path = config.Path
		existing.Type = config.Type
		existing.Interval = config.Interval
		existing.Timeout = config.Timeout
		existing.Threshold = config.Threshold
		existing.Recovery = config.Recovery
		return
	}

	config.Target = target
	if config.Timeout <= 0 {
		config.Timeout = c.timeout
	}
	c.checks[target] = &config
}

// Targets returns every registered health check target.
func (c *Checker) Targets() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	targets := make([]string, 0, len(c.checks))
	for target := range c.checks {
		targets = append(targets, target)
	}
	return targets
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
	// Collect targets under read lock, then run checks without holding the lock
	c.mu.RLock()
	type checkTarget struct {
		target string
		check  *HealthCheck
	}
	targets := make([]checkTarget, 0, len(c.checks))
	for target, check := range c.checks {
		targets = append(targets, checkTarget{target, check})
	}
	c.mu.RUnlock()

	for _, t := range targets {
		go c.checkOne(t.target, t.check)
	}
}

func (c *Checker) checkOne(target string, check *HealthCheck) {
	var healthy bool
	var err error

	switch check.Type {
	case "tcp":
		healthy, err = TCPCheck(target, check.Timeout)
	default:
		healthy, err = HTTPCheck(target, check.Path, check.Timeout)
	}

	if err != nil {
		healthy = false
	}

	c.mu.Lock()
	previous := check.State

	if healthy {
		check.ConsecFail = 0
		check.ConsecPass++

		// Recovery logic
		if check.State == StateUnknown {
			check.State = StateHealthy
		} else if check.State == StateUnhealthy || check.State == StateRecovering {
			if check.Recovery <= 0 || check.ConsecPass >= check.Recovery {
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

		// Ensure minimum threshold
		if check.Threshold <= 0 {
			check.Threshold = 3
		}

		// Degradation logic
		if check.ConsecFail >= check.Threshold {
			check.State = StateUnhealthy
		} else if check.ConsecFail >= check.Threshold/2 {
			check.State = StateDegraded
		}
	}

	current := check.State
	notify := c.onStateChange
	c.mu.Unlock()

	// Notify outside the lock: the callback reaches into the route table.
	if notify != nil && current != previous {
		notify(target, current)
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
