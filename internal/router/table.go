// Package router handles HTTP routing
package router

import (
	"strings"
	"sync"
)

// Table manages all routes with concurrent-safe access
// It supports exact host matching and wildcard matching
type Table struct {
	mu       sync.RWMutex
	exact    map[string]*RadixTree // exact host -> path tree
	wildcard map[string]*RadixTree // *.domain.com -> path tree
	routes   map[string]*Route     // route ID -> route
}

// NewTable creates a new route table
func NewTable() *Table {
	return &Table{
		exact:    make(map[string]*RadixTree),
		wildcard: make(map[string]*RadixTree),
		routes:   make(map[string]*Route),
	}
}

// Add inserts or updates a route
func (t *Table) Add(route *Route) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Remove old route if exists
	if existing, ok := t.routes[route.ID]; ok {
		t.removeFromTrees(existing)
	}

	// Add to routes map
	t.routes[route.ID] = route

	// Add to appropriate tree
	t.addToTree(route)
}

// Remove deletes a route by ID
func (t *Table) Remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if route, ok := t.routes[id]; ok {
		t.removeFromTrees(route)
		delete(t.routes, id)
	}
}

// RemoveByContainer removes all routes for a container
func (t *Table) RemoveByContainer(containerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for id, route := range t.routes {
		if route.ContainerID == containerID {
			t.removeFromTrees(route)
			delete(t.routes, id)
		}
	}
}

// Get retrieves a route by ID
func (t *Table) Get(id string) *Route {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.routes[id]
}

// Match finds a route for the given host and path
func (t *Table) Match(host, path string) *Route {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Normalize
	host = normalizeHost(host)
	path = normalizePath(path)

	// 1. Try exact host match first
	if tree, ok := t.exact[host]; ok {
		if route := tree.Match(path); route != nil {
			return route
		}
	}

	// 2. Try wildcard match
	for pattern, tree := range t.wildcard {
		if wildcardMatch(pattern, host) {
			if route := tree.Match(path); route != nil {
				return route
			}
		}
	}

	return nil
}

// List returns all routes
func (t *Table) List() []*Route {
	t.mu.RLock()
	defer t.mu.RUnlock()

	routes := make([]*Route, 0, len(t.routes))
	for _, r := range t.routes {
		routes = append(routes, r)
	}
	return routes
}

// ListByHost returns all routes for a specific host
func (t *Table) ListByHost(host string) []*Route {
	t.mu.RLock()
	defer t.mu.RUnlock()

	host = normalizeHost(host)
	routes := make([]*Route, 0)

	// Get exact matches
	if tree, ok := t.exact[host]; ok {
		routes = append(routes, tree.List()...)
	}

	// Get wildcard matches
	for pattern, tree := range t.wildcard {
		if wildcardMatch(pattern, host) {
			routes = append(routes, tree.List()...)
		}
	}

	return routes
}

// Hosts returns all configured hosts
func (t *Table) Hosts() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	hosts := make(map[string]bool)
	for h := range t.exact {
		hosts[h] = true
	}
	for h := range t.wildcard {
		hosts[h] = true
	}

	result := make([]string, 0, len(hosts))
	for h := range hosts {
		result = append(result, h)
	}
	return result
}

// Count returns total number of routes
func (t *Table) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.routes)
}

// addToTree adds a route to the appropriate tree
func (t *Table) addToTree(route *Route) {
	host := normalizeHost(route.Host)
	path := normalizePath(route.PathPrefix)

	if path == "" {
		path = "/"
	}

	// Determine if wildcard
	if strings.HasPrefix(host, "*.") {
		// Wildcard pattern
		pattern := host
		if _, ok := t.wildcard[pattern]; !ok {
			t.wildcard[pattern] = NewRadixTree()
		}
		t.wildcard[pattern].Insert(path, route)
	} else {
		// Exact host
		if _, ok := t.exact[host]; !ok {
			t.exact[host] = NewRadixTree()
		}
		t.exact[host].Insert(path, route)
	}
}

// removeFromTrees removes a route from trees
func (t *Table) removeFromTrees(route *Route) {
	host := normalizeHost(route.Host)
	path := normalizePath(route.PathPrefix)

	if path == "" {
		path = "/"
	}

	if strings.HasPrefix(host, "*.") {
		if tree, ok := t.wildcard[host]; ok {
			tree.Delete(path)
			if tree.IsEmpty() {
				delete(t.wildcard, host)
			}
		}
	} else {
		if tree, ok := t.exact[host]; ok {
			tree.Delete(path)
			if tree.IsEmpty() {
				delete(t.exact, host)
			}
		}
	}
}

// normalizeHost normalizes a host string
func normalizeHost(host string) string {
	// Remove port if present
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	return strings.ToLower(strings.TrimSpace(host))
}

// wildcardMatch checks if host matches a wildcard pattern
func wildcardMatch(pattern, host string) bool {
	// Pattern is like *.example.com
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}

	suffix := pattern[1:] // .example.com
	return strings.HasSuffix(host, suffix) || host == pattern[2:]
}
