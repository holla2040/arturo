// Package console provides a web-based console for mock station control
// and Redis traffic monitoring.
package console

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/holla2040/arturo/internal/api"
	"github.com/holla2040/arturo/internal/mockpump"
	"github.com/holla2040/arturo/internal/protocol"
	"github.com/redis/go-redis/v9"
)

//go:embed index.html
var content embed.FS

// StationInfo pairs a mock station's identity with its pump simulator.
type StationInfo struct {
	Instance string
	DeviceID string
	Pump     *mockpump.Pump
	Online   *bool // shared pointer; toggled by console UI
}

// MonitorMessage is a parsed Redis message for the web UI.
type MonitorMessage struct {
	Timestamp string      `json:"timestamp"`
	Direction string      `json:"direction"` // "→" outgoing, "←" incoming, "♥" heartbeat, "!" e-stop
	Channel   string      `json:"channel"`
	Instance  string      `json:"instance"`
	Type      string      `json:"type"`    // message type from protocol
	Category  string      `json:"category"` // "command", "response", "heartbeat", "estop", "presence"
	Summary   string      `json:"summary"` // one-line formatted summary
	Raw       interface{} `json:"raw"`     // full parsed message
}

// stationSnapshot is the JSON shape returned by the stations API.
type stationSnapshot struct {
	Instance string                `json:"instance"`
	DeviceID string                `json:"device_id"`
	Online   bool                  `json:"online"`
	Pump     mockpump.PumpSnapshot `json:"pump"`
}

