"""Redis presence key integration tests for Arturo station alive tracking."""

import time
import uuid

import pytest
import redis


TEST_KEY_PREFIX = "test:device:"


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
def presence_key():
    """Unique presence key for test isolation."""
    return f"{TEST_KEY_PREFIX}{uuid.uuid4().hex[:8]}:alive"


@pytest.fixture(autouse=True)
def cleanup(r, presence_key):
    """Delete test key after each test."""
    yield
    r.delete(presence_key)


class TestPresence:
    """Station presence via Redis keys with TTL (device:{instance}:alive)."""

    @pytest.mark.integration
    def test_set_presence_with_ttl(self, r, presence_key):
        """SET presence key with TTL, verify key exists and TTL is set."""
        ttl_seconds = 90
        r.set(presence_key, "running", ex=ttl_seconds)

        assert r.exists(presence_key)
        assert r.get(presence_key) == "running"

        actual_ttl = r.ttl(presence_key)
        assert 0 < actual_ttl <= ttl_seconds

    @pytest.mark.integration
    def test_ttl_decrements(self, r, presence_key):
        """TTL decrements over time."""
        r.set(presence_key, "running", ex=10)

        ttl_before = r.ttl(presence_key)
        time.sleep(1.5)
        ttl_after = r.ttl(presence_key)

        assert ttl_after < ttl_before

    @pytest.mark.integration
    def test_key_expires(self, r, presence_key):
        """Key with short TTL expires and is no longer accessible."""
        r.set(presence_key, "running", ex=1)
        assert r.exists(presence_key)

        time.sleep(1.5)
        assert not r.exists(presence_key)
        assert r.get(presence_key) is None

    @pytest.mark.integration
    def test_ttl_refresh(self, r, presence_key):
        """Heartbeat refreshes TTL by re-setting the key."""
        r.set(presence_key, "running", ex=5)
        time.sleep(2)

        # Refresh TTL (station heartbeat re-sets the key)
        r.set(presence_key, "running", ex=90)

        actual_ttl = r.ttl(presence_key)
        assert actual_ttl > 5  # TTL was refreshed well beyond original

    @pytest.mark.integration
    def test_presence_key_without_ttl_persists(self, r, presence_key):
        """Key without TTL has ttl -1 (persists forever)."""
        r.set(presence_key, "running")
        assert r.ttl(presence_key) == -1

    @pytest.mark.integration
    def test_presence_value_stores_status(self, r, presence_key):
        """Presence key value can store station status."""
        r.set(presence_key, "degraded", ex=90)
        assert r.get(presence_key) == "degraded"

        r.set(presence_key, "running", ex=90)
        assert r.get(presence_key) == "running"
