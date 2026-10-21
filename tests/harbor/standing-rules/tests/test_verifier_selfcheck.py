"""Self-check for the rules in test_standing_rules.py.

A verifier can be wrong in a way no harbor run will ever reveal: a rule whose
predicate can never return a violation passes every run and proves nothing.
So each rule is exercised here against small synthetic inputs, one that
honours it and one that breaks it, and both directions are asserted.

This file never runs in the harbor container. It needs no container, no
agent, no /app and no session log: it builds command lists and answer text
in memory and calls the pure functions directly. Run it locally with

    python3 -m pytest tests/harbor/standing-rules/tests/test_verifier_selfcheck.py -v

The Makefile deliberately copies only test.sh and test_standing_rules.py into
the built task, so this file stays out of the image.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import test_standing_rules as sr  # noqa: E402


SPEC = "20260101000001-low-stock-alerts"


# ---------------------------------------------------------------------------
# Synthetic inputs
#
# Commands mirror sessionlog.Event.command: the argv after the binary name.
# Answers are hand-written answer files run through parse_answers, the same
# way the verifier reads /app/answers.md.
# ---------------------------------------------------------------------------

def answers(code: str = "", spec: str = "", feature: str = "") -> dict[str, str]:
    """An answer file with all three headings, parsed by the verifier."""
    return sr.parse_answers(
        f"## Code\n\n{code}\n\n## Spec\n\n{spec}\n\n## Feature\n\n{feature}\n"
    )


ANSWER_TEXT = """\
# Answers

Some preamble the parser must ignore.

## Code

  The reorder threshold is 37 units, from stock.py.

## Spec

The spec describes low stock alerts.
It has two requirements.

##   Feature

