// Package regen holds shared helpers for CTI regeneration state semantics.
package regen

// StateName maps a CTI O-command response letter (returned by
// get_regen_status or the regen_char field of get_telemetry) to a
// human-readable regen state description. Mirrors the mapping in
// onboard_regen.art and the RegenPhase enum in internal/mockpump.
func StateName(letter string) string {
	switch letter {
	case "A", "\\":
		return "Pump OFF"
	case "B", "C", "E", "^", "]":
		return "Warmup"
	case "D", "F", "G", "Q", "R":
		return "Purge gas failure"
	case "H":
		return "Extended purge"
	case "S":
		return "Repurge cycle"
	case "I", "J", "K", "T", "a", "b", "j", "n":
		return "Rough to base pressure"
	case "L":
		return "Rate of rise test"
	case "M", "N", "c", "d", "o":
		return "Cooldown"
	case "P":
		return "Regen complete"
	case "U":
		return "Beginning of fast regen"
	case "V":
		return "Regen aborted"
	case "W":
		return "Delay restart"
	case "X", "Y":
		return "Power failure"
	case "Z":
		return "Delay start"
	case "O", "[":
		return "Zeroing TC gauge"
	case "f":
		return "Share regen wait"
	case "e":
		return "Repurge during fast regen"
	case "h":
		return "Purge coordinate wait"
	case "i":
		return "Rough coordinate wait"
	case "k":
		return "Purge gas fail, recovering"
	default:
		return "Unknown (" + letter + ")"
	}
}
