package protocol

import (
	"fmt"
	"regexp"
)

// Compiled regex patterns matching the JSON schema definitions.
var (
	uuidV4Pattern  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	servicePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	instancePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	replyToPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_:/-]*$`)
)

// validTypes is a set for fast type lookup.
var validTypes = func() map[string]bool {
	m := make(map[string]bool, len(ValidMessageTypes))
	for _, t := range ValidMessageTypes {
		m[t] = true
	}
	return m
}()

// requestTypes require both correlation_id and reply_to.
var requestTypes = map[string]bool{
	TypeDeviceCommandRequest: true,
	TypeSystemOTARequest:     true,
}

// responseTypes require correlation_id.
var responseTypes = map[string]bool{
	TypeDeviceCommandResponse: true,
}

// Validate checks a Message against protocol rules.
func Validate(msg *Message) error {
	env := msg.Envelope

	// Validate ID is UUIDv4.
	if !uuidV4Pattern.MatchString(env.ID) {
		return fmt.Errorf("invalid id: must be UUIDv4 format, got %q", env.ID)
	}

	// Validate timestamp.
	if env.Timestamp < 0 {
		return fmt.Errorf("invalid timestamp: must be >= 0, got %d", env.Timestamp)
	}

	// Validate source fields.
	if err := validateSource(env.Source); err != nil {
		return err
	}

	// Validate schema_version.
	if env.SchemaVersion != SchemaVersion {
		return fmt.Errorf("invalid schema_version: must be %q, got %q", SchemaVersion, env.SchemaVersion)
	}

	// Validate type.
	if !validTypes[env.Type] {
		return fmt.Errorf("invalid type: %q is not a valid message type", env.Type)
	}

	// Validate correlation_id format if present.
	if env.CorrelationID != "" {
		if !uuidV4Pattern.MatchString(env.CorrelationID) {
			return fmt.Errorf("invalid correlation_id: must be UUIDv4 format, got %q", env.CorrelationID)
		}
	}

	// Validate reply_to format if present.
	if env.ReplyTo != "" {
		if !replyToPattern.MatchString(env.ReplyTo) {
			return fmt.Errorf("invalid reply_to: must match pattern %q, got %q", replyToPattern.String(), env.ReplyTo)
		}
	}

	// Request types require correlation_id and reply_to.
	if requestTypes[env.Type] {
		if env.CorrelationID == "" {
			return fmt.Errorf("missing correlation_id: required for type %q", env.Type)
		}
		if env.ReplyTo == "" {
			return fmt.Errorf("missing reply_to: required for type %q", env.Type)
		}
	}

	// Response types require correlation_id.
	if responseTypes[env.Type] {
		if env.CorrelationID == "" {
			return fmt.Errorf("missing correlation_id: required for type %q", env.Type)
		}
	}

	return nil
}

func validateSource(src Source) error {
	if src.Service == "" || len(src.Service) > 64 || !servicePattern.MatchString(src.Service) {
		return fmt.Errorf("invalid source.service: must match pattern %q (1-64 chars), got %q", servicePattern.String(), src.Service)
	}
	if src.Instance == "" || len(src.Instance) > 64 || !instancePattern.MatchString(src.Instance) {
		return fmt.Errorf("invalid source.instance: must match pattern %q (1-64 chars), got %q", instancePattern.String(), src.Instance)
	}
	if !versionPattern.MatchString(src.Version) {
		return fmt.Errorf("invalid source.version: must be semver format, got %q", src.Version)
	}
	return nil
}
