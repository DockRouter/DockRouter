// Package discovery handles Docker container discovery
package discovery

import (
	"context"
	"time"
)

// EventStream handles Docker event streaming
type EventStream struct {
	client *DockerClient
}

// NewEventStream creates a new event stream handler
func NewEventStream(client *DockerClient) *EventStream {
	return &EventStream{client: client}
}

// Subscribe starts listening for container events
func (e *EventStream) Subscribe(ctx context.Context) (<-chan Event, error) {
	// Filter for container events only
	filters := map[string]string{
		"type": "container",
	}

	return e.client.EventsStream(ctx, filters)
}

// SubscribeWithFilters starts listening with custom filters
func (e *EventStream) SubscribeWithFilters(ctx context.Context, filters map[string]string) (<-chan Event, error) {
	// Ensure we only get container events
	if _, ok := filters["type"]; !ok {
		filters["type"] = "container"
	}

	return e.client.EventsStream(ctx, filters)
}

// SubscribeLifecycle listens for container lifecycle events (start, stop, die)
func (e *EventStream) SubscribeLifecycle(ctx context.Context) (<-chan Event, error) {
	filters := map[string]string{
		"type":  "container",
		"event": "start,stop,die,health_status",
	}

	return e.client.EventsStream(ctx, filters)
}

// IsStartEvent checks if event is a container start
func IsStartEvent(event Event) bool {
	return event.Type == "container" && event.Action == "start"
}

// IsStopEvent checks if event is a container stop
func IsStopEvent(event Event) bool {
	return event.Type == "container" && (event.Action == "stop" || event.Action == "die")
}

// IsHealthEvent checks if event is a health status change
func IsHealthEvent(event Event) bool {
	return event.Type == "container" && event.Action == "health_status"
}

// GetContainerID extracts container ID from event
func GetContainerID(event Event) string {
	return event.Actor.ID
}

// GetContainerName extracts container name from event
func GetContainerName(event Event) string {
	if event.Actor.Attributes != nil {
		return event.Actor.Attributes["name"]
	}
	return ""
}

// GetContainerImage extracts container image from event
func GetContainerImage(event Event) string {
	if event.Actor.Attributes != nil {
		return event.Actor.Attributes["image"]
	}
	return ""
}

// EventTimestamp returns the event timestamp as time.Time
func EventTimestamp(event Event) time.Time {
	return time.Unix(event.Time, 0)
}
