package redishealth

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// newTestClient creates a Redis client pointed at a non-existent address
// so pings will fail. For tests that need success, use a miniredis or
// override behavior.
func newUnreachableClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1", // nothing listens here
		DialTimeout: 100 * time.Millisecond,
		ReadTimeout: 100 * time.Millisecond,
	})
}

func TestNewMonitorDefaults(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	m := New(rdb)
	if m.interval != 5*time.Second {
		t.Errorf("expected default interval 5s, got %v", m.interval)
	}
	if !m.connected {
		t.Error("expected initial state to be connected")
	}
}

func TestNewMonitorWithOptions(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	called := false
	m := New(rdb,
		WithInterval(1*time.Second),
		WithOnDown(func() { called = true }),
	)
	if m.interval != 1*time.Second {
		t.Errorf("expected interval 1s, got %v", m.interval)
	}
	// onDown is set but not yet called
	if called {
		t.Error("onDown should not be called at construction")
	}
}

func TestCheckFailsAndSetsDisconnected(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	var downCalled atomic.Int32
	m := New(rdb,
		WithInterval(50*time.Millisecond),
		WithOnDown(func() { downCalled.Add(1) }),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run a single check
	m.check(ctx)

	if m.IsConnected() {
		t.Error("expected disconnected after failed ping")
	}
	if downCalled.Load() != 1 {
		t.Errorf("expected onDown called once, got %d", downCalled.Load())
	}

	status := m.GetStatus()
	if status.Connected {
		t.Error("expected status.Connected=false")
	}
	if status.LastError == "" {
		t.Error("expected LastError to be set")
	}
}

func TestOnDownCalledOncePerTransition(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	var downCount atomic.Int32
	m := New(rdb,
		WithInterval(50*time.Millisecond),
		WithOnDown(func() { downCount.Add(1) }),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// First check transitions from up to down
	m.check(ctx)
	if downCount.Load() != 1 {
		t.Fatalf("expected onDown called once, got %d", downCount.Load())
	}

	// Second check: already down, should not call again
	m.check(ctx)
	if downCount.Load() != 1 {
		t.Errorf("expected onDown still called once, got %d", downCount.Load())
	}
}

func TestGetStatusWhenConnected(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	m := New(rdb)
	// Default state: connected
	status := m.GetStatus()
	if !status.Connected {
		t.Error("expected connected=true in initial state")
	}
	if status.Reconnects != 0 {
		t.Errorf("expected 0 reconnects, got %d", status.Reconnects)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	m := New(rdb, WithInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.Run(ctx)
	}()

	// Let it run for a bit
	time.Sleep(50 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
}

func TestReconnectContextCancelled(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	m := New(rdb)
	m.mu.Lock()
	m.connected = false
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	// Should return immediately without panicking
	m.reconnect(ctx)
}

func TestIsConnectedConcurrentAccess(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	m := New(rdb)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.IsConnected()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.GetStatus()
		}()
	}
	wg.Wait()
}

func TestStatusLatencyField(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	m := New(rdb)
	// Simulate a successful ping that set latency
	m.mu.Lock()
	m.latency = 2 * time.Millisecond
	m.mu.Unlock()

	status := m.GetStatus()
	if status.Latency == "" {
		t.Error("expected Latency to be set")
	}
}

func TestStatusReconnectsIncrement(t *testing.T) {
	rdb := newUnreachableClient()
	defer rdb.Close()

	m := New(rdb)
	m.mu.Lock()
	m.reconnects = 3
	m.mu.Unlock()

	status := m.GetStatus()
	if status.Reconnects != 3 {
		t.Errorf("expected 3 reconnects, got %d", status.Reconnects)
	}
}
