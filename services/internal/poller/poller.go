// Package poller provides background polling of station device status and temperatures.
package poller

import (
	"context"
	"log"
	"strconv"
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

// StationPoller periodically queries all online stations for pump status and temperatures.
type StationPoller struct {
	source     protocol.Source
	sender     api.CommandSender
	dispatcher *api.ResponseDispatcher
	registry   *registry.Registry
	hub        Broadcaster
	interval   time.Duration
}

// New creates a StationPoller with a 5-second interval.
func New(source protocol.Source, sender api.CommandSender, dispatcher *api.ResponseDispatcher, reg *registry.Registry, hub Broadcaster) *StationPoller {
	return &StationPoller{
		source:     source,
		sender:     sender,
		dispatcher: dispatcher,
		registry:   reg,
		hub:        hub,
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

func (p *StationPoller) pollDevice(ctx context.Context, stationInstance, deviceID string) {
	commandStream := "commands:" + stationInstance
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Poll status bytes
	s1 := p.queryCommand(ctx, deviceID, "get_status_1", commandStream)
	s2 := p.queryCommand(ctx, deviceID, "get_status_2", commandStream)
	s3 := p.queryCommand(ctx, deviceID, "get_status_3", commandStream)

	if s1 != nil {
		status1, _ := strconv.Atoi(*s1)
		status2 := 0
		if s2 != nil {
			status2, _ = strconv.Atoi(*s2)
		}
		status3 := 0
		if s3 != nil {
			status3, _ = strconv.Atoi(*s3)
		}

		// Poll valve states
		roughValveOpen := false
		if rv := p.queryCommand(ctx, deviceID, "get_rough_valve", commandStream); rv != nil {
			roughValveOpen = *rv == "1"
		}
		purgeValveOpen := false
		if pv := p.queryCommand(ctx, deviceID, "get_purge_valve", commandStream); pv != nil {
			purgeValveOpen = *pv == "1"
		}

		// Always poll regen status character — derive regen active from it
		// (status byte 1 bit 2 is purge valve, not regen)
		regenStatus := ""
		if rs := p.queryCommand(ctx, deviceID, "get_regen_status", commandStream); rs != nil {
			regenStatus = *rs
		}
		regenActive := regenStatus != "" && regenStatus != "A" && regenStatus != "P" && regenStatus != "V"

		p.hub.BroadcastEvent("pump_status", map[string]interface{}{
			"station_instance": stationInstance,
			"device_id":        deviceID,
			"status_1":         status1,
			"status_2":         status2,
			"status_3":         status3,
			"pump_on":          status1&1 != 0,
			"at_temp":          false, // TODO: derive from temperatures, not status byte
			"regen":            regenActive,
			"regen_status":     regenStatus,
			"rough_valve_open": roughValveOpen,
			"purge_valve_open": purgeValveOpen,
			"timestamp":        now,
		})
	}

	// Poll temperatures
	t1 := p.queryCommand(ctx, deviceID, "get_temp_1st_stage", commandStream)
	if t1 != nil {
		if tempK, err := strconv.ParseFloat(*t1, 64); err == nil {
			p.hub.BroadcastEvent("temperature", map[string]interface{}{
				"station_instance": stationInstance,
				"device_id":        deviceID,
				"stage":            "first_stage",
				"temperature_k":    tempK,
				"timestamp":        now,
			})
		}
	}

	t2 := p.queryCommand(ctx, deviceID, "get_temp_2nd_stage", commandStream)
	if t2 != nil {
		if tempK, err := strconv.ParseFloat(*t2, 64); err == nil {
			p.hub.BroadcastEvent("temperature", map[string]interface{}{
				"station_instance": stationInstance,
				"device_id":        deviceID,
				"stage":            "second_stage",
				"temperature_k":    tempK,
				"timestamp":        now,
			})
		}
	}

	if s1 != nil || t1 != nil {
		t1Str, t2Str := "--", "--"
		if t1 != nil {
			t1Str = *t1 + "K"
		}
		if t2 != nil {
			t2Str = *t2 + "K"
		}
		s1Str := "--"
		if s1 != nil {
			s1Str = *s1
		}
		log.Printf("poller: %s/%s S1=%s J=%s K=%s", stationInstance, deviceID, s1Str, t1Str, t2Str)
	}
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
		if err != nil || !payload.Success || payload.Response == nil {
			return nil
		}
		return payload.Response

	case <-time.After(5 * time.Second):
		p.dispatcher.Deregister(msg.Envelope.CorrelationID)
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
