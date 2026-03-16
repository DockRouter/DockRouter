// Package health provides backend health checking
package health

// HealthState represents backend health status
type HealthState int

const (
	StateUnknown HealthState = iota
	StateHealthy
	StateDegraded
	StateUnhealthy
	StateRecovering
)

func (s HealthState) String() string {
	switch s {
	case StateHealthy:
		return "healthy"
	case StateDegraded:
		return "degraded"
	case StateUnhealthy:
		return "unhealthy"
	case StateRecovering:
		return "recovering"
	default:
		return "unknown"
	}
}
