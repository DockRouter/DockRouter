// Package discovery handles Docker container discovery
package discovery

import (
	"context"
	"time"
)

// Poller periodically polls for container changes
type Poller struct {
	client   *DockerClient
	interval time.Duration
}

// NewPoller creates a new container poller
func NewPoller(client *DockerClient, interval time.Duration) *Poller {
	return &Poller{
		client:   client,
		interval: interval,
	}
}

// Start begins periodic polling
func (p *Poller) Start(ctx context.Context) <-chan []Container {
	// TODO: Implement polling
	return nil
}
