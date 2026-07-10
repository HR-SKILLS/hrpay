package breaker

import (
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "CLOSED"
	StateOpen     State = "OPEN"
	StateHalfOpen State = "HALF_OPEN"
)

type CircuitBreaker struct {
	mu               sync.RWMutex
	state            State
	failureCount     int
	lastFailureTime  time.Time
	failureThreshold int
	resetTimeout     time.Duration
}

type Config struct {
	FailureThreshold int
	ResetTimeout     time.Duration
}

func NewCircuitBreaker(config Config) *CircuitBreaker {
	threshold := config.FailureThreshold
	if threshold <= 0 {
		threshold = 5
	}
	timeout := config.ResetTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: threshold,
		resetTimeout:     timeout,
	}
}

func (cb *CircuitBreaker) Execute(req func() error, isSystemFailure func(error) bool) error {
	if err := cb.checkState(); err != nil {
		return err
	}

	err := req()
	if err != nil {
		if isSystemFailure(err) {
			cb.onFailure()
		}
		return err
	}

	cb.onSuccess()
	return nil
}

func (cb *CircuitBreaker) checkState() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen {
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
		} else {
			return ErrCircuitOpen
		}
	}
	return nil
}

func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = StateClosed
}

func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.state == StateHalfOpen || cb.failureCount >= cb.failureThreshold {
		cb.state = StateOpen
	}
}

func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failureCount = 0
	cb.lastFailureTime = time.Time{}
}

// ErrCircuitOpen represents a circuit breaker open error.
type errCircuitOpen struct{}

func (e errCircuitOpen) Error() string {
	return "circuit breaker is open"
}

var ErrCircuitOpen error = errCircuitOpen{}
