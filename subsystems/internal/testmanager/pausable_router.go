// Package testmanager manages test lifecycle per station.
package testmanager

import (
	"context"

	"github.com/holla2040/arturo/internal/script/executor"
)

// PausableRouter wraps a DeviceRouter and blocks SendCommand calls while paused.
// The executor is unaware of the pause — it simply blocks between device commands.
type PausableRouter struct {
	inner   executor.DeviceRouter
	pauseCh chan struct{} // closed = running, created = paused
	paused  bool
}

// NewPausableRouter creates a PausableRouter wrapping the given router.
func NewPausableRouter(inner executor.DeviceRouter) *PausableRouter {
	return &PausableRouter{
		inner:   inner,
		pauseCh: nil,
		paused:  false,
	}
}

// SendCommand delegates to the inner router, but blocks if the session is paused.
func (p *PausableRouter) SendCommand(ctx context.Context, deviceID, command string, params map[string]string, timeoutMs int) (*executor.CommandResult, error) {
	// If paused, wait until resumed or context cancelled
	if p.paused && p.pauseCh != nil {
		select {
		case <-p.pauseCh:
			// Resumed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return p.inner.SendCommand(ctx, deviceID, command, params, timeoutMs)
}

// Pause blocks future SendCommand calls until Resume is called.
func (p *PausableRouter) Pause() {
	if p.paused {
		return
	}
	p.pauseCh = make(chan struct{})
	p.paused = true
}

// Resume unblocks paused SendCommand calls.
func (p *PausableRouter) Resume() {
	if !p.paused {
		return
	}
	close(p.pauseCh)
	p.paused = false
}

// IsPaused returns whether the router is currently paused.
func (p *PausableRouter) IsPaused() bool {
	return p.paused
}
