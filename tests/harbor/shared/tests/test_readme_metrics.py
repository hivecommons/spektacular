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
    "trace_calls",
    "trace_calls_per_task",
    "user_wait_s",
    "helper_wait_s",
    "gap_s",
    "gaps",
    "gap_threshold_s",
]
SUBAGENTS_KEYS = ["started", "completed", "failed", "unfinished", "durations_s", "helpers", "briefs"]
# Each workflow run under steps.workflows, and each step in its steps.
WORKFLOW_KEYS = ["run_id", "kind", "name", "status", "wall_s", "user_wait_s", "helper_wait_s", "gap_s", "gaps"]
STEP_KEYS = [
    "step",
    "started_at",
    "entries",
    "duration_s",
    "working_s",
    "helper_wait_s",
    "user_wait_s",
    "commands",
    "helpers",
    "incomplete",
]
# Each helper under subagents.helpers, from Spektacular's logs.
HELPER_KEYS = [
    "span_id",
    "parent",
    "name",
    "spec",
    "kind",
    "step",
    "task_id",
    "outside_run",
    "started_at",
    "ended_at",
    "duration_s",
    "end_reason",
    "status",
    "complete",
]
# Each brief under subagents.briefs, from the agent's log.
BRIEF_KEYS = ["id", "parent", "kind", "description", "models", "prompt", "prompt_chars"]
SCHEMA_VERSION = "3"
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


def test_schema_version_row_says_current_version():
    row = field_rows()["schema_version"]
    assert re.search(rf"currently `{SCHEMA_VERSION}`", row), f"the schema_version row does not say `{SCHEMA_VERSION}`"


def test_field_table_names_every_workflow_key():
    row = field_rows()["steps"]
    assert mentions(row, "workflows")
    missing = [k for k in WORKFLOW_KEYS if not mentions(row, k)]
    assert not missing, f"the steps row does not name each workflow's {missing}"


def test_field_table_names_every_step_key():
    row = field_rows()["steps"]
    missing = [k for k in STEP_KEYS if not mentions(row, k)]
    assert not missing, f"the steps row does not name each step's {missing}"


def test_steps_row_says_the_split_adds_up_to_the_duration():
    row = field_rows()["steps"]
    assert "add up to its duration" in row


def test_field_table_names_every_helper_key():
    row = field_rows()["subagents"]
    missing = [k for k in HELPER_KEYS if not mentions(row, k)]
    assert not missing, f"the subagents row does not name each helper's {missing}"


def test_subagents_row_says_an_outside_run_helper_has_no_step():
    row = field_rows()["subagents"]
    assert mentions(row, "outside_run") and "outside any run" in row and mentions(row, "null")


def test_field_table_names_every_brief_key():
    row = field_rows()["subagents"]
    missing = [k for k in BRIEF_KEYS if not mentions(row, k)]
    assert not missing, f"the subagents row does not name each brief's {missing}"


def test_bob_row_says_which_brief_fields_it_records():
    row = table_rows(section("Run metrics"), "Agent")["Bob"]
    missing = [k for k in ["kind", "description", "prompt"] if not mentions(row, k)]
    assert not missing, f"the Bob row does not name the brief fields it keeps: {missing}"
    assert mentions(row, "models") and "unknown" in row, "the Bob row does not say its briefs' models are unknown"


def test_codex_row_says_briefs_are_unknown():
    row = table_rows(section("Run metrics"), "Agent")["Codex"]
    assert mentions(row, "briefs"), "the Codex row does not name briefs"
    assert mentions(row, "null"), "the Codex row does not say briefs is null"


def test_metrics_section_says_log_fields_come_through_stats_and_never_from_the_transcript():
    body = section("Run metrics")
    assert "`spektacular stats`" in body, "Run metrics does not say the log fields are read through `spektacular stats`"
    for sub in ("list", "run", "commands", "spans"):
        assert mentions(body, sub), f"Run metrics does not name `stats {sub}`"
    flat = " ".join(body.split())
    assert "never by parsing them" in flat
    assert "none is filled in from the agent's transcript" in flat, (
        "Run metrics does not say a log field is never filled in from the transcript"
    )


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


def test_comparing_runs_shows_prompts_with_run():
    body = section("Comparing runs")
    commands = re.findall(r"```bash\n(.*?)```", body, re.S)
    lines = [line for block in commands for line in block.splitlines() if "--prompts" in line]
    assert lines, "--prompts not in a command"
    assert any(re.search(r"ARGS='--run \S+ --prompts'", line) for line in lines), "--prompts not shown with --run"


def test_comparing_runs_says_prompts_needs_run():
    body = section("Comparing runs")
    assert "`--prompts`" in body and "usage error" in body


def test_comparing_runs_describes_idle_mark():
    body = section("Comparing runs")
    assert "`◀ idle`" in body, "Comparing runs does not show the idle mark"
    assert "quarter" in body, "Comparing runs does not say when a run is marked idle"


def test_comparing_runs_describes_child_table():
    body = section("Comparing runs")
    assert "child agents" in body and "nested" in body


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
