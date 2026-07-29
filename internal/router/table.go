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

	// Deep copy to prevent caller mutation
	cloned := route.Clone()

	// Add to routes map
	t.routes[route.ID] = cloned

	// Add to appropriate tree
	t.addToTree(cloned)
}

// RouteKey is the identity of a route. Routes sharing a host and path prefix
// are the same route served by a pool of backends, so this is what containers
// are keyed by.
func RouteKey(host, path string) string {
	p := normalizePath(path)
	if p == "" {
		p = "/"
	}
	return normalizeHost(host) + "|" + p
}

// Upsert registers a backend target under the route's host and path prefix.
//
// Containers that share a host and path — replicas of the same service — are
// merged into a single route whose pool holds every replica. This is what makes
// the load-balancing strategies effective; keying routes by container ID would
// make each replica overwrite the previous one.
func (t *Table) Upsert(route *Route, target *BackendTarget) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := RouteKey(route.Host, route.PathPrefix)

	// Copy-on-write: readers hold the old *Route outside the table lock, so the
	// stored route is replaced rather than mutated in place.
	updated := route.Clone()
	updated.ID = key

	if existing, ok := t.routes[key]; ok && existing.Backend != nil {
		// Reuse the live pool so health state and connection counts survive.
		updated.Backend = existing.Backend
		updated.Backend.SetStrategy(route.Backend.Strategy)
		t.removeFromTrees(existing)
	} else if updated.Backend == nil {
		updated.Backend = NewBackendPool(RoundRobin)
	}

	if target != nil {
		updated.Backend.Add(target)
	}

	t.routes[key] = updated
	t.addToTree(updated)
}

// SetBackendHealth updates the health of a backend address across every route
// that serves it. Used by the health checker to put backends in and out of
// rotation.
func (t *Table) SetBackendHealth(address string, healthy bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, route := range t.routes {
		if route.Backend == nil {
			continue
		}
		if healthy {
			route.Backend.MarkHealthy(address)
		} else {
			route.Backend.MarkUnhealthy(address)
		}
	}
}

// BackendAddresses returns every distinct backend address in the table.
func (t *Table) BackendAddresses() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	seen := make(map[string]bool)
	addrs := make([]string, 0, len(t.routes))
	for _, route := range t.routes {
		if route.Backend == nil {
			continue
		}
		for _, target := range route.Backend.Snapshot() {
			if !seen[target.Address] {
				seen[target.Address] = true
				addrs = append(addrs, target.Address)
			}
		}
	}
	return addrs
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

// RemoveByContainer removes a container's backend from every route it serves.
//
// A route is only dropped once its pool is empty, so stopping one replica of a
// scaled service leaves the remaining replicas serving traffic.
func (t *Table) RemoveByContainer(containerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for id, route := range t.routes {
		if route.Backend != nil {
			route.Backend.Remove(containerID)
			if !route.Backend.IsEmpty() {
				continue
			}
		}
		// Either the pool drained or the route predates pooling; drop it.
		if route.Backend == nil && route.ContainerID != containerID {
			continue
		}
		t.removeFromTrees(route)
		delete(t.routes, id)
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
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}

	suffix := pattern[1:] // .example.com

	// Exact bare domain match (e.g., example.com matches *.example.com)
	if host == pattern[2:] {
		return true
	}

	// Subdomain match: suffix starts with "." so HasSuffix ensures dot boundary
	if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
		return true
	}

	return false
}
