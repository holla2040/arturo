"""Validate that all .schema.json files are valid JSON Schema draft-07."""

import json

import jsonschema
import pytest


class TestSchemaValidity:
    """All schema files must be valid JSON Schema draft-07."""

    @pytest.mark.schema
    def test_all_schemas_are_valid_draft07(self, all_schema_files):
        """Every .schema.json file must be a valid JSON Schema draft-07."""
        assert len(all_schema_files) > 0, "No schema files found"

        for schema_path in all_schema_files:
            with open(schema_path) as f:
                schema = json.load(f)

            # Validate the schema itself against the JSON Schema meta-schema
            jsonschema.Draft7Validator.check_schema(schema)

    @pytest.mark.schema
    def test_schemas_have_required_meta_fields(self, all_schemas):
        """Each schema must have $schema, title, and type fields."""
        for filename, schema in all_schemas.items():
            assert "$schema" in schema, f"{filename}: missing $schema"
            assert schema["$schema"] == "http://json-schema.org/draft-07/schema#", (
                f"{filename}: $schema is not draft-07"
            )
            assert "title" in schema, f"{filename}: missing title"
            assert "type" in schema, f"{filename}: missing type"

    @pytest.mark.schema
    def test_envelope_schema_exists(self, all_schemas):
        """The envelope schema must exist."""
        assert "envelope.schema.json" in all_schemas

    @pytest.mark.schema
    def test_error_schema_exists(self, all_schemas):
        """The error schema must exist."""
        assert "error.schema.json" in all_schemas

    @pytest.mark.schema
    def test_all_message_type_schemas_exist(self, all_schemas):
        """A schema must exist for each of the 5 message types."""
        expected = [
            "device-command-request.schema.json",
            "device-command-response.schema.json",
            "service-heartbeat.schema.json",
            "system-emergency-stop.schema.json",
            "system-ota-request.schema.json",
        ]
        for name in expected:
            assert name in all_schemas, f"Missing schema: {name}"

    @pytest.mark.schema
    def test_message_schemas_use_additional_properties_false(self, all_schemas):
        """All message schemas should disallow extra top-level properties."""
        # Skip error.schema.json as it's a component schema, not a message schema
        message_schemas = {
            k: v for k, v in all_schemas.items() if k != "error.schema.json"
        }
        for filename, schema in message_schemas.items():
            assert schema.get("additionalProperties") is False, (
                f"{filename}: missing additionalProperties: false at top level"
            )
