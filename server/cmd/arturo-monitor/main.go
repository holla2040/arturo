package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
	"github.com/redis/go-redis/v9"
)

func main() {
	streams := flag.Bool("streams", false, "show only stream messages")
	pubsub := flag.Bool("pubsub", false, "show only pub/sub messages")
	presence := flag.Bool("presence", false, "show only presence keys")
	station := flag.String("station", "", "filter to one station instance")
	msgType := flag.String("type", "", "filter by message type prefix (e.g. device.command.*)")
	corr := flag.String("corr", "", "track one correlation ID")
	jsonOut := flag.Bool("json", false, "raw JSON output")
	logFile := flag.String("log", "", "path to JSONL log file")
	flag.Parse()

	// If no mode flags set, show everything
	showAll := !*streams && !*pubsub && !*presence
	showStreams := showAll || *streams
	showPubSub := showAll || *pubsub
	showPresence := showAll || *presence

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisURL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Verify Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("cannot connect to Redis at %s: %v", redisURL, err)
	}

	// Open log file if requested
	var logWriter *os.File
	if *logFile != "" {
		var err error
		logWriter, err = os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("cannot open log file: %v", err)
		}
		defer logWriter.Close()
	}

	// Correlation tracking: map correlation_id -> request timestamp
	corrTracker := struct {
		sync.Mutex
		m map[string]time.Time
	}{m: make(map[string]time.Time)}

	// Shared channel for all display messages
	displayCh := make(chan *DisplayMessage, 256)

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nshutting down...")
		cancel()
	}()

	var wg sync.WaitGroup

	// Goroutine 1: Stream watcher
	if showStreams {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Track last IDs per stream
			lastIDs := make(map[string]string)

			for {
				if ctx.Err() != nil {
					return
				}

				// SCAN for command and response streams
				var streamKeys []string
				iter := rdb.Scan(ctx, 0, "commands:*", 100).Iterator()
				for iter.Next(ctx) {
					streamKeys = append(streamKeys, iter.Val())
				}
				iter = rdb.Scan(ctx, 0, "responses:*", 100).Iterator()
				for iter.Next(ctx) {
					streamKeys = append(streamKeys, iter.Val())
				}

				if len(streamKeys) == 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Second):
						continue
					}
				}

				// Build XREAD args
				args := &redis.XReadArgs{
					Block:   time.Second,
					Count:   100,
					Streams: make([]string, 0, len(streamKeys)*2),
				}
				for _, key := range streamKeys {
					args.Streams = append(args.Streams, key)
				}
				for _, key := range streamKeys {
					id, ok := lastIDs[key]
					if !ok {
						id = "$"
					}
					_ = key
					args.Streams = append(args.Streams, id)
				}

				results, err := rdb.XRead(ctx, args).Result()
				if err != nil {
					if err == redis.Nil || ctx.Err() != nil {
						continue
					}
					log.Printf("stream read error: %v", err)
					continue
				}

				for _, stream := range results {
					for _, xmsg := range stream.Messages {
						lastIDs[stream.Stream] = xmsg.ID

						fields := make(map[string]string)
						for k, v := range xmsg.Values {
							if s, ok := v.(string); ok {
								fields[k] = s
							}
						}

						msg, err := ParseStreamFields(fields)
						if err != nil {
							log.Printf("parse error on %s/%s: %v", stream.Stream, xmsg.ID, err)
							continue
						}

						direction := "\u2192"
						if strings.HasPrefix(stream.Stream, "responses:") {
							direction = "\u2190"
						}

						dm := &DisplayMessage{
							Timestamp: time.Now(),
							Channel:   stream.Stream,
							Direction: direction,
							Message:   msg,
							StreamID:  xmsg.ID,
						}

						select {
						case displayCh <- dm:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}()
	}

	// Goroutine 2: PubSub watcher
	if showPubSub {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub := rdb.PSubscribe(ctx, "events:*")
			defer sub.Close()

			ch := sub.Channel()
			for {
				select {
				case <-ctx.Done():
					return
				case redisMsg, ok := <-ch:
					if !ok {
						return
					}

					var msg protocol.Message
					if err := json.Unmarshal([]byte(redisMsg.Payload), &msg); err != nil {
						log.Printf("pubsub parse error on %s: %v", redisMsg.Channel, err)
						continue
					}

					dm := &DisplayMessage{
						Timestamp: time.Now(),
						Channel:   redisMsg.Channel,
						Direction: "\u2190",
						Message:   &msg,
					}

					select {
					case displayCh <- dm:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Goroutine 3: Presence poller
	if showPresence {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()

			// Run once immediately, then on ticker
			pollPresence(ctx, rdb)

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					pollPresence(ctx, rdb)
				}
			}
		}()
	}

	// Close display channel when all producers are done
	go func() {
		wg.Wait()
		close(displayCh)
	}()

	// Main loop: read from channel, apply filters, format, print
	for dm := range displayCh {
		// Apply station filter
		if *station != "" && dm.Message.Envelope.Source.Instance != *station {
			continue
		}

		// Apply message type filter
		if *msgType != "" && !matchTypeFilter(dm.Message.Envelope.Type, *msgType) {
			continue
		}

		// Apply correlation ID filter
		if *corr != "" && dm.Message.Envelope.CorrelationID != *corr {
			continue
		}

		// Track correlation IDs
		if dm.Message.Envelope.CorrelationID != "" {
			corrTracker.Lock()
			if dm.Message.Envelope.Type == protocol.TypeDeviceCommandRequest {
				corrTracker.m[dm.Message.Envelope.CorrelationID] = dm.Timestamp
			} else if dm.Message.Envelope.Type == protocol.TypeDeviceCommandResponse {
				if reqTime, ok := corrTracker.m[dm.Message.Envelope.CorrelationID]; ok {
					elapsed := dm.Timestamp.Sub(reqTime)
					fmt.Fprintf(os.Stderr, "  [corr %s round-trip: %s]\n", dm.Message.Envelope.CorrelationID[:8], elapsed.Round(time.Millisecond))
					delete(corrTracker.m, dm.Message.Envelope.CorrelationID)
				}
			}
			corrTracker.Unlock()
		}

		// Output
		if *jsonOut {
			data, err := json.Marshal(dm.Message)
			if err != nil {
				log.Printf("json marshal error: %v", err)
				continue
			}
			fmt.Println(string(data))
		} else {
			fmt.Println(FormatMessage(dm))
		}

		// Log to JSONL file
		if logWriter != nil {
			data, err := json.Marshal(dm.Message)
			if err == nil {
				logWriter.Write(data)
				logWriter.Write([]byte("\n"))
			}
		}
	}
}

// matchTypeFilter checks if msgType matches a filter pattern.
// Supports wildcard "*" at the end (e.g., "device.command.*" matches "device.command.request").
func matchTypeFilter(msgType, filter string) bool {
	if strings.HasSuffix(filter, "*") {
		prefix := strings.TrimSuffix(filter, "*")
		return strings.HasPrefix(msgType, prefix)
	}
	return msgType == filter
}

// pollPresence scans for device:*:alive keys and prints their status.
func pollPresence(ctx context.Context, rdb *redis.Client) {
	iter := rdb.Scan(ctx, 0, "device:*:alive", 100).Iterator()
	found := false
	for iter.Next(ctx) {
		key := iter.Val()
		ttl, err := rdb.TTL(ctx, key).Result()
		if err != nil {
			log.Printf("presence TTL error for %s: %v", key, err)
			continue
		}
		fmt.Printf("[presence] %s\n", FormatPresence(key, int64(ttl.Seconds())))
		found = true
	}
	if !found {
		fmt.Println("[presence] no stations detected")
	}
}
