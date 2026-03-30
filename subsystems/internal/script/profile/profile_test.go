package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// repoProfilesDir returns the absolute path to the profiles/ directory at the
// repository root. It computes this relative to the test source file location
// so that tests work regardless of working directory.
func repoProfilesDir(t *testing.T) string {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to determine test file location via runtime.Caller")
	}
	// testFile is .../server/internal/script/profile/profile_test.go
	// repo root is four levels up: profile -> script -> internal -> server -> repo
	repoRoot := filepath.Join(filepath.Dir(testFile), "..", "..", "..", "..")
	dir := filepath.Join(repoRoot, "profiles")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("profiles directory not found at %s: %v", dir, err)
	}
	return dir
}

// writeYAML is a helper that writes content to a file inside dir and returns the path.
func writeYAML(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestLoadProfile_SCPI(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "fluke_8846a.yaml", `
manufacturer: "Fluke"
model: "8846A"
type: "dmm"
protocol: "scpi"
packetizer:
  type: "scpi"
  line_ending: "\n"
commands:
  identify: "*IDN?"
  reset: "*RST"
  measure_dc_voltage: "MEAS:VOLT:DC?"
responses:
  success: '0,"No error"'
  error: "ERR"
`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error: %v", err)
	}

	if p.DeviceID != "fluke_8846a" {
		t.Errorf("DeviceID = %q, want %q", p.DeviceID, "fluke_8846a")
	}
	if p.Manufacturer != "Fluke" {
		t.Errorf("Manufacturer = %q, want %q", p.Manufacturer, "Fluke")
	}
	if p.Model != "8846A" {
		t.Errorf("Model = %q, want %q", p.Model, "8846A")
	}
	if p.Type != "dmm" {
		t.Errorf("Type = %q, want %q", p.Type, "dmm")
	}
	if p.Protocol != "scpi" {
		t.Errorf("Protocol = %q, want %q", p.Protocol, "scpi")
	}
	if p.Packetizer.Type != "scpi" {
		t.Errorf("Packetizer.Type = %q, want %q", p.Packetizer.Type, "scpi")
	}
	if p.Packetizer.LineEnding != "\n" {
		t.Errorf("Packetizer.LineEnding = %q, want %q", p.Packetizer.LineEnding, "\n")
	}
	if got := p.Commands["identify"]; got != "*IDN?" {
		t.Errorf("Commands[identify] = %q, want %q", got, "*IDN?")
	}
	if got := len(p.Commands); got != 3 {
		t.Errorf("len(Commands) = %d, want 3", got)
	}
	if got := p.Responses["error"]; got != "ERR" {
		t.Errorf("Responses[error] = %q, want %q", got, "ERR")
	}
	if p.Modbus != nil {
		t.Error("Modbus should be nil for SCPI device")
	}
}

func TestLoadProfile_Modbus(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "omega_cn7500.yaml", `
manufacturer: "Omega"
model: "CN7500"
type: "temperature_controller"
protocol: "modbus_rtu"
packetizer:
  type: "modbus"
  options:
    slave_address: 1
modbus:
  slave_address: 1
  registers:
    temperature: 0x1000
    setpoint: 0x1001
commands:
  read_temperature: "read_holding_registers"
responses: {}
`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error: %v", err)
	}

	if p.DeviceID != "omega_cn7500" {
		t.Errorf("DeviceID = %q, want %q", p.DeviceID, "omega_cn7500")
	}
	if p.Protocol != "modbus_rtu" {
		t.Errorf("Protocol = %q, want %q", p.Protocol, "modbus_rtu")
	}
	if p.Modbus == nil {
		t.Fatal("Modbus config should not be nil")
	}
	if p.Modbus.SlaveAddress != 1 {
		t.Errorf("Modbus.SlaveAddress = %d, want 1", p.Modbus.SlaveAddress)
	}
	if got, ok := p.Modbus.Registers["temperature"]; !ok || got != 0x1000 {
		t.Errorf("Modbus.Registers[temperature] = %d, ok=%v, want 0x1000", got, ok)
	}
	if got, ok := p.Modbus.Registers["setpoint"]; !ok || got != 0x1001 {
		t.Errorf("Modbus.Registers[setpoint] = %d, ok=%v, want 0x1001", got, ok)
	}
	if p.Packetizer.Options == nil {
		t.Fatal("Packetizer.Options should not be nil for modbus")
	}
}

