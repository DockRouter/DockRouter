// Package discovery handles Docker container discovery
package discovery

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Engine orchestrates container discovery
type Engine struct {
	client    *DockerClient
	events    *EventStream
	poller    *Poller
	routes    RouteSink
	logger    Logger

	mu        sync.RWMutex
	containers map[string]*ContainerInfo
	running   bool
}

// ContainerInfo holds cached container information
type ContainerInfo struct {
	ID        string
	Name      string
	Image     string
	Labels    map[string]string
	Config    *RouteConfig
	Address   string
	Port      int
	Healthy   bool
	UpdatedAt time.Time
}

// RouteSink is the interface for receiving route updates
type RouteSink interface {
	AddRoute(container *ContainerInfo)
	RemoveRoute(containerID string)
}

// Logger interface for discovery engine
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NewEngine creates a new discovery engine
func NewEngine(client *DockerClient, routes RouteSink, logger Logger) *Engine {
	return &Engine{
		client:     client,
		events:     NewEventStream(client),
		poller:     NewPoller(client, 10*time.Second),
		routes:     routes,
		containers: make(map[string]*ContainerInfo),
		logger:     logger,
	}
}

// Start begins container discovery
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = true
	e.mu.Unlock()

	// Initial sync
	if err := e.Sync(ctx); err != nil {
		return err
	}

	// Start event stream
	go e.watchEvents(ctx)

	// Start polling as fallback
	go e.pollLoop(ctx)

	e.logger.Info("Discovery engine started")
	return nil
}

// Sync performs a full sync of all containers
func (e *Engine) Sync(ctx context.Context) error {
	e.logger.Debug("Starting full container sync")

	containers, err := e.client.ListContainers(ctx)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Track which containers we've seen
	seen := make(map[string]bool)

	// Process each container
	for _, c := range containers {
		seen[c.ID] = true

		// Parse labels
		config := ParseLabels(c.Labels)
		if config == nil || !config.Enabled {
			continue
		}

		// Get detailed info
		detail, err := e.client.InspectContainer(ctx, c.ID)
		if err != nil {
			e.logger.Warn("Failed to inspect container",
				"container", c.ID[:12],
				"error", err,
			)
			continue
		}

		// Build container info
		info := e.buildContainerInfo(c, detail, config)

		// Check if new or changed
		if existing, ok := e.containers[c.ID]; ok {
			if !existing.Changed(info) {
				continue
			}
			e.logger.Info("Container updated",
				"container", info.Name,
				"host", config.Host,
			)
		} else {
			e.logger.Info("Container discovered",
				"container", info.Name,
				"host", config.Host,
				"address", info.Address,
			)
		}

		e.containers[c.ID] = info
		e.routes.AddRoute(info)
	}

	// Remove containers that no longer exist
	for id := range e.containers {
		if !seen[id] {
			e.logger.Info("Container removed",
				"container", e.containers[id].Name,
			)
			e.routes.RemoveRoute(id)
			delete(e.containers, id)
		}
	}

	e.logger.Debug("Container sync complete",
		"total", len(containers),
		"enabled", len(e.containers),
	)

	return nil
}

// buildContainerInfo creates ContainerInfo from container data
func (e *Engine) buildContainerInfo(c Container, detail *ContainerDetail, config *RouteConfig) *ContainerInfo {
	info := &ContainerInfo{
		ID:        c.ID,
		Name:      extractName(c.Names),
		Image:     c.Image,
		Labels:    c.Labels,
		Config:    config,
		UpdatedAt: time.Now(),
	}

	// Determine address
	if config.Address != "" {
		info.Address = config.Address
	} else {
		// Get container IP
		ip := GetContainerIP(detail, "")
		port := config.Port
		if port == 0 {
			// Try to auto-detect port from exposed ports
			port = detectPort(c.Ports, detail)
		}
		if port == 0 {
			port = 80 // default
		}
		info.Address = ip + ":" + intToStr(port)
		info.Port = port
	}

	// Check health status
	if detail.State.Healthy {
		info.Healthy = true
	} else if detail.State.Running {
		info.Healthy = true // Assume healthy if running
	}

	return info
}

