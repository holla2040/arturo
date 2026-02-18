"""Validate that all YAML device profiles have required structure."""

import pytest
import yaml

from conftest import PROFILES_DIR


REQUIRED_FIELDS = ["manufacturer", "model", "protocol", "commands"]


class TestProfileValidity:
    """All YAML profiles must have required fields and valid structure."""

    @pytest.mark.schema
    def test_profiles_exist(self, all_profile_files):
        """At least one profile file must exist."""
        assert len(all_profile_files) > 0, "No profile YAML files found"

    @pytest.mark.schema
    def test_profiles_are_valid_yaml(self, all_profile_files):
        """Every profile file must parse as valid YAML."""
        for path in all_profile_files:
            with open(path) as f:
                data = yaml.safe_load(f)
            assert data is not None, f"{path.name}: empty YAML file"
            assert isinstance(data, dict), f"{path.name}: YAML root must be a mapping"

    @pytest.mark.schema
    def test_profiles_have_required_fields(self, all_profiles):
        """Every profile must have manufacturer, model, protocol, commands."""
        for path, data in all_profiles:
            for field in REQUIRED_FIELDS:
                assert field in data, (
                    f"{path.name}: missing required field '{field}'"
                )

    @pytest.mark.schema
    def test_manufacturer_is_string(self, all_profiles):
        """manufacturer must be a non-empty string."""
        for path, data in all_profiles:
            assert isinstance(data["manufacturer"], str), (
                f"{path.name}: manufacturer must be a string"
            )
            assert len(data["manufacturer"]) > 0, (
                f"{path.name}: manufacturer must not be empty"
            )

    @pytest.mark.schema
    def test_model_is_string(self, all_profiles):
        """model must be a non-empty string."""
        for path, data in all_profiles:
            assert isinstance(data["model"], str), (
                f"{path.name}: model must be a string"
            )
            assert len(data["model"]) > 0, (
                f"{path.name}: model must not be empty"
            )

    @pytest.mark.schema
    def test_protocol_is_known(self, all_profiles):
        """protocol must be one of the known protocol types."""
        known_protocols = {"scpi", "modbus", "modbus_rtu", "cti", "gpio", "serial", "ascii"}
        for path, data in all_profiles:
            assert data["protocol"] in known_protocols, (
                f"{path.name}: unknown protocol '{data['protocol']}'"
            )

    @pytest.mark.schema
    def test_commands_is_mapping(self, all_profiles):
        """commands must be a dict/mapping."""
        for path, data in all_profiles:
            assert isinstance(data["commands"], dict), (
                f"{path.name}: commands must be a mapping"
            )

    @pytest.mark.schema
    def test_commands_not_empty(self, all_profiles):
        """commands must contain at least one command."""
        for path, data in all_profiles:
            assert len(data["commands"]) > 0, (
                f"{path.name}: commands must not be empty"
            )
