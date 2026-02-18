"""Verify that invalid messages are correctly rejected by schema validation."""

import copy
import json

import jsonschema
import pytest

from conftest import SCHEMA_DIR


def _load_schema(name):
    with open(SCHEMA_DIR / f"{name}.schema.json") as f:
        return json.load(f)


def _valid_heartbeat():
    """A minimal valid heartbeat message for mutation."""
    return {
        "envelope": {
            "id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
            "timestamp": 1771329600,
            "source": {
                "service": "esp32_tcp_bridge",
                "instance": "station-01",
                "version": "1.0.0",
            },
            "schema_version": "v1.0.0",
            "type": "service.heartbeat",
        },
        "payload": {
            "status": "running",
            "uptime_seconds": 3600,
            "devices": ["fluke-8846a"],
            "free_heap": 245000,
            "wifi_rssi": -42,
            "firmware_version": "1.0.0",
        },
    }


def _valid_command_response():
    """A minimal valid command response with success=false for mutation."""
    return {
        "envelope": {
            "id": "b1ffc99a-8d1c-4ef9-8c7e-7cc0ce491b22",
            "timestamp": 1771329665,
            "source": {
                "service": "esp32_tcp_bridge",
                "instance": "dmm-station-01",
                "version": "1.0.0",
            },
            "schema_version": "v1.0.0",
            "type": "device.command.response",
            "correlation_id": "d4e5f6a7-b8c9-4d0e-a1a2-b3c4d5e6f7a8",
        },
        "payload": {
            "device_id": "fluke-8846a",
            "command_name": "measure_dc_voltage",
            "success": False,
            "response": None,
            "error": {
                "code": "E_DEVICE_TIMEOUT",
                "message": "Device did not respond within 5000ms",
            },
            "duration_ms": 5001,
        },
    }


def _valid_command_request():
    """A minimal valid command request for mutation."""
    return {
        "envelope": {
            "id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
            "timestamp": 1771329600,
            "source": {
                "service": "controller",
                "instance": "ctrl-01",
                "version": "1.0.0",
            },
            "schema_version": "v1.0.0",
            "type": "device.command.request",
            "correlation_id": "d4e5f6a7-b8c9-4d0e-a1a2-b3c4d5e6f7a8",
            "reply_to": "responses:controller:ctrl-01",
        },
        "payload": {
            "device_id": "fluke-8846a",
            "command_name": "measure_dc_voltage",
        },
    }


class TestMissingRequiredFields:
    """Messages with missing required fields must be rejected."""

    @pytest.mark.schema
    def test_missing_envelope_id(self):
        """Missing envelope.id must fail validation."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        del msg["envelope"]["id"]
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_missing_envelope_timestamp(self):
        """Missing envelope.timestamp must fail validation."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        del msg["envelope"]["timestamp"]
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_missing_envelope_source(self):
        """Missing envelope.source must fail validation."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        del msg["envelope"]["source"]
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_missing_envelope_type(self):
        """Missing envelope.type must fail validation."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        del msg["envelope"]["type"]
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_missing_payload(self):
        """Missing payload must fail validation."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        del msg["payload"]
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_missing_correlation_id_on_request(self):
        """Command request without correlation_id must fail."""
        schema = _load_schema("device-command-request")
        msg = _valid_command_request()
        del msg["envelope"]["correlation_id"]
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_missing_reply_to_on_request(self):
        """Command request without reply_to must fail."""
        schema = _load_schema("device-command-request")
        msg = _valid_command_request()
        del msg["envelope"]["reply_to"]
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)


class TestWrongTypes:
    """Fields with wrong types must be rejected."""

    @pytest.mark.schema
    def test_timestamp_as_string(self):
        """timestamp as string must fail validation."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["envelope"]["timestamp"] = "2024-01-01T00:00:00Z"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_timestamp_negative(self):
        """Negative timestamp must fail validation."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["envelope"]["timestamp"] = -1
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_payload_as_string(self):
        """payload as string instead of object must fail."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["payload"] = "not an object"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_success_as_string(self):
        """Command response success as string must fail."""
        schema = _load_schema("device-command-response")
        msg = _valid_command_response()
        msg["payload"]["success"] = "true"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)