// Changed checks if container info has changed
func (ci *ContainerInfo) Changed(other *ContainerInfo) bool {
	return ci.Address != other.Address ||
		ci.Healthy != other.Healthy ||
		ci.Config.Host != other.Config.Host ||
		ci.Config.Path != other.Config.Path
}

// watchEvents watches Docker events for container changes
func (e *Engine) watchEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		events, err := e.events.SubscribeLifecycle(ctx)
		if err != nil {
			e.logger.Error("Failed to subscribe to events",
				"error", err,
			)
			time.Sleep(5 * time.Second)
			continue
		}

		for event := range events {
			e.handleEvent(ctx, event)
		}

		// Reconnect delay
		e.logger.Warn("Event stream disconnected, reconnecting...")
		time.Sleep(2 * time.Second)
	}
}

// handleEvent processes a Docker event
func (e *Engine) handleEvent(ctx context.Context, event Event) {
	containerID := GetContainerID(event)
	containerName := GetContainerName(event)

	switch {
	case IsStartEvent(event):
		e.logger.Debug("Container started",
			"container", containerName,
			"id", containerID[:12],
		)
		e.onContainerStart(ctx, containerID)

	case IsStopEvent(event):
		e.logger.Debug("Container stopped",
			"container", containerName,
			"id", containerID[:12],
		)
		e.onContainerStop(containerID)

	case IsHealthEvent(event):
		e.logger.Debug("Container health changed",
			"container", containerName,
			"id", containerID[:12],
		)
		// Refresh container info
		e.onContainerStart(ctx, containerID)
	}
}

// onContainerStart handles container start events
func (e *Engine) onContainerStart(ctx context.Context, id string) {
	// Inspect container
	detail, err := e.client.InspectContainer(ctx, id)
	if err != nil {
		e.logger.Error("Failed to inspect started container",
			"container", id[:12],
			"error", err,
		)
		return
	}

	// Check if enabled for routing
	config := ParseLabels(detail.Config.Labels)
	if config == nil || !config.Enabled {
		return
	}

	// Validate config
	if err := config.Validate(); err != nil {
		e.logger.Warn("Invalid container config",
			"container", extractNameFromDetail(detail),
			"error", err,
		)
		return
	}

	// Build container info
	c := Container{
		ID:     id,
		Names:  []string{detail.Name},
		Image:  detail.Config.Image,
		Labels: detail.Config.Labels,
	}

	info := e.buildContainerInfo(c, detail, config)

	e.mu.Lock()
	e.containers[id] = info
	e.mu.Unlock()

	e.routes.AddRoute(info)

	e.logger.Info("Route added",
		"container", info.Name,
		"host", config.Host,
		"path", config.Path,
		"address", info.Address,
	)
}

// onContainerStop handles container stop events
func (e *Engine) onContainerStop(id string) {
	e.mu.Lock()
	info, exists := e.containers[id]
	if exists {
		delete(e.containers, id)
	}
	e.mu.Unlock()

	if exists {
		e.routes.RemoveRoute(id)
		e.logger.Info("Route removed",
			"container", info.Name,
			"host", info.Config.Host,
		)
	}
}

// pollLoop periodically polls for containers as fallback
func (e *Engine) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.Sync(ctx); err != nil {
				e.logger.Error("Poll sync failed", "error", err)
			}
		}
	}
}

// GetContainers returns all discovered containers
func (e *Engine) GetContainers() []*ContainerInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*ContainerInfo, 0, len(e.containers))
	for _, info := range e.containers {
		result = append(result, info)
	}
	return result
}

// GetContainer returns a specific container
func (e *Engine) GetContainer(id string) *ContainerInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.containers[id]
}

// Helper functions

func extractName(names []string) string {
	for _, name := range names {
		return strings.TrimPrefix(name, "/")
	}
	return ""
}

func extractNameFromDetail(detail *ContainerDetail) string {
	return strings.TrimPrefix(detail.Name, "/")
}

func detectPort(ports []PortBinding, detail *ContainerDetail) int {
	// Try published ports first
	for _, p := range ports {
		if p.PublicPort > 0 {
			return p.PrivatePort
		}
	}

	// Try to find common ports
	// This is a simplified detection - could be improved
	return 0
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var s []byte
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	return string(s)
}
