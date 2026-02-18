"""Redis ACL integration tests for Arturo station isolation."""

import uuid

import pytest
import redis


@pytest.fixture
def r():
    """Redis connection (admin/default), skip if unavailable."""
    try:
        conn = redis.Redis(host="localhost", port=6379, decode_responses=True)
        conn.ping()
        return conn
    except Exception:
        pytest.skip("Redis not available at localhost:6379")


def _acl_supported(r):
    """Check if ACL commands are available and we can manage users."""
    try:
        r.execute_command("ACL", "WHOAMI")
        return True
    except redis.ResponseError:
        return False


def _create_test_user(r, username, password, keys_pattern, commands):
    """Create a temporary ACL user for testing."""
    # Reset user first in case it exists
    try:
        r.execute_command("ACL", "DELUSER", username)
    except redis.ResponseError:
        pass
    rule = f"on >{password} ~{keys_pattern} {commands}"
    r.execute_command("ACL", "SETUSER", username, *rule.split())


def _delete_test_user(r, username):
    """Remove a test ACL user."""
    try:
        r.execute_command("ACL", "DELUSER", username)
    except redis.ResponseError:
        pass


@pytest.fixture
def acl_check(r):
    """Skip if ACL is not supported or we can't manage users."""
    if not _acl_supported(r):
        pytest.skip("Redis ACL not supported or not accessible")


class TestACL:
    """Station-scoped ACL: each station can only read its own command stream."""

    @pytest.mark.integration
    def test_station_can_read_own_commands(self, r, acl_check):
        """Station user can read its own command stream."""
        username = f"test-station-{uuid.uuid4().hex[:6]}"
        password = "testpass123"
        stream = f"commands:{username}"

        try:
            _create_test_user(
                r, username, password,
                f"commands:{username}",
                "+xread +xadd +ping +auth",
            )

            # Add a message to the station's stream (as admin)
            r.xadd(stream, {"data": "test-command"})

            # Connect as the station user
            station_conn = redis.Redis(
                host="localhost", port=6379,
                username=username, password=password,
                decode_responses=True,
            )
            station_conn.ping()

            # Station can read its own stream
            results = station_conn.xread({stream: "0-0"}, count=1)
            assert len(results) == 1
            assert results[0][0] == stream

            station_conn.close()
        finally:
            r.delete(stream)
            _delete_test_user(r, username)

    @pytest.mark.integration
    def test_station_cannot_read_other_commands(self, r, acl_check):
        """Station user CANNOT read another station's command stream."""
        username = f"test-station-{uuid.uuid4().hex[:6]}"
        other_stream = f"commands:other-station-{uuid.uuid4().hex[:6]}"
        password = "testpass123"

        try:
            _create_test_user(
                r, username, password,
                f"commands:{username}",
                "+xread +xadd +ping +auth",
            )

            # Add a message to the OTHER station's stream (as admin)
            r.xadd(other_stream, {"data": "secret-command"})

            # Connect as the station user
            station_conn = redis.Redis(
                host="localhost", port=6379,
                username=username, password=password,
                decode_responses=True,
            )

            # Station should NOT be able to read another station's stream
            with pytest.raises(redis.ResponseError):
                station_conn.xread({other_stream: "0-0"}, count=1)

            station_conn.close()
        finally:
            r.delete(other_stream)
            _delete_test_user(r, username)

    @pytest.mark.integration
    def test_station_can_write_responses(self, r, acl_check):
        """Station user can write to response streams."""
        username = f"test-station-{uuid.uuid4().hex[:6]}"
        password = "testpass123"
        response_stream = f"responses:controller:ctrl-01"

        try:
            _create_test_user(
                r, username, password,
                f"commands:{username} ~responses:*",
                "+xread +xadd +ping +auth",
            )

            station_conn = redis.Redis(
                host="localhost", port=6379,
                username=username, password=password,
                decode_responses=True,
            )

            # Station can write to response stream
            msg_id = station_conn.xadd(response_stream, {"data": "test-response"})
            assert msg_id is not None

            station_conn.close()
        finally:
            r.delete(response_stream)
            _delete_test_user(r, username)

    @pytest.mark.integration
    def test_station_cannot_use_admin_commands(self, r, acl_check):
        """Station user cannot run admin commands like CONFIG."""
        username = f"test-station-{uuid.uuid4().hex[:6]}"
        password = "testpass123"

        try:
            _create_test_user(
                r, username, password,
                f"commands:{username}",
                "+xread +xadd +ping +auth",
            )

            station_conn = redis.Redis(
                host="localhost", port=6379,
                username=username, password=password,
                decode_responses=True,
            )

            with pytest.raises(redis.ResponseError):
                station_conn.execute_command("CONFIG", "GET", "maxmemory")

            station_conn.close()
        finally:
            _delete_test_user(r, username)
