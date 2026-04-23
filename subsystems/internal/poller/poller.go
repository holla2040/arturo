// Package poller provides background polling of station device status and temperatures.
package poller

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/holla2040/arturo/internal/api"
	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/registry"
)

// Broadcaster sends events to connected clients (e.g., WebSocket).
type Broadcaster interface {
	BroadcastEvent(eventType string, payload interface{})
}

// TempRecorder persists temperature readings and pump status (e.g., to SQLite).
type TempRecorder interface {
	RecordTemperatureLog(stationInstance, deviceID, stage string, temperatureK float64) error
	RecordPumpStatusLog(stationInstance, deviceID string, pumpOn, roughValveOpen, purgeValveOpen bool, regenStatus string) error
}

// StationPoller periodically queries all online stations for pump status and temperatures.
type StationPoller struct {
	source     protocol.Source
	sender     api.CommandSender
	dispatcher *api.ResponseDispatcher
	registry   *registry.Registry
	hub        Broadcaster
	recorder   TempRecorder
	interval   time.Duration
}

// New creates a StationPoller with a 5-second interval.
func New(source protocol.Source, sender api.CommandSender, dispatcher *api.ResponseDispatcher, reg *registry.Registry, hub Broadcaster, recorder TempRecorder) *StationPoller {
	return &StationPoller{
		source:     source,
		sender:     sender,
		dispatcher: dispatcher,
		registry:   reg,
		hub:        hub,
		recorder:   recorder,
		interval:   5 * time.Second,
	}
}

