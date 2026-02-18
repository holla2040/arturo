"""Redis Pub/Sub integration tests for Arturo heartbeat and E-stop channels."""

import json
import threading
import time
import uuid

import jsonschema
import pytest
import redis

from conftest import SCHEMA_DIR


@pytest.fixture
def r():
    """Redis connection, skip if unavailable."""
    try:
        conn = redis.Redis(host="localhost", port=6379, decode_responses=True)
        conn.ping()
        return conn
    except Exception:
        pytest.skip("Redis not available at localhost:6379")


@pytest.fixture
def heartbeat_schema():
    """Load the service-heartbeat schema."""
    import json as _json
    with open(SCHEMA_DIR / "service-heartbeat.schema.json") as f:
        return _json.load(f)


@pytest.fixture
def estop_schema():
    """Load the system-emergency-stop schema."""
    import json as _json
    with open(SCHEMA_DIR / "system-emergency-stop.schema.json") as f:
        return _json.load(f)


def _valid_heartbeat_msg():
    """A valid heartbeat message for publishing."""
    return {
        "envelope": {
            "id": str(uuid.uuid4()),
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


def _valid_estop_msg():
    """A valid E-stop message for publishing."""
    return {
        "envelope": {
            "id": str(uuid.uuid4()),
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
            "reason": "operator_command",
            "description": "Integration test E-stop",
            "initiator": "test-runner",
        },
    }


class TestPubSub:
    """Pub/Sub for heartbeat and E-stop channels."""

    @pytest.mark.integration
    def test_publish_heartbeat_received_by_subscriber(self, r):
        """PUBLISH heartbeat, subscriber receives it."""
        channel = "events:heartbeat"
        received = []

        pubsub = r.pubsub()
        pubsub.subscribe(channel)
        # Consume the subscription confirmation message
        pubsub.get_message(timeout=1)

        msg = _valid_heartbeat_msg()
        payload = json.dumps(msg)

        # Publish
        num_subscribers = r.publish(channel, payload)
        assert num_subscribers >= 1

        # Receive
        message = pubsub.get_message(timeout=2)
        assert message is not None
        assert message["type"] == "message"
        assert message["channel"] == channel
        assert message["data"] == payload

        pubsub.unsubscribe()
        pubsub.close()

    @pytest.mark.integration
    def test_heartbeat_message_matches_schema(self, r, heartbeat_schema):
        """Published heartbeat validates against schema when received."""
        channel = "events:heartbeat"

        pubsub = r.pubsub()
        pubsub.subscribe(channel)
        pubsub.get_message(timeout=1)

        msg = _valid_heartbeat_msg()
        r.publish(channel, json.dumps(msg))

        message = pubsub.get_message(timeout=2)
        assert message is not None

        received_data = json.loads(message["data"])
        jsonschema.validate(instance=received_data, schema=heartbeat_schema)

        pubsub.unsubscribe()
        pubsub.close()

    @pytest.mark.integration
    def test_estop_broadcast_received(self, r, estop_schema):
        """E-stop published on events:emergency_stop is received and valid."""
        channel = "events:emergency_stop"

        pubsub = r.pubsub()
        pubsub.subscribe(channel)
        pubsub.get_message(timeout=1)

        msg = _valid_estop_msg()
        r.publish(channel, json.dumps(msg))

        message = pubsub.get_message(timeout=2)
        assert message is not None
        assert message["type"] == "message"

        received_data = json.loads(message["data"])
        jsonschema.validate(instance=received_data, schema=estop_schema)

        pubsub.unsubscribe()
        pubsub.close()

    @pytest.mark.integration
    def test_multiple_subscribers_receive(self, r):
        """Multiple subscribers all receive the same published message."""
        channel = "events:heartbeat"

        subs = []
        for _ in range(3):
            ps = r.pubsub()
            ps.subscribe(channel)
            ps.get_message(timeout=1)
            subs.append(ps)

        msg = json.dumps(_valid_heartbeat_msg())
        num = r.publish(channel, msg)
        assert num >= 3

        for ps in subs:
            message = ps.get_message(timeout=2)
            assert message is not None
            assert message["data"] == msg
            ps.unsubscribe()
            ps.close()