class TestInvalidEnums:
    """Invalid enum values must be rejected."""

    @pytest.mark.schema
    def test_unknown_message_type(self):
        """envelope.type with unknown value must fail validation."""
        schema = _load_schema("envelope")
        msg = _valid_heartbeat()
        msg["envelope"]["type"] = "unknown.type"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_invalid_heartbeat_status(self):
        """Heartbeat with invalid status enum must fail."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["payload"]["status"] = "broken"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_invalid_estop_reason(self):
        """E-stop with invalid reason enum must fail."""
        schema = _load_schema("system-emergency-stop")
        msg = {
            "envelope": {
                "id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
                "timestamp": 1771329600,
                "source": {
                    "service": "esp32_estop",
                    "instance": "estop-01",
                    "version": "1.0.0",
                },
                "schema_version": "v1.0.0",
                "type": "system.emergency_stop",
            },
            "payload": {
                "reason": "invalid_reason",
            },
        }
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_invalid_error_code(self):
        """Command response with invalid error code must fail."""
        schema = _load_schema("device-command-response")
        msg = _valid_command_response()
        msg["payload"]["error"]["code"] = "E_UNKNOWN_CODE"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)


class TestPatternValidation:
    """Fields with pattern constraints must reject invalid values."""

    @pytest.mark.schema
    def test_invalid_uuid_format(self):
        """envelope.id with non-UUIDv4 format must fail."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["envelope"]["id"] = "not-a-uuid"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_uuid_v1_rejected(self):
        """UUIDv1 (not v4) must fail the pattern check."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        # UUIDv1 has version nibble '1' not '4'
        msg["envelope"]["id"] = "a1b2c3d4-e5f6-1a7b-8c9d-0e1f2a3b4c5d"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_correlation_id_not_uuidv4(self):
        """correlation_id that isn't UUIDv4 must fail."""
        schema = _load_schema("device-command-response")
        msg = _valid_command_response()
        msg["envelope"]["correlation_id"] = "not-a-valid-uuid-at-all"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_invalid_schema_version(self):
        """schema_version != 'v1.0.0' must fail."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["envelope"]["schema_version"] = "v2.0.0"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_invalid_semver_format(self):
        """source.version with invalid semver must fail."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["envelope"]["source"]["version"] = "v1.0"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_invalid_service_name_uppercase(self):
        """source.service with uppercase must fail pattern check."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["envelope"]["source"]["service"] = "MyService"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)


class TestRangeValidation:
    """Numeric range constraints must be enforced."""

    @pytest.mark.schema
    def test_heartbeat_wifi_rssi_positive(self):
        """wifi_rssi = 5 (positive, out of range) must fail."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["payload"]["wifi_rssi"] = 5
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_heartbeat_wifi_rssi_too_low(self):
        """wifi_rssi below -127 must fail."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["payload"]["wifi_rssi"] = -200
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_command_timeout_too_low(self):
        """timeout_ms below 100 must fail."""
        schema = _load_schema("device-command-request")
        msg = _valid_command_request()
        msg["payload"]["timeout_ms"] = 50
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_command_timeout_too_high(self):
        """timeout_ms above 300000 must fail."""
        schema = _load_schema("device-command-request")
        msg = _valid_command_request()
        msg["payload"]["timeout_ms"] = 500000
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)


class TestConditionalValidation:
    """Conditional schema rules must be enforced."""

    @pytest.mark.schema
    def test_command_response_failure_without_error(self):
        """success: false without error object must fail."""
        schema = _load_schema("device-command-response")
        msg = _valid_command_response()
        del msg["payload"]["error"]
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_command_response_success_without_error_ok(self):
        """success: true without error is valid."""
        schema = _load_schema("device-command-response")
        msg = _valid_command_response()
        msg["payload"]["success"] = True
        del msg["payload"]["error"]
        msg["payload"]["response"] = "1.23456789"
        # Should NOT raise
        jsonschema.validate(instance=msg, schema=schema)


class TestAdditionalProperties:
    """additionalProperties: false must reject unknown fields."""

    @pytest.mark.schema
    def test_extra_top_level_field(self):
        """Extra top-level field must be rejected."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["extra_field"] = "should not be here"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_extra_envelope_field(self):
        """Extra field inside envelope must be rejected."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["envelope"]["extra_field"] = "should not be here"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_extra_payload_field(self):
        """Extra field inside payload must be rejected."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["payload"]["extra_field"] = "should not be here"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_extra_source_field(self):
        """Extra field inside source must be rejected."""
        schema = _load_schema("service-heartbeat")
        msg = _valid_heartbeat()
        msg["envelope"]["source"]["extra_field"] = "nope"
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)


class TestOtaValidation:
    """OTA-specific validation rules."""

    @pytest.mark.schema
    def test_invalid_sha256_length(self):
        """sha256 that isn't exactly 64 hex chars must fail."""
        schema = _load_schema("system-ota-request")
        msg = {
            "envelope": {
                "id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
                "timestamp": 1771329600,
                "source": {
                    "service": "controller",
                    "instance": "ctrl-01",
                    "version": "1.0.0",
                },
                "schema_version": "v1.0.0",
                "type": "system.ota.request",
                "correlation_id": "d4e5f6a7-b8c9-4d0e-a1a2-b3c4d5e6f7a8",
                "reply_to": "responses:controller:ctrl-01",
            },
            "payload": {
                "firmware_url": "http://192.168.1.10:8080/firmware/test.bin",
                "version": "1.1.0",
                "sha256": "tooshort",
            },
        }
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)

    @pytest.mark.schema
    def test_invalid_firmware_url_scheme(self):
        """firmware_url without http/https must fail."""
        schema = _load_schema("system-ota-request")
        msg = {
            "envelope": {
                "id": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
                "timestamp": 1771329600,
                "source": {
                    "service": "controller",
                    "instance": "ctrl-01",
                    "version": "1.0.0",
                },
                "schema_version": "v1.0.0",
                "type": "system.ota.request",
                "correlation_id": "d4e5f6a7-b8c9-4d0e-a1a2-b3c4d5e6f7a8",
                "reply_to": "responses:controller:ctrl-01",
            },
            "payload": {
                "firmware_url": "ftp://192.168.1.10/firmware/test.bin",
                "version": "1.1.0",
                "sha256": "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
            },
        }
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.validate(instance=msg, schema=schema)
