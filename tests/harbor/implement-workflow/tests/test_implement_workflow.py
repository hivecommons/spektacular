"""Verify the spektacular implement workflow's changelog behaviour end to end.

The environment pre-seeds a multi-repo project (`auth-service`) whose plan
implementation is complete: two phases touched the colocated `auth` repo,
one phase touched the separate `docs` repo (Spektacular files under the
project at /app/repos/docs, code at /opt/docs-repo via a file source), all
phases are checked off, and the workflow state is positioned at `test_plan`. The driving agent
runs `/spek:implement` from that point through `finished`.

This verifier is intentionally narrow. It exists to catch the class of
regression that motivated its creation: the implement workflow silently
skipping the changelog write (the finished step used to swallow
store.ErrNotFound and mark the workflow finished with no record on
disk), or writing to the wrong path (the projects PR briefly wrote the
project-level record under a `<project>/` subfolder that collides with
the colocated repo-level write when project root == repo root).

Every oracle here is hand-maintained. Do NOT derive expected paths from
`spektacular` at runtime — that closes the loop and defeats the point of
an independent behavioural check. When the changelog path model changes,
update the constants below in the same commit as the code change.
"""

import json
import re
from dataclasses import dataclass
from pathlib import Path

import pytest


# ---------------------------------------------------------------------------
# Paths and constants — hand-maintained behavioural oracle
# ---------------------------------------------------------------------------

PROJECT_DIR = Path("/app")
PROJECT_SPEK_DIR = PROJECT_DIR / ".spektacular"
# The docs repo's Spektacular files live under the project (its registry
# `location`); its code lives elsewhere (its repo.yaml `source`). Nothing
# Spektacular writes may land in the source.
DOCS_REPO_SPEK_DIR = PROJECT_DIR / "repos" / "docs" / ".spektacular"
EXTERNAL_REPO_SOURCE = Path("/opt/docs-repo")

TRANSCRIPT = Path("/logs/agent/claude-code.txt")

# Constants pinned from the environment: project name from config.yaml,
# plan name from state.json's data.name, member names from config.yaml's
# repos list. Update in lockstep with environment/config.yaml and
# environment/state.json.
PROJECT_NAME = "auth-service"
PLAN_NAME = "20260101000000-jwt-auth"
COLOCATED_REPO_NAME = "auth"
EXTERNAL_REPO_NAME = "docs"

# Canonical FSM step order for the implement workflow. Update in lockstep
# with internal/steps/implement/steps.go Steps().
EXPECTED_STEP_ORDER = (
    "new",
    "read_plan",
    "analyze",
    "implement",
    "test",
    "verify",
    "update_plan",
    "update_changelog",
    "test_plan",
    "update_feature_changelog",
    "reconcile_spec",
    "finished",
)

# Expected on-disk changelog paths. These are the whole point of the test —
# encode them literally so a path-model regression fails loudly.
#
# Project-level: flat under the project's configured changelog dir, no
# `<project>/` subfolder. This is what the projects PR broke and the
# fix restored.
PROJECT_CHANGELOG_PATH = PROJECT_SPEK_DIR / "changelog" / f"{PLAN_NAME}.md"

# Colocated repo-level: under the SAME .spektacular/changelog directory
# but namespaced by `<project>/`. Because the colocated repo shares its
# working tree with the project, this and PROJECT_CHANGELOG_PATH must NOT
# collide — the `auth-service/` subfolder is what keeps them distinct.
COLOCATED_REPO_CHANGELOG_PATH = (
    PROJECT_SPEK_DIR / "changelog" / PROJECT_NAME / f"{PLAN_NAME}.md"
)

# External repo-level: inside the docs repo's own changelog store — which
# sits under the project folder, not in the code checkout — namespaced by
# `<project>/` so multiple projects sharing this repo do not collide.
EXTERNAL_REPO_CHANGELOG_PATH = (
    DOCS_REPO_SPEK_DIR / "changelog" / PROJECT_NAME / f"{PLAN_NAME}.md"
)

# CLI substrings that must appear in Bash tool_use inputs. Substrings, not
# exact matches, so `--from ...` argument tail is free to vary.
PROJECT_WRITE_SUBSTR = f"changelog file write {PLAN_NAME}.md --from"
COLOCATED_WRITE_SUBSTR = (
    f"changelog file write {PLAN_NAME}.md --repo {COLOCATED_REPO_NAME}"
)
EXTERNAL_WRITE_SUBSTR = (
    f"changelog file write {PLAN_NAME}.md --repo {EXTERNAL_REPO_NAME}"
)

# The finished template renders these substrings — used to prove the
# workflow rendered the finished step at least once.
FINISHED_TEMPLATE_MARKER = "This is the terminal state of the implement workflow"

# Built-in file-mutation tools. If any of these targeted a changelog
# record path, the agent bypassed the CLI — a hard failure.
BUILTIN_FILE_TOOLS = frozenset({"Write", "Edit", "MultiEdit", "NotebookEdit"})

STATUS_FINAL = "final"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ToolCall:
    index: int
    type: str
    input: dict