func TestLoadProfile_ModbusWithCoils(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "modbus_tcp_device.yaml", `
manufacturer: "Generic"
model: "Modbus-TCP-Device"
type: "plc"
protocol: "modbus"
packetizer:
  type: "modbus"
  options:
    slave_address: 1
modbus:
  slave_address: 1
  registers:
    temperature: 0x0000
    humidity: 0x0001
  coils:
    output1: 0x0000
    output2: 0x0001
commands:
  read_temperature: "read_holding_registers"
responses: {}
`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error: %v", err)
	}

	if p.Modbus == nil {
		t.Fatal("Modbus config should not be nil")
	}
	if p.Modbus.Coils == nil {
		t.Fatal("Modbus.Coils should not be nil")
	}
	if got, ok := p.Modbus.Coils["output1"]; !ok || got != 0x0000 {
		t.Errorf("Modbus.Coils[output1] = %d, ok=%v, want 0", got, ok)
	}
	if got, ok := p.Modbus.Coils["output2"]; !ok || got != 0x0001 {
		t.Errorf("Modbus.Coils[output2] = %d, ok=%v, want 1", got, ok)
	}
}

func TestLoadProfile_CTI(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "cti_onboard.yaml", `
manufacturer: "CTI"
model: "On-Board Cryopump"
type: "cryopump"
protocol: "cti"
packetizer:
  type: "cti"
commands:
  pump_on: "$P{addr}A1{checksum}"
  pump_off: "$P{addr}A0{checksum}"
  pump_status: "$P{addr}A?{checksum}"
responses:
  ack: "$A"
  error: "$E"
`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error: %v", err)
	}

	if p.DeviceID != "cti_onboard" {
		t.Errorf("DeviceID = %q, want %q", p.DeviceID, "cti_onboard")
	}
	if p.Protocol != "cti" {
		t.Errorf("Protocol = %q, want %q", p.Protocol, "cti")
	}
	if p.Packetizer.Type != "cti" {
		t.Errorf("Packetizer.Type = %q, want %q", p.Packetizer.Type, "cti")
	}
	if got := p.Commands["pump_on"]; got != "$P{addr}A1{checksum}" {
		t.Errorf("Commands[pump_on] = %q, want %q", got, "$P{addr}A1{checksum}")
	}
	if got := len(p.Commands); got != 3 {
		t.Errorf("len(Commands) = %d, want 3", got)
	}
	if got := p.Responses["ack"]; got != "$A" {
		t.Errorf("Responses[ack] = %q, want %q", got, "$A")
	}
	if p.Modbus != nil {
		t.Error("Modbus should be nil for CTI device")
	}
}

func TestLoadProfile_ASCII(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "arduino_uno.yaml", `
manufacturer: "Arduino"
model: "Uno"
type: "microcontroller"
protocol: "ascii"
packetizer:
  type: "ascii"
  line_ending: "\n"
commands:
  digital_write: "DW{pin},{value}"
  digital_read: "DR{pin}?"
  identify: "ID?"
responses:
  success: "OK"
  error: "ERR"
`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error: %v", err)
	}

	if p.Protocol != "ascii" {
		t.Errorf("Protocol = %q, want %q", p.Protocol, "ascii")
	}
	if p.Packetizer.Type != "ascii" {
		t.Errorf("Packetizer.Type = %q, want %q", p.Packetizer.Type, "ascii")
	}
	if got := p.Commands["digital_write"]; got != "DW{pin},{value}" {
		t.Errorf("Commands[digital_write] = %q, want %q", got, "DW{pin},{value}")
	}
	if got := p.Responses["success"]; got != "OK" {
		t.Errorf("Responses[success] = %q, want %q", got, "OK")
	}
}

