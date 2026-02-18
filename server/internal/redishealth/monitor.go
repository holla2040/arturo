package redishealth

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Status represents the current Redis connection state.
type Status struct {
	Connected    bool      `json:"connected"`
	LastPingOK   time.Time `json:"last_ping_ok,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	Reconnects   int       `json:"reconnects"`
	Latency      string    `json:"latency,omitempty"`
}

// Monitor performs periodic ping-based health checks on a Redis client
// and tracks connection state. It supports automatic reconnection with
// exponential backoff.
type Monitor struct {
	rdb      *redis.Client
	interval time.Duration

	mu        sync.RWMutex
	connected bool
	lastPing  time.Time
	lastErr   string
	reconnects int
	latency   time.Duration

	// callbacks
	onDown func()
	onUp   func()
}

// Option configures the Monitor.
type Option func(*Monitor)

// WithInterval sets the health check interval (default 5s).
func WithInterval(d time.Duration) Option {
	return func(m *Monitor) {
		m.interval = d
	}
}

// WithOnDown is called when the connection transitions from up to down.
func WithOnDown(fn func()) Option {
	return func(m *Monitor) {
		m.onDown = fn
	}
}

// WithOnUp is called when the connection transitions from down to up.
func WithOnUp(fn func()) Option {
	return func(m *Monitor) {
		m.onUp = fn
	}
}

// New creates a new Redis health monitor.
func New(rdb *redis.Client, opts ...Option) *Monitor {
	m := &Monitor{
		rdb:       rdb,
		interval:  5 * time.Second,
		connected: true, // assume connected at start
		lastPing:  time.Now(),
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Run starts the health check loop. It blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

// check performs a single PING and updates state.
func (m *Monitor) check(ctx context.Context) {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	start := time.Now()
	err := m.rdb.Ping(pingCtx).Err()
	elapsed := time.Since(start)

	m.mu.Lock()
	wasConnected := m.connected

	if err != nil {
		m.connected = false
		m.lastErr = err.Error()
		m.mu.Unlock()

		if wasConnected {
			log.Printf("redis health: connection lost: %v", err)
			if m.onDown != nil {
				m.onDown()
			}
		}

		m.reconnect(ctx)
		return
	}

	m.connected = true
	m.lastPing = time.Now()
	m.latency = elapsed
	m.lastErr = ""
	m.mu.Unlock()

	if !wasConnected {
		log.Printf("redis health: connection restored (latency=%v)", elapsed)
		if m.onUp != nil {
			m.onUp()
		}
	}
}

// reconnect attempts to re-establish the Redis connection with exponential backoff.
// It tries up to 10 times per reconnect cycle.
func (m *Monitor) reconnect(ctx context.Context) {
	const maxAttempts = 10
	const baseDelay = 500 * time.Millisecond
	const maxDelay = 30 * time.Second

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
		if delay > maxDelay {
			delay = maxDelay
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := m.rdb.Ping(pingCtx).Err()
		cancel()

		if err == nil {
			m.mu.Lock()
			m.connected = true
			m.lastPing = time.Now()
			m.lastErr = ""
			m.reconnects++
			m.mu.Unlock()

			log.Printf("redis health: reconnected after %d attempts", attempt+1)
			if m.onUp != nil {
				m.onUp()
			}
			return
		}

		log.Printf("redis health: reconnect attempt %d/%d failed: %v", attempt+1, maxAttempts, err)
	}

	log.Printf("redis health: reconnect failed after %d attempts, will retry on next health check", maxAttempts)
}

// IsConnected returns whether the last health check succeeded.
func (m *Monitor) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// GetStatus returns the current health status.
func (m *Monitor) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s := Status{
		Connected:  m.connected,
		LastPingOK: m.lastPing,
		Reconnects: m.reconnects,
	}
	if m.lastErr != "" {
		s.LastError = m.lastErr
	}
	if m.latency > 0 {
		s.Latency = m.latency.String()
	}
	return s
}
