"""Redis Streams integration tests for Arturo command/response flow."""

import json
import uuid

import pytest
import redis


TEST_STREAM_PREFIX = "test:commands:"


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
def stream_name():
    """Unique stream name for test isolation."""
    return f"{TEST_STREAM_PREFIX}{uuid.uuid4().hex[:8]}"


@pytest.fixture(autouse=True)
def cleanup(r, stream_name):
    """Delete test stream after each test."""
    yield
    r.delete(stream_name)


class TestStreams:
    """Redis Streams operations matching Arturo command/response pattern."""

    @pytest.mark.integration
    def test_xadd_and_xread(self, r, stream_name):
        """XADD to a station stream, XREAD retrieves the message."""
        msg = {
            "envelope": json.dumps({
                "id": str(uuid.uuid4()),
                "type": "device.command.request",
            }),
            "payload": json.dumps({
                "device_id": "fluke-8846a",
                "command_name": "measure_dc_voltage",
            }),
        }

        msg_id = r.xadd(stream_name, msg)
        assert msg_id is not None

        results = r.xread({stream_name: "0-0"}, count=1)
        assert len(results) == 1

        returned_stream, messages = results[0]
        assert returned_stream == stream_name
        assert len(messages) == 1

        returned_id, returned_fields = messages[0]
        assert returned_id == msg_id
        assert returned_fields["envelope"] == msg["envelope"]
        assert returned_fields["payload"] == msg["payload"]

    @pytest.mark.integration
    def test_xadd_multiple_messages(self, r, stream_name):
        """Multiple XADD messages are read in order."""
        ids = []
        for i in range(3):
            msg_id = r.xadd(stream_name, {"seq": str(i)})
            ids.append(msg_id)

        results = r.xread({stream_name: "0-0"}, count=10)
        messages = results[0][1]
        assert len(messages) == 3

        for i, (mid, fields) in enumerate(messages):
            assert mid == ids[i]
            assert fields["seq"] == str(i)

    @pytest.mark.integration
    def test_consumer_group_and_xack(self, r, stream_name):
        """Consumer group: XREADGROUP reads, XACK removes from pending."""
        group = "test-group"
        consumer = "test-consumer"

        # Add a message first so the stream exists
        msg_id = r.xadd(stream_name, {"data": "test-payload"})

        # Create consumer group starting from beginning
        r.xgroup_create(stream_name, group, id="0")

        # Read with consumer group
        results = r.xreadgroup(group, consumer, {stream_name: ">"}, count=1)
        assert len(results) == 1
        messages = results[0][1]
        assert len(messages) == 1

        returned_id = messages[0][0]
        assert returned_id == msg_id

        # Message should be in pending list
        pending = r.xpending(stream_name, group)
        assert pending["pending"] == 1

        # ACK the message
        ack_count = r.xack(stream_name, group, returned_id)
        assert ack_count == 1

        # Pending should now be empty
        pending = r.xpending(stream_name, group)
        assert pending["pending"] == 0

    @pytest.mark.integration
    def test_xread_block_returns_none_on_empty(self, r, stream_name):
        """XREAD with short block timeout returns None on empty stream."""
        # Create the stream with a dummy message then delete it
        r.xadd(stream_name, {"init": "1"})
        # Read from latest - no new messages
        results = r.xread({stream_name: "$"}, count=1, block=100)
        assert results is None or len(results) == 0

    @pytest.mark.integration
    def test_stream_length(self, r, stream_name):
        """XLEN reports correct stream length."""
        assert r.xlen(stream_name) == 0 or not r.exists(stream_name)

        for i in range(5):
            r.xadd(stream_name, {"i": str(i)})

        assert r.xlen(stream_name) == 5