func TestLoadProfile_MissingFile(t *testing.T) {
	_, err := LoadProfile("/nonexistent/path/device.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadProfile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "bad.yaml", `
manufacturer: "Test"
model: [[[invalid yaml unclosed
commands: {
`)

	_, err := LoadProfile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadProfile_DeviceIDFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		wantID   string
	}{
		{"fluke_8846a.yaml", "fluke_8846a"},
		{"rigol_dp832.yaml", "rigol_dp832"},
		{"cti_onboard.yml", "cti_onboard"},
		{"device.YAML", "device"},
		{"multi.dot.name.yaml", "multi.dot.name"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			dir := t.TempDir()
			path := writeYAML(t, dir, tt.filename, `
manufacturer: "Test"
model: "Test"
type: "test"
protocol: "test"
packetizer:
  type: "test"
commands:
  test: "test"
`)

			p, err := LoadProfile(path)
			if err != nil {
				t.Fatalf("LoadProfile() error: %v", err)
			}
			if p.DeviceID != tt.wantID {
				t.Errorf("DeviceID = %q, want %q", p.DeviceID, tt.wantID)
			}
		})
	}
}

func TestLoadProfile_NoResponses(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "minimal.yaml", `
manufacturer: "Test"
model: "Minimal"
type: "test"
protocol: "test"
packetizer:
  type: "test"
commands:
  ping: "PING"
`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error: %v", err)
	}

	if p.Responses != nil && len(p.Responses) != 0 {
		t.Errorf("Responses should be nil or empty, got %v", p.Responses)
	}
	if got := p.Commands["ping"]; got != "PING" {
		t.Errorf("Commands[ping] = %q, want %q", got, "PING")
	}
}

func TestLoadProfile_EmptyResponses(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "empty_resp.yaml", `
manufacturer: "Test"
model: "EmptyResp"
type: "test"
protocol: "test"
packetizer:
  type: "test"
commands:
  cmd: "CMD"
responses: {}
`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error: %v", err)
	}

	if len(p.Responses) != 0 {
		t.Errorf("Responses should be empty, got %v", p.Responses)
	}
}

func TestLoadAllProfiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	profiles, err := LoadAllProfiles(dir)
	if err != nil {
		t.Fatalf("LoadAllProfiles() error: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected empty slice, got %d profiles", len(profiles))
	}
}

func TestLoadAllProfiles_SkipsNonYAML(t *testing.T) {
	dir := t.TempDir()

	writeYAML(t, dir, "device.yaml", `
manufacturer: "Test"
model: "Device"
type: "test"
protocol: "test"
packetizer:
  type: "test"
commands:
  cmd: "CMD"
`)
	// Write a non-YAML file that should be skipped.
	writeYAML(t, dir, "README.md", "# Not a YAML profile")
	writeYAML(t, dir, "notes.txt", "some notes")

	profiles, err := LoadAllProfiles(dir)
	if err != nil {
		t.Fatalf("LoadAllProfiles() error: %v", err)
	}
	if len(profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(profiles))
	}
}

func TestLoadAllProfiles_SortedByDeviceID(t *testing.T) {
	dir := t.TempDir()

	base := `
manufacturer: "Test"
model: "Test"
type: "test"
protocol: "test"
packetizer:
  type: "test"
commands:
  cmd: "CMD"
`
	writeYAML(t, dir, "zebra.yaml", base)
	writeYAML(t, dir, "alpha.yaml", base)
	writeYAML(t, dir, "middle.yaml", base)

	profiles, err := LoadAllProfiles(dir)
	if err != nil {
		t.Fatalf("LoadAllProfiles() error: %v", err)
	}

	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}
	want := []string{"alpha", "middle", "zebra"}
	for i, p := range profiles {
		if p.DeviceID != want[i] {
			t.Errorf("profiles[%d].DeviceID = %q, want %q", i, p.DeviceID, want[i])
		}
	}
}

func TestLoadAllProfiles_Recursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	base := `
manufacturer: "Test"
model: "Test"
type: "test"
protocol: "test"
packetizer:
  type: "test"
commands:
  cmd: "CMD"
`
	writeYAML(t, dir, "top_level.yaml", base)
	writeYAML(t, sub, "nested.yaml", base)

	profiles, err := LoadAllProfiles(dir)
	if err != nil {
		t.Fatalf("LoadAllProfiles() error: %v", err)
	}

	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}

	ids := make([]string, len(profiles))
	for i, p := range profiles {
		ids[i] = p.DeviceID
	}
	sort.Strings(ids)
	if ids[0] != "nested" || ids[1] != "top_level" {
		t.Errorf("unexpected device IDs: %v", ids)
	}
}

func TestLoadAllProfiles_NonexistentDir(t *testing.T) {
	_, err := LoadAllProfiles("/nonexistent/directory/path")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

func TestLoadAllProfiles_ActualProfiles(t *testing.T) {
	dir := repoProfilesDir(t)

	profiles, err := LoadAllProfiles(dir)
	if err != nil {
		t.Fatalf("LoadAllProfiles() error: %v", err)
	}

	// We know there are 8 YAML files in the profiles directory.
	if len(profiles) < 8 {
		t.Errorf("expected at least 8 profiles, got %d", len(profiles))
	}

	// Verify sorted order.
	for i := 1; i < len(profiles); i++ {
		if profiles[i].DeviceID < profiles[i-1].DeviceID {
			t.Errorf("profiles not sorted: %q comes after %q",
				profiles[i].DeviceID, profiles[i-1].DeviceID)
		}
	}

	// Verify known profiles are present.
	found := make(map[string]bool)
	for _, p := range profiles {
		found[p.DeviceID] = true
	}
	want := []string{
		"fluke_8846a",
		"rigol_dp832",
		"keysight_34461a",
		"omega_cn7500",
		"arduino_uno",
		"cti_onboard",
		"usb_relay_8ch",
		"modbus_tcp_device",
	}
	for _, id := range want {
		if !found[id] {
			t.Errorf("expected profile %q not found", id)
		}
	}
}

func TestLoadActualProfile_Fluke(t *testing.T) {
	dir := repoProfilesDir(t)
	path := filepath.Join(dir, "testequipment", "fluke_8846a.yaml")

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile() error: %v", err)
	}

	if p.Manufacturer != "Fluke" {
		t.Errorf("Manufacturer = %q, want %q", p.Manufacturer, "Fluke")
	}
	if p.Model != "8846A" {
		t.Errorf("Model = %q, want %q", p.Model, "8846A")
	}
	if p.Protocol != "scpi" {
		t.Errorf("Protocol = %q, want %q", p.Protocol, "scpi")
	}
	if _, ok := p.Commands["identify"]; !ok {
		t.Error("expected 'identify' command")
	}
	if _, ok := p.Commands["measure_dc_voltage"]; !ok {
		t.Error("expected 'measure_dc_voltage' command")
	}
}

func TestBuildIntrospection(t *testing.T) {
	profiles := []*DeviceProfile{
		{
			DeviceID:     "device_b",
			Manufacturer: "MfgB",
			Model:        "ModelB",
			Type:         "typeB",
			Protocol:     "scpi",
			Commands:     map[string]string{"zebra": "Z", "alpha": "A", "middle": "M"},
		},
		{
			DeviceID:     "device_a",
			Manufacturer: "MfgA",
			Model:        "ModelA",
			Type:         "typeA",
			Protocol:     "ascii",
			Commands:     map[string]string{"cmd1": "C1"},
		},
	}

	intro := BuildIntrospection(profiles)

	if intro.Count != 2 {
		t.Errorf("Count = %d, want 2", intro.Count)
	}
	if len(intro.Devices) != 2 {
		t.Fatalf("len(Devices) = %d, want 2", len(intro.Devices))
	}

	// Devices preserve input order (they were pre-sorted by LoadAllProfiles).
	if intro.Devices[0].DeviceID != "device_b" {
		t.Errorf("Devices[0].DeviceID = %q, want %q", intro.Devices[0].DeviceID, "device_b")
	}
	if intro.Devices[1].DeviceID != "device_a" {
		t.Errorf("Devices[1].DeviceID = %q, want %q", intro.Devices[1].DeviceID, "device_a")
	}

	// Commands within device_b should be sorted.
	wantCmds := []string{"alpha", "middle", "zebra"}
	gotCmds := intro.Devices[0].Commands
	if len(gotCmds) != len(wantCmds) {
		t.Fatalf("device_b commands len = %d, want %d", len(gotCmds), len(wantCmds))
	}
	for i, cmd := range gotCmds {
		if cmd != wantCmds[i] {
			t.Errorf("device_b commands[%d] = %q, want %q", i, cmd, wantCmds[i])
		}
	}

	// Verify other fields propagated.
	if intro.Devices[0].Manufacturer != "MfgB" {
		t.Errorf("Devices[0].Manufacturer = %q, want %q", intro.Devices[0].Manufacturer, "MfgB")
	}
	if intro.Devices[1].Protocol != "ascii" {
		t.Errorf("Devices[1].Protocol = %q, want %q", intro.Devices[1].Protocol, "ascii")
	}
}

func TestBuildIntrospection_Empty(t *testing.T) {
	intro := BuildIntrospection(nil)

	if intro.Count != 0 {
		t.Errorf("Count = %d, want 0", intro.Count)
	}
	if len(intro.Devices) != 0 {
		t.Errorf("len(Devices) = %d, want 0", len(intro.Devices))
	}
}

func TestBuildIntrospection_FromActualProfiles(t *testing.T) {
	dir := repoProfilesDir(t)

	profiles, err := LoadAllProfiles(dir)
	if err != nil {
		t.Fatalf("LoadAllProfiles() error: %v", err)
	}

	intro := BuildIntrospection(profiles)

	if intro.Count != len(profiles) {
		t.Errorf("Count = %d, want %d", intro.Count, len(profiles))
	}

	// Verify each device has at least one command.
	for _, d := range intro.Devices {
		if len(d.Commands) == 0 {
			t.Errorf("device %q has no commands", d.DeviceID)
		}
		// Verify commands are sorted within each device.
		if !sort.StringsAreSorted(d.Commands) {
			t.Errorf("device %q commands not sorted: %v", d.DeviceID, d.Commands)
		}
	}
}

func TestAllProtocolTypes(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		yaml     string
	}{
		{
			name:     "scpi",
			protocol: "scpi",
			yaml: `
manufacturer: "Test"
model: "SCPI"
type: "test"
protocol: "scpi"
packetizer:
  type: "scpi"
  line_ending: "\n"
commands:
  identify: "*IDN?"
responses:
  success: "OK"
`,
		},
		{
			name:     "ascii",
			protocol: "ascii",
			yaml: `
manufacturer: "Test"
model: "ASCII"
type: "test"
protocol: "ascii"
packetizer:
  type: "ascii"
  line_ending: "\n"
commands:
  cmd: "CMD"
responses:
  success: "OK"
`,
		},
		{
			name:     "modbus_rtu",
			protocol: "modbus_rtu",
			yaml: `
manufacturer: "Test"
model: "Modbus"
type: "test"
protocol: "modbus_rtu"
packetizer:
  type: "modbus"
  options:
    slave_address: 1
modbus:
  slave_address: 1
  registers:
    reg1: 0x0001
commands:
  read: "read_holding_registers"
responses: {}
`,
		},
		{
			name:     "modbus_tcp",
			protocol: "modbus",
			yaml: `
manufacturer: "Test"
model: "ModbusTCP"
type: "test"
protocol: "modbus"
packetizer:
  type: "modbus"
  options:
    slave_address: 1
modbus:
  slave_address: 1
  registers:
    reg1: 0x0000
  coils:
    coil1: 0x0000
commands:
  read: "read_holding_registers"
responses: {}
`,
		},
		{
			name:     "cti",
			protocol: "cti",
			yaml: `
manufacturer: "Test"
model: "CTI"
type: "test"
protocol: "cti"
packetizer:
  type: "cti"
commands:
  pump_on: "$P{addr}A1{checksum}"
responses:
  ack: "$A"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeYAML(t, dir, tt.name+".yaml", tt.yaml)

			p, err := LoadProfile(path)
			if err != nil {
				t.Fatalf("LoadProfile() error: %v", err)
			}
			if p.Protocol != tt.protocol {
				t.Errorf("Protocol = %q, want %q", p.Protocol, tt.protocol)
			}
			if len(p.Commands) == 0 {
				t.Error("expected at least one command")
			}
		})
	}
}
