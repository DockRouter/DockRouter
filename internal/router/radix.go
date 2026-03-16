// Package router handles HTTP routing
package router

import (
	"strings"
	"sync"
)

// RadixNode represents a node in the radix tree
type RadixNode struct {
	path     string
	children []*RadixNode
	route    *Route
	isLeaf   bool
}

// RadixTree implements a radix tree for fast path matching
type RadixTree struct {
	mu   sync.RWMutex
	root *RadixNode
}

// NewRadixTree creates a new radix tree
func NewRadixTree() *RadixTree {
	return &RadixTree{
		root: &RadixNode{},
	}
}

// Insert adds a path with associated route
func (t *RadixTree) Insert(path string, route *Route) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Normalize path
	path = normalizePath(path)

	if path == "/" {
		t.root.route = route
		t.root.isLeaf = true
		return
	}

	t.insert(t.root, path, route)
}

func (t *RadixTree) insert(node *RadixNode, path string, route *Route) {
	// Find common prefix
	for i, child := range node.children {
		prefix := commonPrefix(child.path, path)

		if len(prefix) > 0 {
			if len(prefix) == len(child.path) {
				// Path continues in child
				remaining := path[len(prefix):]
				if remaining == "" {
					// Exact match, update route
					child.route = route
					child.isLeaf = true
					return
				}
				t.insert(child, remaining, route)
				return
			}

			// Need to split
			splitNode := &RadixNode{
				path:     prefix,
				children: []*RadixNode{},
			}

			// Update existing child
			child.path = child.path[len(prefix):]
			splitNode.children = append(splitNode.children, child)

			// Create new child for remaining path
			remaining := path[len(prefix):]
			if remaining != "" {
				newChild := &RadixNode{
					path:   remaining,
					route:  route,
					isLeaf: true,
				}
				splitNode.children = append(splitNode.children, newChild)
			} else {
				splitNode.route = route
				splitNode.isLeaf = true
			}

			node.children[i] = splitNode
			return
		}
	}

	// No common prefix, add as new child
	node.children = append(node.children, &RadixNode{
		path:   path,
		route:  route,
		isLeaf: true,
	})
}

// Match finds the longest prefix match for a path
func (t *RadixTree) Match(path string) *Route {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Normalize path
	path = normalizePath(path)

	if path == "/" {
		return t.root.route
	}

	return t.match(t.root, path, nil)
}

func (t *RadixTree) match(node *RadixNode, path string, lastMatch *Route) *Route {
	// Check if this node has a route (potential match)
	if node.isLeaf && node.route != nil {
		lastMatch = node.route
	}

	if path == "" {
		return lastMatch
	}

	// Search children
	for _, child := range node.children {
		if strings.HasPrefix(path, child.path) {
			remaining := path[len(child.path):]
			if match := t.match(child, remaining, lastMatch); match != nil {
				return match
			}
		}
	}

	// Return the last match found (longest prefix)
	return lastMatch
}

// Delete removes a path from the tree
func (t *RadixTree) Delete(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	path = normalizePath(path)

	if path == "/" {
		t.root.route = nil
		t.root.isLeaf = false
		return
	}

	t.delete(t.root, path)
}

func (t *RadixTree) delete(node *RadixNode, path string) bool {
	for i, child := range node.children {
		if strings.HasPrefix(path, child.path) {
			remaining := path[len(child.path):]

			if remaining == "" {
				// Found the node
				if len(child.children) == 0 {
					// Remove leaf node
					node.children = append(node.children[:i], node.children[i+1:]...)
					return true
				}
				// Just clear route
				child.route = nil
				child.isLeaf = false
				return false
			}

			if t.delete(child, remaining) {
				// Compact if needed
				if len(child.children) == 1 && !child.isLeaf {
					grandchild := child.children[0]
					child.path += grandchild.path
					child.route = grandchild.route
					child.isLeaf = grandchild.isLeaf
					child.children = grandchild.children
				}
			}
			return false
		}
	}
	return false
}

// RemoveByContainerID removes all routes for a container
func (t *RadixTree) RemoveByContainerID(containerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.removeByContainerID(t.root, containerID)
}

func (t *RadixTree) removeByContainerID(node *RadixNode, containerID string) {
	if node.route != nil && node.route.ContainerID == containerID {
		node.route = nil
		node.isLeaf = false
	}

	for _, child := range node.children {
		t.removeByContainerID(child, containerID)
	}
}

// List returns all routes in the tree
func (t *RadixTree) List() []*Route {
	t.mu.RLock()
	defer t.mu.RUnlock()

	routes := make([]*Route, 0)
	t.collectRoutes(t.root, &routes)
	return routes
}

func (t *RadixTree) collectRoutes(node *RadixNode, routes *[]*Route) {
	if node.route != nil {
		*routes = append(*routes, node.route)
	}
	for _, child := range node.children {
		t.collectRoutes(child, routes)
	}
}

// Helper functions

func normalizePath(path string) string {
	// Ensure starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Remove trailing slash (except for root)
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func commonPrefix(a, b string) string {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	i := 0
	for i < minLen && a[i] == b[i] {
		i++
	}

	return a[:i]
}
