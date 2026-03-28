// Package circuitbreaker provides a reusable circuit breaker abstraction
// built on top of sony/gobreaker for wrapping fallible remote calls.
package circuitbreaker

import (
	"fmt"
	"time"

	"github.com/sony/gobreaker"
)

// Settings controls the circuit breaker behavior.
type Settings struct {
	// Name is a unique identifier for this breaker (used in logs/metrics).
	Name string

	// MaxRequests is the number of allowed requests in half-open state.
	MaxRequests uint32

	// Interval is the rolling window for counting failures (0 = no rolling window).
	Interval time.Duration

	// Timeout is how long the breaker stays open before going half-open.
	Timeout time.Duration

	// Threshold is the minimum number of failures before the breaker opens.
	Threshold uint32
}

// DefaultSettings returns sensible defaults for inter-service gRPC calls.
func DefaultSettings(name string) Settings {
	return Settings{
		Name:        name,
		MaxRequests: 5,
		Interval:    30 * time.Second,
		Timeout:     10 * time.Second,
		Threshold:   5,
	}
}

// CircuitBreaker wraps sony/gobreaker with a simpler API.
type CircuitBreaker struct {
	cb *gobreaker.CircuitBreaker
}

// New creates a new CircuitBreaker with the given settings.
func New(s Settings) *CircuitBreaker {
	st := gobreaker.Settings{
		Name:        s.Name,
		MaxRequests: s.MaxRequests,
		Interval:    s.Interval,
		Timeout:     s.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(s.Threshold)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			// State changes are observable via logs / metrics externally.
			_ = fmt.Sprintf("circuit breaker %s: %s → %s", name, from, to)
		},
	}

	return &CircuitBreaker{cb: gobreaker.NewCircuitBreaker(st)}
}

// Execute runs the given function through the circuit breaker.
// If the breaker is open, it returns an error immediately without calling fn.
func (c *CircuitBreaker) Execute(fn func() (interface{}, error)) (interface{}, error) {
	return c.cb.Execute(fn)
}

// State returns the current state of the circuit breaker.
func (c *CircuitBreaker) State() gobreaker.State {
	return c.cb.State()
}
