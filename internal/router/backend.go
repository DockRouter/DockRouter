// Package router handles HTTP routing
package router

import (
	"sync"
	"sync/atomic"
	"time"
)

// LoadBalanceStrategy defines load balancing algorithm
type LoadBalanceStrategy int

const (
	RoundRobin LoadBalanceStrategy = iota
	Random
	IPHash
	LeastConn
)

// BackendPool manages multiple backend targets
type BackendPool struct {
	mu       sync.RWMutex
	Targets  []*BackendTarget
	Strategy LoadBalanceStrategy

	// Round-robin counter
	rrCounter uint64
}

// BackendTarget represents a single backend server
type BackendTarget struct {
	Address     string
	ContainerID string
	Weight      int
	Healthy     bool
	LastCheck   time.Time

	// Stats
	requests int64
	failures int64
}

// NewBackendPool creates a new backend pool
func NewBackendPool(strategy LoadBalanceStrategy) *BackendPool {
	return &BackendPool{
		Targets:  make([]*BackendTarget, 0),
		Strategy: strategy,
	}
}

// Add adds a backend target
func (p *BackendPool) Add(target *BackendTarget) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if already exists
	for _, t := range p.Targets {
		if t.Address == target.Address {
			// Update existing
			t.ContainerID = target.ContainerID
			t.Weight = target.Weight
			t.Healthy = target.Healthy
			return
		}
	}

	p.Targets = append(p.Targets, target)
}

// Remove removes a backend target by container ID
func (p *BackendPool) Remove(containerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, t := range p.Targets {
		if t.ContainerID == containerID {
			p.Targets = append(p.Targets[:i], p.Targets[i+1:]...)
			return
		}
	}
}

// Select chooses a backend based on the strategy
func (p *BackendPool) Select(clientIP string) *BackendTarget {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Filter healthy targets
	healthy := make([]*BackendTarget, 0)
	for _, t := range p.Targets {
		if t.Healthy {
			healthy = append(healthy, t)
		}
	}

	if len(healthy) == 0 {
		return nil
	}

	switch p.Strategy {
	case RoundRobin:
		return p.selectRoundRobin(healthy)
	case IPHash:
		return p.selectIPHash(healthy, clientIP)
	case LeastConn:
		return p.selectLeastConn(healthy)
	default:
		return p.selectRoundRobin(healthy)
	}
}

// selectRoundRobin selects using round-robin
func (p *BackendPool) selectRoundRobin(targets []*BackendTarget) *BackendTarget {
	idx := atomic.AddUint64(&p.rrCounter, 1) - 1
	return targets[idx%uint64(len(targets))]
}

// selectIPHash selects based on client IP hash
func (p *BackendPool) selectIPHash(targets []*BackendTarget, clientIP string) *BackendTarget {
	if clientIP == "" {
		return p.selectRoundRobin(targets)
	}

	// Simple hash
	hash := uint64(0)
	for _, c := range clientIP {
		hash = hash*31 + uint64(c)
	}

	return targets[hash%uint64(len(targets))]
}

// selectLeastConn selects target with least connections
func (p *BackendPool) selectLeastConn(targets []*BackendTarget) *BackendTarget {
	var selected *BackendTarget
	minReqs := int64(1<<63 - 1)

	for _, t := range targets {
		reqs := atomic.LoadInt64(&t.requests)
		if reqs < minReqs {
			minReqs = reqs
			selected = t
		}
	}

	return selected
}

// MarkHealthy marks a backend as healthy
func (p *BackendPool) MarkHealthy(address string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, t := range p.Targets {
		if t.Address == address {
			t.Healthy = true
			t.LastCheck = time.Now()
			return
		}
	}
}

// MarkUnhealthy marks a backend as unhealthy
func (p *BackendPool) MarkUnhealthy(address string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, t := range p.Targets {
		if t.Address == address {
			t.Healthy = false
			t.LastCheck = time.Now()
			return
		}
	}
}

// RecordRequest records a request to a backend
func (p *BackendPool) RecordRequest(address string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, t := range p.Targets {
		if t.Address == address {
			atomic.AddInt64(&t.requests, 1)
			return
		}
	}
}

// RecordFailure records a failure for a backend
func (p *BackendPool) RecordFailure(address string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, t := range p.Targets {
		if t.Address == address {
			atomic.AddInt64(&t.failures, 1)
			return
		}
	}
}

// HealthyCount returns number of healthy backends
func (p *BackendPool) HealthyCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, t := range p.Targets {
		if t.Healthy {
			count++
		}
	}
	return count
}

// IsEmpty returns true if pool has no targets
func (p *BackendPool) IsEmpty() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.Targets) == 0
}

// AllUnhealthy returns true if all backends are unhealthy
func (p *BackendPool) AllUnhealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, t := range p.Targets {
		if t.Healthy {
			return false
		}
	}
	return len(p.Targets) > 0
}
