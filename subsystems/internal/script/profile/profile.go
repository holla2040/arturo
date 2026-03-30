// Package profile loads and parses device YAML profiles for the Arturo script engine.
//
// Device profiles describe instrument protocol, connection parameters, and command
// vocabulary. They live in the profiles/ directory at the repository root, organized
// by device category (testequipment, controllers, pumps, relays, modbus).
//
// Four protocol variants are supported: SCPI, ASCII, Modbus (RTU/TCP), and CTI.
// Each profile is a single YAML file that maps to a DeviceProfile struct.
//
// BuildIntrospection converts loaded profiles into a structured JSON representation
// suitable for LLM consumption, providing device discovery and command enumeration.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// PacketizerConfig describes how to frame messages for a device's protocol.
type PacketizerConfig struct {
	Type       string                 `yaml:"type" json:"type"`
	LineEnding string                 `yaml:"line_ending,omitempty" json:"line_ending,omitempty"`
	Options    map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
}

// ModbusConfig holds Modbus-specific addressing and register maps.
type ModbusConfig struct {
	SlaveAddress int            `yaml:"slave_address" json:"slave_address"`
	Registers    map[string]int `yaml:"registers,omitempty" json:"registers,omitempty"`
	Coils        map[string]int `yaml:"coils,omitempty" json:"coils,omitempty"`
}

// DeviceProfile represents a single device's configuration loaded from YAML.
type DeviceProfile struct {
	Manufacturer string            `yaml:"manufacturer" json:"manufacturer"`
	Model        string            `yaml:"model" json:"model"`
	Type         string            `yaml:"type" json:"type"`
	Protocol     string            `yaml:"protocol" json:"protocol"`
	Packetizer   PacketizerConfig  `yaml:"packetizer" json:"packetizer"`
	Modbus       *ModbusConfig     `yaml:"modbus,omitempty" json:"modbus,omitempty"`
	Commands     map[string]string `yaml:"commands" json:"commands"`
	Responses    map[string]string `yaml:"responses,omitempty" json:"responses,omitempty"`

	// DeviceID is derived from the filename (extension stripped), not from YAML.
	DeviceID string `yaml:"-" json:"device_id"`
}

// DeviceIntrospection is an LLM-friendly summary of all loaded device profiles.
type DeviceIntrospection struct {
	Devices []DeviceInfo `json:"devices"`
	Count   int          `json:"count"`
}

// DeviceInfo is a summary of a single device for introspection.
type DeviceInfo struct {
	DeviceID     string   `json:"device_id"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
	Type         string   `json:"type"`
	Protocol     string   `json:"protocol"`
	Commands     []string `json:"commands"`
}

// LoadProfile reads and parses a single YAML profile file.
// The DeviceID is derived from the filename with the extension stripped
// (e.g., "fluke_8846a.yaml" becomes "fluke_8846a").
func LoadProfile(path string) (*DeviceProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile %s: %w", path, err)
	}

	var p DeviceProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile %s: %w", path, err)
	}

	// Derive DeviceID from filename without extension.
	base := filepath.Base(path)
	p.DeviceID = strings.TrimSuffix(base, filepath.Ext(base))

	return &p, nil
}

// LoadAllProfiles walks a directory recursively, loads all .yaml files, and
// returns them sorted by DeviceID for deterministic output. Non-YAML files
// are silently skipped.
func LoadAllProfiles(dir string) ([]*DeviceProfile, error) {
	var profiles []*DeviceProfile

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walking %s: %w", path, err)
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		p, err := LoadProfile(path)
		if err != nil {
			return err
		}

		profiles = append(profiles, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("loading profiles from %s: %w", dir, err)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].DeviceID < profiles[j].DeviceID
	})

	return profiles, nil
}

// BuildIntrospection converts a slice of DeviceProfile into a DeviceIntrospection
// structure suitable for LLM consumption. Command names are sorted alphabetically
// within each device.
func BuildIntrospection(profiles []*DeviceProfile) *DeviceIntrospection {
	devices := make([]DeviceInfo, 0, len(profiles))

	for _, p := range profiles {
		commands := make([]string, 0, len(p.Commands))
		for name := range p.Commands {
			commands = append(commands, name)
		}
		sort.Strings(commands)

		devices = append(devices, DeviceInfo{
			DeviceID:     p.DeviceID,
			Manufacturer: p.Manufacturer,
			Model:        p.Model,
			Type:         p.Type,
			Protocol:     p.Protocol,
			Commands:     commands,
		})
	}

	return &DeviceIntrospection{
		Devices: devices,
		Count:   len(devices),
	}
}
