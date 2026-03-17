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
	ch := make(chan []Container, 10)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		// Initial poll
		p.poll(ctx, ch)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.poll(ctx, ch)
			}
		}
	}()

	return ch
}

func (p *Poller) poll(ctx context.Context, ch chan<- []Container) {
	containers, err := p.client.ListContainers(ctx)
	if err != nil {
		return
	}

	select {
	case ch <- containers:
	case <-ctx.Done():
	default:
		// Channel full, skip this update
	}
}
