"""Static self-check that the README documents the run metrics record and
the report as metrics.py and report.py produce them.

Everything here only reads README.md as text, against hand-written
expectations: the record's keys below are written out by hand from
metrics.py's `build_record`, not derived by importing it, so a key added to
the record without a line in the README fails here.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_readme_metrics.py
"""

import re
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[4]
README = ROOT / "README.md"

# The record's shape, by hand from metrics.py's build_record.
TOP_LEVEL_KEYS = ["schema_version", "run", "result", "totals", "steps", "subagents", "unknown"]
RUN_KEYS = ["suite", "scenario", "agent", "model", "spektacular_version", "commit", "dirty"]
TOTALS_KEYS = [
    "wall_time_s",
    "model_time_s",
    "turns",
    "cost_usd",
    "input_tokens",
    "output_tokens",
    "cache_read_tokens",
    "cache_write_tokens",
]
SUBAGENTS_KEYS = ["started", "completed", "failed", "stopped", "durations_s"]
CHECKS_KEYS = ["passed", "total", "score"]


def readme_text():
    return README.read_text()


def section(heading):
    """The body of a `#### heading` section, up to the next heading of any
    level from `##` to `####`."""
    text = readme_text()
    m = re.search(rf"^#### {re.escape(heading)}\n(.*?)(?=^#{{2,4}} )", text, re.M | re.S)
    assert m, f"README has no '#### {heading}' section"
    return m.group(1)


def table_rows(body, first_header):
    """The data rows of the markdown table whose header row starts with
    `| first_header |`, keyed by their first cell with backticks stripped."""
    lines = body.splitlines()
    starts = [i for i, line in enumerate(lines) if line.startswith(f"| {first_header} |")]
    assert len(starts) == 1, f"expected one table headed '{first_header}', found {len(starts)}"
    rows = {}
    for line in lines[starts[0] + 2:]:
        if not line.startswith("|"):
            break
        cells = [c.strip() for c in line.strip("|").split("|")]
        rows[cells[0].strip("`")] = " | ".join(cells[1:])
    return rows


def field_rows():
    return table_rows(section("Run metrics"), "Field")


def mentions(text, key):
    return f"`{key}`" in text


# Acceptance 3: the job layout includes the new record and test results.


def test_readme_exists():
    assert README.is_file()


@pytest.mark.parametrize("name", ["junit.xml", "metrics.json"])
def test_results_tree_lists_file_under_verifier(name):
    body = section("Test results")
    tree = re.search(r"```\n(.*?)```", body, re.S)
    assert tree, "Test results section has no layout block"
    lines = tree.group(1).splitlines()
    at = [i for i, line in enumerate(lines) if line.rstrip().endswith("verifier/") or "verifier/ " in line]
    assert len(at) == 1, "the layout has no single verifier/ directory"
    verifier_indent = lines[at[0]].index("verifier/")
    children = []
    for line in lines[at[0] + 1:]:
        name_at = re.search(r"[├└]── ", line)
        if not name_at or name_at.start() <= verifier_indent - 4:
            break
        children.append(line[name_at.end():].split()[0])
    assert name in children, f"{name} not listed under verifier/: {children}"


# Acceptance 1: every field of the record is explained.


def test_field_table_names_every_top_level_key():
    assert sorted(field_rows()) == sorted(TOP_LEVEL_KEYS)


@pytest.mark.parametrize(
    "parent,keys",
    [("run", RUN_KEYS), ("totals", TOTALS_KEYS), ("subagents", SUBAGENTS_KEYS)],
)
def test_field_table_names_every_nested_key(parent, keys):
    row = field_rows()[parent]
    missing = [k for k in keys if not mentions(row, k)]
    assert not missing, f"the {parent} row does not name {missing}"


def test_field_table_names_every_check_key():
    row = field_rows()["result"]
    assert mentions(row, "checks")
    missing = [k for k in CHECKS_KEYS if not mentions(row, k)]
    assert not missing, f"the result row does not name checks' {missing}"


@pytest.mark.parametrize("agent", ["Bob", "Codex"])
def test_unknowns_table_has_row_for_agent(agent):
    rows = table_rows(section("Run metrics"), "Agent")
    assert agent in rows
    assert rows[agent].strip(), f"the {agent} row lists nothing"


# Acceptance 2: how to run the report, one run and a comparison.


@pytest.mark.parametrize("needle", ["make harbor-report", "--run", "--baseline", "--candidate"])
def test_comparing_runs_shows(needle):
    body = section("Comparing runs")
    commands = re.findall(r"```bash\n(.*?)```", body, re.S)
    assert commands, "Comparing runs has no command block"
    assert any(needle in block for block in commands), f"{needle} not in a command"


def test_comparison_command_passes_both_sets_in_one_args():
    body = section("Comparing runs")
    assert re.search(r"make harbor-report ARGS=\"--baseline '[^']+' --candidate '[^']+'\"", body)


def test_shared_self_check_command_is_present():
    text = readme_text()
    blocks = re.findall(r"```bash\n(.*?)```", text, re.S)
    assert any(
        line.startswith("uvx --with pytest pytest") and line.rstrip().endswith("tests/harbor/shared/tests")
        for block in blocks
        for line in block.splitlines()
    )
