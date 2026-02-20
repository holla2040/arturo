// Package redisrouter implements executor.DeviceRouter by sending protocol
// messages through Redis Pub/Sub. It publishes command requests to the station's
// command channel and subscribes to its own response channel for the correlated response.
package redisrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/script/executor"
	"github.com/redis/go-redis/v9"
)

// RedisRouter sends device commands over Redis Pub/Sub and waits for responses.
type RedisRouter struct {
	rdb     *redis.Client
	source  protocol.Source
	station string // station instance id, e.g. "station-01"
}

// New creates a RedisRouter.
//   - rdb: connected go-redis client
//   - source: protocol Source for this engine instance
//   - station: station instance id (used to address command channel)
func New(rdb *redis.Client, source protocol.Source, station string) *RedisRouter {
	return &RedisRouter{rdb: rdb, source: source, station: station}
}

// SendCommand implements executor.DeviceRouter.
func (r *RedisRouter) SendCommand(ctx context.Context, deviceID, command string, params map[string]string, timeoutMs int) (*executor.CommandResult, error) {
	// Default timeout: 5 seconds.
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}

	// 1. Build protocol message.
	msg, err := protocol.BuildCommandRequest(r.source, deviceID, command, params, timeoutMs)
	if err != nil {
		return nil, fmt.Errorf("build command request: %w", err)
	}
	correlationID := msg.Envelope.CorrelationID

	// 2. Marshal to JSON.
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}

	// 3. Subscribe to response channel BEFORE publishing command.
	responseChannel := "responses:" + r.source.Instance
	sub := r.rdb.Subscribe(ctx, responseChannel)
	defer sub.Close()

	ch := sub.Channel()

	// 4. Publish to the station's command channel.
	channelKey := "commands:" + r.station
	if err := r.rdb.Publish(ctx, channelKey, string(data)).Err(); err != nil {
		return nil, fmt.Errorf("PUBLISH %s: %w", channelKey, err)
	}

	// 5. Read response from subscription with timeout.
	deadline := time.Duration(timeoutMs) * time.Millisecond
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	for {
		select {
		case subMsg, ok := <-ch:
			if !ok {
				return nil, fmt.Errorf("subscription channel closed")
			}

			respMsg, parseErr := protocol.Parse([]byte(subMsg.Payload))
			if parseErr != nil {
				continue
			}

			// Match by correlation_id.
			if respMsg.Envelope.CorrelationID != correlationID {
				continue
			}

			// 6. Parse command response payload.
			payload, payloadErr := protocol.ParseCommandResponse(respMsg)
			if payloadErr != nil {
				return nil, fmt.Errorf("parse response payload: %w", payloadErr)
			}

			resp := ""
			if payload.Response != nil {
				resp = *payload.Response
			}
			dur := 0
			if payload.DurationMs != nil {
				dur = *payload.DurationMs
			}

			return &executor.CommandResult{
				Success:    payload.Success,
				Response:   resp,
				DurationMs: dur,
			}, nil

		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for response on %s (correlation_id=%s)", responseChannel, correlationID)

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
