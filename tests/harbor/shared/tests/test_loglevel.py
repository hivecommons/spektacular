"""Self-check for loglevel.py, which seeds and verifiers use to set the
project's log.level.

Run it locally with

    uvx --with pytest pytest -p no:cacheprovider tests/harbor/shared/tests/test_loglevel.py
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from loglevel import set_log_level  # noqa: E402

# The shape `spektacular init` writes: 4-space indents, a nested config block.
CONFIG = (
    "schema: 5\n"
    "name: todo-app\n"
    "agent: claude\n"
    "log:\n"
    "    provider: file\n"
    "    level: standard\n"
    "    retention: 10\n"
    "    config:\n"
    "        directory: logs\n"
    "spec:\n"
    "    level: not-the-log-level\n"
)


def test_changes_only_the_log_level_line(tmp_path):
    path = tmp_path / "config.yaml"
    path.write_text(CONFIG)
    set_log_level(path, "off")
    assert path.read_text() == CONFIG.replace("    level: standard\n", "    level: off\n")


def test_is_idempotent(tmp_path):
    path = tmp_path / "config.yaml"
    path.write_text(CONFIG)
    set_log_level(path, "verbose")
    once = path.read_text()
    set_log_level(path, "verbose")
    assert path.read_text() == once
    assert "    level: verbose\n" in once


def test_a_config_without_a_log_block_raises(tmp_path):
    path = tmp_path / "config.yaml"
    body = "schema: 5\nname: todo-app\nspec:\n    level: x\n"
    path.write_text(body)
    with pytest.raises(ValueError):
        set_log_level(path, "off")
    assert path.read_text() == body


def test_a_log_block_without_a_level_raises(tmp_path):
    path = tmp_path / "config.yaml"
    path.write_text("log:\n    provider: file\nspec:\n    level: x\n")
    with pytest.raises(ValueError):
        set_log_level(path, "off")


def test_an_unknown_level_raises(tmp_path):
    path = tmp_path / "config.yaml"
    path.write_text(CONFIG)
    with pytest.raises(ValueError):
        set_log_level(path, "loud")
    assert path.read_text() == CONFIG