def load_state() -> dict:
    state_file = PROJECT_SPEK_DIR / "state.json"
    assert state_file.exists(), f"state.json missing at {state_file}"
    return json.loads(state_file.read_text())


FRONTMATTER_RE = re.compile(r"\A---\n(.*?)\n---[ \t]*\n", re.DOTALL)
FRONTMATTER_LINE_RE = re.compile(r"^([A-Za-z0-9_-]+):\s*(.*)$")


def parse_frontmatter(text: str) -> dict:
    m = FRONTMATTER_RE.match(text)
    if not m:
        return {}
    fields: dict = {}
    for line in m.group(1).splitlines():
        lm = FRONTMATTER_LINE_RE.match(line)
        if not lm:
            continue
        value = lm.group(2).strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        fields[lm.group(1)] = value
    return fields


def _iter_transcript_objects():
    if not TRANSCRIPT.exists():
        pytest.fail(f"Agent transcript not found at {TRANSCRIPT} — the agent never ran")
    for line in TRANSCRIPT.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            yield json.loads(line)
        except json.JSONDecodeError:
            continue


def extract_tool_calls() -> list:
    """Ordered Bash / Skill / Task / Write / Edit tool_use blocks."""
    calls: list = []
    interesting = {"Bash", "Skill", "Task", "Agent"} | BUILTIN_FILE_TOOLS
    for obj in _iter_transcript_objects():
        if obj.get("type") != "assistant":
            continue
        for block in obj.get("message", {}).get("content", []):
            if block.get("type") != "tool_use":
                continue
            name = block.get("name", "")
            if name in interesting:
                calls.append(
                    ToolCall(
                        index=len(calls),
                        type=name,
                        input=block.get("input", {}) or {},
                    )
                )
    return calls


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="module")
def state() -> dict:
    return load_state()


@pytest.fixture(scope="module")
def tool_calls() -> list:
    return extract_tool_calls()


# ---------------------------------------------------------------------------
# Layer 1: workflow integrity
# ---------------------------------------------------------------------------


def test_workflow_reached_finished(state):
    assert state.get("current_step") == "finished", (
        f"workflow did not reach finished — current_step is {state.get('current_step')!r}. "
        "If the run failed at the finished step with a `changelog_missing` error, "
        "that IS the regression this test is meant to catch: update_feature_changelog "
        "never wrote the project-level record"
    )


def test_completed_steps_include_full_fsm(state):
    completed = state.get("completed_steps") or []
    for step in EXPECTED_STEP_ORDER:
        assert step in completed, (
            f"step {step!r} missing from completed_steps — the workflow did not "
            f"traverse the full FSM. completed_steps: {completed}"
        )


def test_state_data_carries_plan_name(state):
    data = state.get("data") or {}
    assert data.get("name") == PLAN_NAME, (
        f"state.data.name is {data.get('name')!r}, expected {PLAN_NAME!r} — "
        "the seeded state was clobbered or the workflow's data plumbing broke"
    )


# ---------------------------------------------------------------------------
# Layer 2: changelog artifacts land at the correct paths
# ---------------------------------------------------------------------------


def test_project_level_changelog_exists_at_flat_path():
    assert PROJECT_CHANGELOG_PATH.exists(), (
        f"project-level changelog missing at {PROJECT_CHANGELOG_PATH} — this is "
        "the exact regression the fix addressed. update_feature_changelog must "
        "commit the aggregate record with `changelog file write <plan>.md --from ...` "
        "(no --repo flag). The finished step should have failed hard if this "
        "was not written, so seeing this test fail while `finished` was reached "
        "means the finished-step guard was also regressed."
    )


def test_colocated_repo_changelog_exists_under_project_subfolder():
    assert COLOCATED_REPO_CHANGELOG_PATH.exists(), (
        f"colocated repo-level changelog missing at {COLOCATED_REPO_CHANGELOG_PATH} — "
        f"update_feature_changelog must commit the colocated repo's own record with "
        f"`changelog file write <plan>.md --repo {COLOCATED_REPO_NAME} --from ...`. "
        "Every affected repo, INCLUDING the colocated one, gets its own repo-level "
        "record; there is no 'the project-level already covers it' carve-out."
    )


def test_external_repo_changelog_exists_under_project_subfolder():
    assert EXTERNAL_REPO_CHANGELOG_PATH.exists(), (
        f"external repo-level changelog missing at {EXTERNAL_REPO_CHANGELOG_PATH} — "
        f"update_feature_changelog must commit the docs repo's own record with "
        f"`changelog file write <plan>.md --repo {EXTERNAL_REPO_NAME} --from ...`. "
        "The write routes into the external repo's own changelog store, under a "
        f"{PROJECT_NAME}/ subfolder so multiple projects sharing that repo cannot collide."
    )


def test_project_and_repo_records_have_distinct_content():
    """The project-level record aggregates all changes across every affected
    repo; the colocated repo-level record covers only the auth repo's slice.
    They should not be byte-identical.
    """
    project = PROJECT_CHANGELOG_PATH.read_text()
    colocated = COLOCATED_REPO_CHANGELOG_PATH.read_text()
    assert project != colocated, (
        "the project-level record and the colocated repo-level record are byte-identical — "
        "one of the two writes clobbered the other, or the agent wrote the same content twice. "
        "The project-level record must aggregate across all repos; each repo record must be "
        "scoped to only its own changes"
    )