// Run starts the polling loop. It blocks until ctx is cancelled.
func (p *StationPoller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

func (p *StationPoller) pollAll(ctx context.Context) {
	stations := p.registry.ListStations()
	for _, station := range stations {
		if station.Status != registry.StatusOnline {
			continue
		}
		for _, deviceID := range station.Devices {
			if !isPumpDevice(deviceID) {
				continue
			}
			p.pollDevice(ctx, station.Instance, deviceID)
		}
	}
}

// telemetrySnapshot matches the JSON object returned by the firmware's
// get_telemetry command. See docs/SCRIPTING_HAL.md "Telemetry Snapshot".
type telemetrySnapshot struct {
	Stage1TempK    float64 `json:"stage1_temp_k"`
	Stage2TempK    float64 `json:"stage2_temp_k"`
	PressureTorr   float64 `json:"pressure_torr"`
	PumpOn         bool    `json:"pump_on"`
	RoughValveOpen bool    `json:"rough_valve_open"`
	PurgeValveOpen bool    `json:"purge_valve_open"`
	RegenChar      string  `json:"regen_char"`
	OperatingHours int     `json:"operating_hours"`
	Status1        int     `json:"status_1"`
	StaleCount     int     `json:"stale_count"`
	LastUpdateMs   uint32  `json:"last_update_ms"`
}

func (p *StationPoller) pollDevice(ctx context.Context, stationInstance, deviceID string) {
	commandStream := "commands:" + stationInstance
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// One cache-served round-trip replaces 8 individual CTI queries. The
	// firmware serves this from the RAM snapshot its own poll task maintains.
	// See docs/architecture/ARCHITECTURE.md §4.6.
	raw := p.queryCommand(ctx, deviceID, "get_telemetry", commandStream)
	if raw == nil {
		log.Printf("poller: %s/%s get_telemetry returned nil (timeout or error response)",
			stationInstance, deviceID)
		return
	}

	var snap telemetrySnapshot
	if err := json.Unmarshal([]byte(*raw), &snap); err != nil {
		log.Printf("poller: %s/%s telemetry parse error: %v (raw=%q)",
			stationInstance, deviceID, err, *raw)
		return
	}

	// regen is active whenever the regen state character is not pump-off ('A'),
	// regen-complete ('P'), or regen-aborted ('V'). Matches the pre-telemetry
	// derivation at the old poller call site.
	regenActive := snap.RegenChar != "" && snap.RegenChar != "A" &&
		snap.RegenChar != "P" && snap.RegenChar != "V"

	// status_1_raw: single byte with the same bit pattern as Status1, to
	// preserve WS event shape for any client that inspects it.
	status1Raw := string([]byte{byte(snap.Status1)})

	p.hub.BroadcastEvent("pump_status", map[string]interface{}{
		"station_instance": stationInstance,
		"device_id":        deviceID,
		"status_1":         snap.Status1,
		"status_2":         0, // un-cached; see docs/SCRIPTING_HAL.md
		"status_3":         0, // un-cached
		"status_1_raw":     status1Raw,
		"pump_on":          snap.PumpOn,
		"at_temp":          false, // TODO: derive from temperatures, not status byte
		"regen":            regenActive,
		"regen_status":     snap.RegenChar,
		"rough_valve_open": snap.RoughValveOpen,
		"purge_valve_open": snap.PurgeValveOpen,
		"timestamp":        now,
	})

	if p.recorder != nil {
		p.recorder.RecordPumpStatusLog(stationInstance, deviceID,
			snap.PumpOn, snap.RoughValveOpen, snap.PurgeValveOpen, snap.RegenChar)
	}

	p.hub.BroadcastEvent("temperature", map[string]interface{}{
		"station_instance": stationInstance,
		"device_id":        deviceID,
		"stage":            "first_stage",
		"temperature_k":    snap.Stage1TempK,
		"timestamp":        now,
	})
	if p.recorder != nil {
		p.recorder.RecordTemperatureLog(stationInstance, deviceID, "first_stage", snap.Stage1TempK)
	}

	p.hub.BroadcastEvent("temperature", map[string]interface{}{
		"station_instance": stationInstance,
		"device_id":        deviceID,
		"stage":            "second_stage",
		"temperature_k":    snap.Stage2TempK,
		"timestamp":        now,
	})
	if p.recorder != nil {
		p.recorder.RecordTemperatureLog(stationInstance, deviceID, "second_stage", snap.Stage2TempK)
	}

	log.Printf("poller: %s/%s S1=0x%02X J=%.1fK K=%.1fK regen=%s",
		stationInstance, deviceID, snap.Status1, snap.Stage1TempK, snap.Stage2TempK, snap.RegenChar)
}

// queryCommand sends a single command and waits for the response.
// Returns the response string on success, nil on failure.
func (p *StationPoller) queryCommand(ctx context.Context, deviceID, command, stream string) *string {
	msg, err := protocol.BuildCommandRequest(p.source, deviceID, command, nil, 5000)
	if err != nil {
		return nil
	}

	waiterCh := p.dispatcher.Register(msg.Envelope.CorrelationID)

	if err := p.sender.SendCommand(ctx, stream, msg); err != nil {
		p.dispatcher.Deregister(msg.Envelope.CorrelationID)
		return nil
	}

	select {
	case resp := <-waiterCh:
		payload, err := protocol.ParseCommandResponse(resp)
		if err != nil {
			log.Printf("poller: %s parse response error: %v", command, err)
			return nil
		}
		if !payload.Success {
			if payload.Error != nil {
				log.Printf("poller: %s returned error code=%q message=%q",
					command, payload.Error.Code, payload.Error.Message)
			} else {
				log.Printf("poller: %s returned success=false with no error object", command)
			}
			return nil
		}
		if payload.Response == nil {
			log.Printf("poller: %s returned success with nil response field", command)
			return nil
		}
		return payload.Response

	case <-time.After(5 * time.Second):
		p.dispatcher.Deregister(msg.Envelope.CorrelationID)
		log.Printf("poller: %s timed out after 5s", command)
		return nil

	case <-ctx.Done():
		p.dispatcher.Deregister(msg.Envelope.CorrelationID)
		return nil
	}
}

// isPumpDevice returns true if the device ID looks like a CTI cryopump.
func isPumpDevice(deviceID string) bool {
	id := strings.ToUpper(deviceID)
	return strings.HasPrefix(id, "PUMP-") || strings.HasPrefix(id, "CTI-")
}