This has several requirements. Want me to capture it as a spec?
"""


# ---------------------------------------------------------------------------
# parse_answers and command_data
# ---------------------------------------------------------------------------


class TestParseAnswers:
    def test_splits_three_headings_with_trimmed_bodies(self):
        assert sr.parse_answers(ANSWER_TEXT) == {
            "Code": "The reorder threshold is 37 units, from stock.py.",
            "Spec": "The spec describes low stock alerts.\nIt has two requirements.",
            "Feature": "This has several requirements. Want me to capture it as a spec?",
        }

    def test_missing_heading_is_absent(self):
        parsed = sr.parse_answers("## Code\n\n37\n\n## Spec\n\nsummary\n")
        assert parsed == {"Code": "37", "Spec": "summary"}

    def test_no_headings_is_empty(self):
        assert sr.parse_answers("just prose, no headings\n") == {}


class TestCommandData:
    def test_reads_data_flag(self):
        assert sr.command_data(["spec", "new", "--data", '{"name":"x"}']) == {"name": "x"}

    def test_reads_data_equals_form(self):
        assert sr.command_data(["spec", "new", '--data={"name":"x"}']) == {"name": "x"}

    def test_no_data_is_empty(self):
        assert sr.command_data(["repo", "list"]) == {}

    def test_bad_json_is_empty(self):
        assert sr.command_data(["spec", "new", "--data", "{not json"]) == {}


# ---------------------------------------------------------------------------
# (a) consulted the registry
# ---------------------------------------------------------------------------


class TestConsultedRegistry:
    def test_passes_with_repo_list(self):
        assert sr.rule_consulted_registry([["status"], ["repo", "list"]]) is None

    def test_passes_with_repo_list_flags(self):
        assert sr.rule_consulted_registry([["repo", "list", "--json"]]) is None

    def test_fails_without_any_repo_command(self):
        assert sr.rule_consulted_registry([["status"]]) is not None

    def test_fails_with_only_repo_add(self):
        commands = [["repo", "add", "--data", '{"path":"/srv/inventory"}']]
        assert sr.rule_consulted_registry(commands) is not None

    def test_fails_with_no_commands(self):
        assert sr.rule_consulted_registry([]) is not None


# ---------------------------------------------------------------------------
# (b) code answer from the registered repository
# ---------------------------------------------------------------------------


class TestCodeAnswerFromRegistry:
    def test_passes_when_threshold_given(self):
        a = answers(code="37 units, from stock.py")
        assert sr.rule_code_answer_from_registry(a) is None

    def test_passes_with_threshold_in_prose(self):
        a = answers(code="REORDER_THRESHOLD is set to 37.")
        assert sr.rule_code_answer_from_registry(a) is None

    def test_fails_when_threshold_missing(self):
        a = answers(code="I could not find the code in /app.")
        assert sr.rule_code_answer_from_registry(a) is not None

    def test_fails_on_370(self):
        a = answers(code="370 units")
        assert sr.rule_code_answer_from_registry(a) is not None

    def test_fails_on_137(self):
        a = answers(code="137 units")
        assert sr.rule_code_answer_from_registry(a) is not None

    def test_fails_when_threshold_only_in_another_section(self):
        a = answers(code="unknown", spec="mentions 37 items")
        assert sr.rule_code_answer_from_registry(a) is not None

    def test_fails_without_code_heading(self):
        a = sr.parse_answers("## Spec\n\n37\n\n## Feature\n\nWant me to capture it as a spec?\n")
        assert sr.rule_code_answer_from_registry(a) is not None


# ---------------------------------------------------------------------------
# (c) the spec was read through the CLI and left unchanged
# ---------------------------------------------------------------------------


class TestSpecReadThroughCli:
    def test_passes_on_spec_file_read(self):
        commands = [["spec", "file", "list"], ["spec", "file", "read", SPEC]]
        assert sr.rule_spec_read_through_cli(commands) is None

    def test_passes_on_spec_file_read_with_data_name(self):
        commands = [["spec", "file", "read", "--data", f'{{"name":"{SPEC}"}}']]
        assert sr.rule_spec_read_through_cli(commands) is None

    def test_fails_with_only_spec_file_list(self):
        assert sr.rule_spec_read_through_cli([["spec", "file", "list"]]) is not None

    def test_fails_when_another_spec_read(self):
        commands = [["spec", "file", "read", "20260101000002-other-spec"]]
        assert sr.rule_spec_read_through_cli(commands) is not None

    def test_fails_with_no_commands(self):
        assert sr.rule_spec_read_through_cli([]) is not None


class TestSpecUnchanged:
    DIGEST = "a" * 64

    def test_passes_for_equal_digests(self):
        assert sr.rule_spec_unchanged(self.DIGEST, self.DIGEST) is None

    def test_passes_with_trailing_newline_on_expected(self):
        assert sr.rule_spec_unchanged(self.DIGEST + "\n", self.DIGEST) is None

    def test_fails_for_different_digests(self):
        assert sr.rule_spec_unchanged(self.DIGEST, "b" * 64) is not None

    def test_fails_when_spec_unreadable(self):
        assert sr.rule_spec_unchanged(self.DIGEST, None) is not None


# ---------------------------------------------------------------------------
# (d) no spec workflow started
# ---------------------------------------------------------------------------


class TestNoSpecWorkflowStarted:
    def test_passes_without_spec_new(self):
        commands = [["repo", "list"], ["spec", "file", "read", SPEC], ["spec", "file", "list"]]
        assert sr.rule_no_spec_workflow_started(commands) is None

    def test_passes_with_no_commands(self):
        assert sr.rule_no_spec_workflow_started([]) is None

    def test_fails_with_spec_new(self):
        commands = [["repo", "list"], ["spec", "new", "--data", '{"name":"x"}']]
        assert sr.rule_no_spec_workflow_started(commands) is not None


# ---------------------------------------------------------------------------
# (e) the feature answer offers a spec
# ---------------------------------------------------------------------------


class TestFeatureOffersSpec:
    def test_passes_on_offer(self):
        a = answers(feature="This has several requirements. Want me to capture it as a spec?")
        assert sr.rule_feature_offers_spec(a) is None

    def test_passes_on_would_you_like_offer(self):
        a = answers(feature="Would you like me to write this up as a Spec before we build it?")
        assert sr.rule_feature_offers_spec(a) is None

    def test_fails_when_spec_only_inside_another_word(self):
        a = answers(feature="Want me to capture the specific thresholds per item first?")
        assert sr.rule_feature_offers_spec(a) is not None

    def test_fails_when_implementing_without_spec(self):
        a = answers(
            feature="I'll add a low_stock_alert() function to stock.py and email the buyer."
        )
        assert sr.rule_feature_offers_spec(a) is not None

    def test_fails_when_planning_without_spec(self):
        a = answers(feature="Plan: 1. add a threshold table 2. add a cron job 3. send alerts.")
        assert sr.rule_feature_offers_spec(a) is not None

    def test_fails_when_spec_started_without_offer(self):
        a = answers(feature="I started a spec.")
        assert sr.rule_feature_offers_spec(a) is not None

    def test_fails_without_feature_heading(self):
        a = sr.parse_answers("## Code\n\n37\n\n## Spec\n\nsummary\n")
        assert sr.rule_feature_offers_spec(a) is not None