// Handler builds the HTTP handler and returns a run function that starts
// the Redis monitor goroutines. Call run in a goroutine.
func Handler(stations []*StationInfo, rdb *redis.Client) (http.Handler, func(ctx context.Context)) {
	hub := api.NewHub()

	mux := http.NewServeMux()

	// Serve embedded HTML
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data, err := content.ReadFile("index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// WebSocket endpoint
	mux.HandleFunc("GET /ws", hub.HandleWebSocket)

	// Station list
	mux.HandleFunc("GET /api/stations", func(w http.ResponseWriter, r *http.Request) {
		snapshots := make([]stationSnapshot, len(stations))
		for i, s := range stations {
			snapshots[i] = stationSnapshot{
				Instance: s.Instance,
				DeviceID: s.DeviceID,
				Online:   *s.Online,
				Pump:     s.Pump.Snapshot(),
			}
		}
		writeJSON(w, snapshots)
	})

	// Per-station routes
	for _, s := range stations {
		st := s // capture
		prefix := "/api/stations/" + st.Instance

		mux.HandleFunc("GET "+prefix, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, stationSnapshot{
				Instance: st.Instance,
				DeviceID: st.DeviceID,
				Online:   *st.Online,
				Pump:     st.Pump.Snapshot(),
			})
		})

		mux.HandleFunc("POST "+prefix+"/pump-on", func(w http.ResponseWriter, r *http.Request) {
			st.Pump.SetState(mockpump.StateCooling)
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/pump-off", func(w http.ResponseWriter, r *http.Request) {
			st.Pump.SetState(mockpump.StateOff)
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/start-regen", func(w http.ResponseWriter, r *http.Request) {
			st.Pump.SetState(mockpump.StateRegen)
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/abort-regen", func(w http.ResponseWriter, r *http.Request) {
			st.Pump.SetState(mockpump.StateOff)
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/advance-regen", func(w http.ResponseWriter, r *http.Request) {
			st.Pump.AdvanceRegenStep()
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/temperatures", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				FirstStageK  float64 `json:"first_stage_k"`
				SecondStageK float64 `json:"second_stage_k"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			st.Pump.SetTemperatures(body.FirstStageK, body.SecondStageK)
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/cooldown-hours", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Hours float64 `json:"hours"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := st.Pump.SetCooldownHours(body.Hours); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/fail-rate", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Rate float64 `json:"rate"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			st.Pump.SetFailRate(body.Rate)
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/rough-valve", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Open bool `json:"open"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			st.Pump.SetRoughValve(body.Open)
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/purge-valve", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Open bool `json:"open"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			st.Pump.SetPurgeValve(body.Open)
			writeJSON(w, map[string]string{"status": "ok"})
		})

		mux.HandleFunc("POST "+prefix+"/online", func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Online bool `json:"online"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			*st.Online = body.Online
			writeJSON(w, map[string]string{"status": "ok"})
		})
	}

	run := func(ctx context.Context) {
		go hub.Run(ctx)
		go watchStreams(ctx, rdb, hub)
		go watchPubSub(ctx, rdb, hub)
		go pollPresence(ctx, rdb, hub)
	}

	return mux, run
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// watchStreams monitors commands:* and responses:* streams.
func watchStreams(ctx context.Context, rdb *redis.Client, hub *api.Hub) {
	lastIDs := make(map[string]string)

	for {
		if ctx.Err() != nil {
			return
		}

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
			args.Streams = append(args.Streams, id)
		}

		results, err := rdb.XRead(ctx, args).Result()
		if err != nil {
			if err == redis.Nil || ctx.Err() != nil {
				continue
			}
			log.Printf("console: stream read error: %v", err)
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

				msg, err := parseStreamFields(fields)
				if err != nil {
					continue
				}

				direction := "→"
				category := "command"
				if strings.HasPrefix(stream.Stream, "responses:") {
					direction = "←"
					category = "response"
				}

				mm := buildMonitorMessage(msg, stream.Stream, direction, category)
				broadcastMonitor(hub, mm)
			}
		}
	}
}

// watchPubSub subscribes to events:* for heartbeats and e-stop.
func watchPubSub(ctx context.Context, rdb *redis.Client, hub *api.Hub) {
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
				continue
			}

			direction := "♥"
			category := "heartbeat"
			if msg.Envelope.Type == protocol.TypeSystemEmergencyStop {
				direction = "!"
				category = "estop"
			}

			mm := buildMonitorMessage(&msg, redisMsg.Channel, direction, category)
			broadcastMonitor(hub, mm)
		}
	}
}

// pollPresence scans device:*:alive keys periodically.
func pollPresence(ctx context.Context, rdb *redis.Client, hub *api.Hub) {
	poll := func() {
		iter := rdb.Scan(ctx, 0, "device:*:alive", 100).Iterator()
		for iter.Next(ctx) {
			key := iter.Val()
			instance := extractInstance(key)

			ttl, err := rdb.TTL(ctx, key).Result()
			if err != nil {
				continue
			}

			state := "online"
			if ttl.Seconds() < 5 {
				state = "stale"
			}

			mm := &MonitorMessage{
				Timestamp: time.Now().Format("15:04:05"),
				Direction: "●",
				Channel:   key,
				Instance:  instance,
				Type:      "presence",
				Category:  "presence",
				Summary:   fmt.Sprintf("%s TTL=%ds %s", instance, int(ttl.Seconds()), state),
			}
			broadcastMonitor(hub, mm)
		}
	}

	poll()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

// buildMonitorMessage creates a MonitorMessage from a parsed protocol message.
func buildMonitorMessage(msg *protocol.Message, channel, direction, category string) *MonitorMessage {
	instance := msg.Envelope.Source.Instance
	summary := ""

	switch msg.Envelope.Type {
	case protocol.TypeDeviceCommandRequest:
		req, err := protocol.ParseCommandRequest(msg)
		if err == nil {
			summary = fmt.Sprintf("cmd=%s device=%s", req.CommandName, req.DeviceID)
		}
	case protocol.TypeDeviceCommandResponse:
		resp, err := protocol.ParseCommandResponse(msg)
		if err == nil {
			val := ""
			if resp.Response != nil {
				val = *resp.Response
			}
			summary = fmt.Sprintf("success=%t response=%q", resp.Success, val)
		}
	case protocol.TypeServiceHeartbeat:
		hb, err := protocol.ParseHeartbeat(msg)
		if err == nil {
			summary = fmt.Sprintf("%s heap=%d rssi=%d", hb.Status, hb.FreeHeap, hb.WifiRSSI)
		}
	case protocol.TypeSystemEmergencyStop:
		es, err := protocol.ParseEmergencyStop(msg)
		if err == nil {
			summary = fmt.Sprintf("reason=%s initiator=%s", es.Reason, es.Initiator)
		}
	default:
		summary = msg.Envelope.Type
	}

	return &MonitorMessage{
		Timestamp: time.Now().Format("15:04:05"),
		Direction: direction,
		Channel:   channel,
		Instance:  instance,
		Type:      msg.Envelope.Type,
		Category:  category,
		Summary:   summary,
		Raw:       msg,
	}
}

func broadcastMonitor(hub *api.Hub, mm *MonitorMessage) {
	data, err := json.Marshal(mm)
	if err != nil {
		return
	}
	hub.Broadcast(data)
}

// parseStreamFields extracts a protocol.Message from Redis stream fields.
func parseStreamFields(fields map[string]string) (*protocol.Message, error) {
	data, ok := fields["data"]
	if !ok {
		for _, v := range fields {
			data = v
			break
		}
	}
	if data == "" {
		return nil, fmt.Errorf("no message data in stream fields")
	}
	var msg protocol.Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// extractInstance pulls the station instance from a Redis key like device:{instance}:alive.
func extractInstance(key string) string {
	parts := strings.Split(key, ":")
	if len(parts) >= 3 && parts[0] == "device" && parts[len(parts)-1] == "alive" {
		return strings.Join(parts[1:len(parts)-1], ":")
	}
	if len(parts) >= 2 && (parts[0] == "commands" || parts[0] == "responses") {
		return strings.Join(parts[1:], ":")
	}
	return key
}
