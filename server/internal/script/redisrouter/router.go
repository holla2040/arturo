// Package redisrouter implements executor.DeviceRouter by sending protocol
// messages through Redis streams. It writes command requests to the station's
// command stream and reads the correlated response from its own response stream.
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

// RedisRouter sends device commands over Redis streams and waits for responses.
type RedisRouter struct {
	rdb     *redis.Client
	source  protocol.Source
	station string // station instance id, e.g. "station-01"
}

// New creates a RedisRouter.
//   - rdb: connected go-redis client
//   - source: protocol Source for this engine instance
//   - station: station instance id (used to address command stream)
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

	// 3. Send to the station's command stream.
	streamKey := "commands:" + r.station
	if err := r.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		Values: map[string]interface{}{"message": string(data)},
	}).Err(); err != nil {
		return nil, fmt.Errorf("XADD %s: %w", streamKey, err)
	}

	// 4. Read response from our response stream with timeout.
	responseStream := "responses:" + r.source.Instance
	deadline := time.Duration(timeoutMs) * time.Millisecond

	readCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	// Poll for the correlated response.
	lastID := "0-0"
	for {
		streams, readErr := r.rdb.XRead(readCtx, &redis.XReadArgs{
			Streams: []string{responseStream, lastID},
			Count:   10,
			Block:   deadline,
		}).Result()
		if readErr != nil {
			return nil, fmt.Errorf("XREAD %s: %w", responseStream, readErr)
		}

		for _, stream := range streams {
			for _, entry := range stream.Messages {
				lastID = entry.ID

				raw, ok := entry.Values["message"]
				if !ok {
					continue
				}
				rawStr, ok := raw.(string)
				if !ok {
					continue
				}

				respMsg, parseErr := protocol.Parse([]byte(rawStr))
				if parseErr != nil {
					continue
				}

				// Match by correlation_id.
				if respMsg.Envelope.CorrelationID != correlationID {
					continue
				}

				// 5. Parse command response payload.
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
			}
		}
	}
}