# ---------------------------------------------------------------------------
# Layer 3: metadata — finished step must stamp document_status=final on the
# project-level record. This exercises the changelog-artifact half of the
# finished-step guard: had the guard swallowed ErrNotFound (the pre-fix
# behaviour), no document status would ever get stamped.
# ---------------------------------------------------------------------------


def test_project_changelog_frontmatter_document_status_final():
    fm = parse_frontmatter(PROJECT_CHANGELOG_PATH.read_text())
    assert fm.get("document_status") == STATUS_FINAL, (
        f"project-level changelog document_status is {fm.get('document_status')!r}, expected {STATUS_FINAL!r} — "
        "the finished step's metadata.Close(changelogPath, StatusFinal) call did not "
        "run or did not persist. If document_status is `draft`, the finished step returned "
        "early or its post-condition on the changelog was skipped"
    )


# ---------------------------------------------------------------------------
# Layer 4: the agent used the CLI, not built-in file tools, to commit the
# changelog records. Write/Edit on `.spektacular/changelog/**` = hard fail —
# scratch staging at `.spektacular/tmp/changelog_*.md` is fine.
# ---------------------------------------------------------------------------


def test_no_builtin_file_tool_wrote_a_changelog_record(tool_calls):
    """The changelog store is the CLI's to manage. Every commit must go
    through `changelog file write`. Built-in Write/Edit on scratch staging
    paths under .spektacular/tmp/ is explicitly allowed."""
    offenders = []
    for c in tool_calls:
        if c.type not in BUILTIN_FILE_TOOLS:
            continue
        path = c.input.get("file_path") or c.input.get("notebook_path") or ""
        norm = str(path)
        if "/.spektacular/changelog/" in norm and "/.spektacular/tmp/" not in norm:
            offenders.append((c.type, norm))
    assert not offenders, (
        "the agent wrote a changelog record with a built-in file tool instead of the CLI. "
        f"Offenders: {offenders}. Every changelog write must go through "
        "`spektacular changelog file write` — that is the only path the CLI's provenance "
        "stamping and metadata.Merge runs through"
    )


def test_project_level_changelog_was_committed_via_cli(tool_calls):
    for c in tool_calls:
        if c.type != "Bash":
            continue
        cmd = str(c.input.get("command", ""))
        if PROJECT_WRITE_SUBSTR in cmd and "--repo" not in cmd:
            return
    pytest.fail(
        f"no Bash call ran `{PROJECT_WRITE_SUBSTR}` (without --repo). "
        "The project-level changelog record MUST be committed through the CLI"
    )


def test_colocated_repo_changelog_was_committed_via_cli(tool_calls):
    for c in tool_calls:
        if c.type != "Bash":
            continue
        cmd = str(c.input.get("command", ""))
        if COLOCATED_WRITE_SUBSTR in cmd:
            return
    pytest.fail(
        f"no Bash call ran `{COLOCATED_WRITE_SUBSTR}`. "
        f"The colocated `{COLOCATED_REPO_NAME}` repo's per-repo record MUST be "
        f"committed through the CLI with --repo {COLOCATED_REPO_NAME}"
    )


def test_external_repo_changelog_was_committed_via_cli(tool_calls):
    for c in tool_calls:
        if c.type != "Bash":
            continue
        cmd = str(c.input.get("command", ""))
        if EXTERNAL_WRITE_SUBSTR in cmd:
            return
    pytest.fail(
        f"no Bash call ran `{EXTERNAL_WRITE_SUBSTR}`. "
        f"The external `{EXTERNAL_REPO_NAME}` repo's per-repo record MUST be "
        f"committed through the CLI with --repo {EXTERNAL_REPO_NAME}"
    )


# ---------------------------------------------------------------------------
# Layer 5: a repo whose Spektacular files live apart from its code gets
# nothing written into the code. This is plan 000046's success metric — a
# clean `git status` in the code repository apart from intended code changes.
# ---------------------------------------------------------------------------


def test_separate_repo_source_has_no_spektacular_dir():
    assert EXTERNAL_REPO_SOURCE.is_dir(), f"{EXTERNAL_REPO_SOURCE} must exist as the docs repo's code"
    assert not (EXTERNAL_REPO_SOURCE / ".spektacular").exists(), (
        f"{EXTERNAL_REPO_SOURCE}/.spektacular was created — the docs repo's Spektacular "
        f"files belong under its registered location {DOCS_REPO_SPEK_DIR}, never in its source"
    )


def test_no_changelog_md_written_anywhere():
    offenders = [
        str(p)
        for root in (EXTERNAL_REPO_SOURCE, PROJECT_DIR)
        for p in root.rglob("CHANGELOG.md")
    ]
    assert not offenders, (
        f"a root CHANGELOG.md was written: {offenders}. The implement workflow writes "
        "exactly one project record and one record per affected repo, each at the "
        "location its configuration declares, and no other changelog file"
    )

