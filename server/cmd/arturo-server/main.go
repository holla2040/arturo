package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
	"github.com/redis/go-redis/v9"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "send" {
		fmt.Fprintf(os.Stderr, "Usage: arturo-server send --station STATION --device DEVICE --cmd CMD [--timeout MS] [--redis ADDR]\n")
		os.Exit(1)
	}

	sendFlags := flag.NewFlagSet("send", flag.ExitOnError)
	station := sendFlags.String("station", "", "station instance ID (required)")
	device := sendFlags.String("device", "", "device ID (required)")
	cmd := sendFlags.String("cmd", "", "command string (required)")
	timeout := sendFlags.Int("timeout", 5000, "timeout in milliseconds")
	redisAddr := sendFlags.String("redis", "localhost:6379", "Redis address")

	sendFlags.Parse(os.Args[2:])

	if *station == "" || *device == "" || *cmd == "" {
		fmt.Fprintf(os.Stderr, "Error: --station, --device, and --cmd are required\n")
		sendFlags.Usage()
		os.Exit(1)
	}

	source := protocol.Source{
		Service:  "arturo_server",
		Instance: "server-01",
		Version:  "1.0.0",
	}

	msg, err := protocol.BuildCommandRequest(source, *device, *cmd, nil, *timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building command request: %v\n", err)
		os.Exit(1)
	}

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling message: %v\n", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: *redisAddr,
	})
	defer rdb.Close()

	ctx := context.Background()

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to Redis at %s: %v\n", *redisAddr, err)
		os.Exit(1)
	}

	cmdStream := "commands:" + *station
	_, err = rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: cmdStream,
		Values: map[string]interface{}{"message": string(msgJSON)},
	}).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending command to %s: %v\n", cmdStream, err)
		os.Exit(1)
	}

	fmt.Printf("Sent command to %s\n", cmdStream)
	fmt.Printf("  device:         %s\n", *device)
	fmt.Printf("  command:        %s\n", *cmd)
	fmt.Printf("  correlation_id: %s\n", msg.Envelope.CorrelationID)
	fmt.Printf("  timeout:        %dms\n", *timeout)

	responseStream := msg.Envelope.ReplyTo
	timeoutDur := time.Duration(*timeout) * time.Millisecond

	resp, err := waitForResponse(ctx, rdb, responseStream, msg.Envelope.CorrelationID, timeoutDur)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	payload, err := protocol.ParseCommandResponse(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nResponse from %s\n", resp.Envelope.Source.Instance)
	fmt.Printf("  device:    %s\n", payload.DeviceID)
	fmt.Printf("  command:   %s\n", payload.CommandName)
	fmt.Printf("  success:   %v\n", payload.Success)

	if payload.Response != nil {
		fmt.Printf("  response:  %s\n", *payload.Response)
	}
	if payload.DurationMs != nil {
		fmt.Printf("  duration:  %dms\n", *payload.DurationMs)
	}
	if payload.Error != nil {
		fmt.Printf("  error:     [%s] %s\n", payload.Error.Code, payload.Error.Message)
	}

	if !payload.Success {
		os.Exit(1)
	}
}

func waitForResponse(ctx context.Context, rdb *redis.Client, stream, correlationID string, timeout time.Duration) (*protocol.Message, error) {
	deadline := time.Now().Add(timeout)
	lastID := "0-0"

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		result, err := rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{stream, lastID},
			Block:   remaining,
			Count:   10,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("XREAD error: %w", err)
		}

		for _, s := range result {
			for _, xmsg := range s.Messages {
				lastID = xmsg.ID
				jsonStr, ok := xmsg.Values["message"].(string)
				if !ok {
					continue
				}
				parsed, err := protocol.Parse([]byte(jsonStr))
				if err != nil {
					continue
				}
				if parsed.Envelope.CorrelationID == correlationID {
					return parsed, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("timeout waiting for response (correlation_id=%s)", correlationID)
}
