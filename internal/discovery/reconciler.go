// Package discovery handles Docker container discovery
package discovery

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

// Engine orchestrates container discovery
type Engine struct {
	client   *DockerClient
	events   *EventStream
	poller   *Poller
	routes   RouteSink
	logger   Logger
	interval time.Duration

	mu         sync.RWMutex
	containers map[string]*ContainerInfo
	running    bool
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
		interval:   30 * time.Second,
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

// IsRunning reports whether discovery has started successfully.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
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
				"container", truncateID(c.ID),
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
	} else if detail.State.Running && detail.State.Status != "unhealthy" {
		// Only assume healthy if running and not explicitly unhealthy
		// (containers without health checks have empty Status)
		info.Healthy = true
	}

	return info
}

// Changed checks if container info has changed
func (ci *ContainerInfo) Changed(other *ContainerInfo) bool {
	if ci.Config == nil || other.Config == nil {
		return ci.Config != other.Config
	}
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

	shortID := truncateID(containerID)

	switch {
	case IsStartEvent(event):
		e.logger.Debug("Container started",
			"container", containerName,
			"id", shortID,
		)
		e.onContainerStart(ctx, containerID)

	case IsStopEvent(event):
		e.logger.Debug("Container stopped",
			"container", containerName,
			"id", shortID,
		)
		e.onContainerStop(containerID)

	case IsHealthEvent(event):
		e.logger.Debug("Container health changed",
			"container", containerName,
			"id", shortID,
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
			"container", truncateID(id),
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
	e.routes.AddRoute(info)
	e.mu.Unlock()

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
		e.routes.RemoveRoute(id)
	}
	e.mu.Unlock()

	if exists {
		e.logger.Info("Route removed",
			"container", info.Name,
			"host", info.Config.Host,
		)
	}
}

// pollLoop periodically polls for containers as fallback
func (e *Engine) pollLoop(ctx context.Context) {
	interval := e.interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
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

// GetContainers returns all discovered containers (returns deep copies to avoid data races)
func (e *Engine) GetContainers() []*ContainerInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*ContainerInfo, 0, len(e.containers))
	for _, info := range e.containers {
		result = append(result, info.deepCopy())
	}
	return result
}

// GetContainer returns a specific container (returns a deep copy to avoid data races)
func (e *Engine) GetContainer(id string) *ContainerInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	info, ok := e.containers[id]
	if !ok {
		return nil
	}
	return info.deepCopy()
}

// InjectContainerForTest adds a container to the engine's cache (for testing only).
func (e *Engine) InjectContainerForTest(info *ContainerInfo) {
	e.mu.Lock()
	e.containers[info.ID] = info
	e.mu.Unlock()
}

// deepCopy creates a deep copy of ContainerInfo, including all nested reference types.
func (ci *ContainerInfo) deepCopy() *ContainerInfo {
	cp := *ci

	// Deep copy Labels map
	if ci.Labels != nil {
		cp.Labels = make(map[string]string, len(ci.Labels))
		for k, v := range ci.Labels {
			cp.Labels[k] = v
		}
	}

	// Deep copy Config
	if ci.Config != nil {
		cfg := *ci.Config

		// TLSDomains
		if ci.Config.TLSDomains != nil {
			cfg.TLSDomains = make([]string, len(ci.Config.TLSDomains))
			copy(cfg.TLSDomains, ci.Config.TLSDomains)
		}

		// CORS.Origins
		if ci.Config.CORS.Origins != nil {
			cfg.CORS.Origins = make([]string, len(ci.Config.CORS.Origins))
			copy(cfg.CORS.Origins, ci.Config.CORS.Origins)
		}

		// CORS.Methods
		if ci.Config.CORS.Methods != nil {
			cfg.CORS.Methods = make([]string, len(ci.Config.CORS.Methods))
			copy(cfg.CORS.Methods, ci.Config.CORS.Methods)
		}

		// CORS.Headers
		if ci.Config.CORS.Headers != nil {
			cfg.CORS.Headers = make([]string, len(ci.Config.CORS.Headers))
			copy(cfg.CORS.Headers, ci.Config.CORS.Headers)
		}

		// BasicAuthUsers
		if ci.Config.BasicAuthUsers != nil {
			cfg.BasicAuthUsers = make([]BasicAuthUser, len(ci.Config.BasicAuthUsers))
			copy(cfg.BasicAuthUsers, ci.Config.BasicAuthUsers)
		}

		// IPWhitelist
		if ci.Config.IPWhitelist != nil {
			cfg.IPWhitelist = make([]*net.IPNet, len(ci.Config.IPWhitelist))
			for i, cidr := range ci.Config.IPWhitelist {
				cpy := *cidr
				cfg.IPWhitelist[i] = &cpy
			}
		}

		// IPBlacklist
		if ci.Config.IPBlacklist != nil {
			cfg.IPBlacklist = make([]*net.IPNet, len(ci.Config.IPBlacklist))
			for i, cidr := range ci.Config.IPBlacklist {
				cpy := *cidr
				cfg.IPBlacklist[i] = &cpy
			}
		}

		// Middlewares
		if ci.Config.Middlewares != nil {
			cfg.Middlewares = make([]string, len(ci.Config.Middlewares))
			copy(cfg.Middlewares, ci.Config.Middlewares)
		}

		// RawLabels
		if ci.Config.RawLabels != nil {
			cfg.RawLabels = make(map[string]string, len(ci.Config.RawLabels))
			for k, v := range ci.Config.RawLabels {
				cfg.RawLabels[k] = v
			}
		}

		cp.Config = &cfg
	}

	return &cp
}

// Helper functions

func extractName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
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
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var s []byte
	for n > 0 {
		s = append([]byte{byte('0' + n%10)}, s...)
		n /= 10
	}
	if neg {
		s = append([]byte{'-'}, s...)
	}
	return string(s)
}

// truncateID safely truncates a container ID to 12 characters
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
