"""Shared fixtures for Arturo test suite."""

import json
from pathlib import Path

import pytest
import yaml

PROJECT_ROOT = Path(__file__).parent.parent
SCHEMA_DIR = PROJECT_ROOT / "schemas" / "v1.0.0"
PROFILES_DIR = PROJECT_ROOT / "profiles"

# Map schema directory names to their schema files
# e.g. "device-command-request" -> "device-command-request.schema.json"
TYPE_SCHEMAS = {
    "device-command-request": "device.command.request",
    "device-command-response": "device.command.response",
    "service-heartbeat": "service.heartbeat",
    "system-emergency-stop": "system.emergency_stop",
    "system-ota-request": "system.ota.request",
}


@pytest.fixture(scope="session")
def schema_dir():
    """Path to the v1.0.0 schema directory."""
    return SCHEMA_DIR


@pytest.fixture(scope="session")
def all_schema_files():
    """List of all .schema.json file paths."""
    return sorted(SCHEMA_DIR.glob("*.schema.json"))


@pytest.fixture(scope="session")
def all_schemas():
    """Dict of filename -> parsed schema for all .schema.json files."""
    schemas = {}
    for path in sorted(SCHEMA_DIR.glob("*.schema.json")):
        with open(path) as f:
            schemas[path.name] = json.load(f)
    return schemas


@pytest.fixture(scope="session")
def envelope_schema():
    """The generic envelope schema."""
    with open(SCHEMA_DIR / "envelope.schema.json") as f:
        return json.load(f)


@pytest.fixture(scope="session")
def type_schemas():
    """Dict of message type string -> parsed type-specific schema."""
    schemas = {}
    for dir_name, type_name in TYPE_SCHEMAS.items():
        schema_file = SCHEMA_DIR / f"{dir_name}.schema.json"
        if schema_file.exists():
            with open(schema_file) as f:
                schemas[type_name] = json.load(f)
    return schemas


def _discover_examples():
    """Find all example JSON files and their associated schema dir name."""
    examples = []
    for dir_name in TYPE_SCHEMAS:
        examples_dir = SCHEMA_DIR / dir_name / "examples"
        if examples_dir.exists():
            for example_file in sorted(examples_dir.glob("*.json")):
                examples.append((dir_name, example_file))
    return examples


@pytest.fixture(scope="session")
def all_examples():
    """List of (dir_name, example_path, example_data) tuples."""
    results = []
    for dir_name, path in _discover_examples():
        with open(path) as f:
            data = json.load(f)
        results.append((dir_name, path, data))
    return results


@pytest.fixture(scope="session")
def all_profile_files():
    """List of all YAML profile file paths."""
    return sorted(PROFILES_DIR.rglob("*.yaml"))


@pytest.fixture(scope="session")
def all_profiles():
    """List of (path, parsed_data) tuples for all YAML profiles."""
    profiles = []
    for path in sorted(PROFILES_DIR.rglob("*.yaml")):
        with open(path) as f:
            data = yaml.safe_load(f)
        profiles.append((path, data))
    return profiles


@pytest.fixture(scope="session")
def redis_connection():
    """Redis connection, skip test if unavailable."""
    try:
        import redis
        conn = redis.Redis(host="localhost", port=6379, decode_responses=True)
        conn.ping()
        return conn
    except Exception:
        pytest.skip("Redis not available at localhost:6379")
