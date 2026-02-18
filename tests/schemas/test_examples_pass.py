"""Validate that all example JSON files pass their schema validation."""

import json

import jsonschema
import pytest

from conftest import SCHEMA_DIR, TYPE_SCHEMAS, _discover_examples


def _example_ids():
    """Generate test IDs like 'device-command-request/measure_voltage'."""
    return [
        f"{dir_name}/{path.stem}" for dir_name, path in _discover_examples()
    ]


def _example_params():
    """Load all examples with their schemas for parametrize."""
    params = []
    for dir_name, example_path in _discover_examples():
        with open(example_path) as f:
            example_data = json.load(f)
        schema_file = SCHEMA_DIR / f"{dir_name}.schema.json"
        with open(schema_file) as f:
            type_schema = json.load(f)
        params.append((dir_name, example_path, example_data, type_schema))
    return params


class TestExamplesPass:
    """Every example must validate against its type-specific schema and the envelope."""

    @pytest.mark.schema
    @pytest.mark.parametrize(
        "dir_name, example_path, example_data, type_schema",
        _example_params(),
        ids=_example_ids(),
    )
    def test_example_validates_against_type_schema(
        self, dir_name, example_path, example_data, type_schema
    ):
        """Each example must validate against its message-type schema."""
        jsonschema.validate(instance=example_data, schema=type_schema)

    @pytest.mark.schema
    @pytest.mark.parametrize(
        "dir_name, example_path, example_data, type_schema",
        _example_params(),
        ids=_example_ids(),
    )
    def test_example_validates_against_envelope_schema(
        self, dir_name, example_path, example_data, type_schema, envelope_schema
    ):
        """Each example must also validate against the generic envelope schema."""
        jsonschema.validate(instance=example_data, schema=envelope_schema)

    @pytest.mark.schema
    def test_at_least_one_example_per_message_type(self):
        """Every message type must have at least one example."""
        for dir_name in TYPE_SCHEMAS:
            examples_dir = SCHEMA_DIR / dir_name / "examples"
            assert examples_dir.exists(), (
                f"No examples directory for {dir_name}"
            )
            examples = list(examples_dir.glob("*.json"))
            assert len(examples) > 0, (
                f"No example files in {examples_dir}"
            )

    @pytest.mark.schema
    def test_examples_have_correct_envelope_type(self):
        """Each example's envelope.type must match its schema directory."""
        for dir_name, example_path in _discover_examples():
            with open(example_path) as f:
                data = json.load(f)
            expected_type = TYPE_SCHEMAS[dir_name]
            actual_type = data["envelope"]["type"]
            assert actual_type == expected_type, (
                f"{example_path.name}: envelope.type is '{actual_type}', "
                f"expected '{expected_type}'"
            )
