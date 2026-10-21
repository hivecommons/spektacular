"""Verify that the spektacular spec workflow completed successfully.

Tests are grouped by behaviour: one class per workflow step, plus a class
for overall workflow integrity.  Each step class checks that the step was
completed, that the agent called the spektacular CLI for it, and that it
produced meaningful, on-topic content in the spec file.
"""

import json
import re
from pathlib import Path

PROJECT_DIR = Path("/app")
SPEK_DIR = PROJECT_DIR / ".spektacular"
STATE_FILE = SPEK_DIR / "state.json"
AGENT_LOG_DIR = Path("/logs/agent")

# The canonical step order as defined by the state machine in
# internal/steps/spec/steps.go → Steps().
EXPECTED_STEP_ORDER = [
    "new",
    "interview",
    "overview",
    "requirements",
    "acceptance_criteria",
    "constraints",
    "technical_approach",
    "success_metrics",
    "non_goals",
    "verification",
    "split",
    "finished",
]

# Minimum character count for each section's content (excluding comments).
MIN_SECTION_LENGTH = 100

# Built-in agent tools that mutate files directly, bypassing the spektacular
# CLI. The spec file must never be written with these.
BUILTIN_FILE_TOOLS = {"Write", "Edit", "MultiEdit", "NotebookEdit"}


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def load_state() -> dict:
    assert STATE_FILE.exists(), f"State file not found at {STATE_FILE}"
    return json.loads(STATE_FILE.read_text())


def spec_file_path() -> Path:
    state = load_state()
    spec_name = (state.get("data") or {}).get("name")
    assert spec_name, "state.json has no data.name — cannot resolve spec file"
    return SPEK_DIR / "specs" / f"{spec_name}.md"


def load_spec() -> str:
    path = spec_file_path()
    assert path.exists(), f"Spec file not found at {path}"
    return path.read_text()


def parse_sections(spec_text: str) -> dict[str, str]:
    """Split the spec markdown into sections keyed by heading (lowercase).

    Returns a dict like {"overview": "section body …", "requirements": "…"}.
    Content between HTML comments (<!-- … -->) is stripped so we only measure
    real content the agent wrote.
    """
    cleaned = re.sub(r"<!--.*?-->", "", spec_text, flags=re.DOTALL)

    sections: dict[str, str] = {}
    current_heading = None
    current_lines: list[str] = []

    for line in cleaned.splitlines():
        heading_match = re.match(r"^##\s+(.+)", line)
        if heading_match:
            if current_heading is not None:
                sections[current_heading] = "\n".join(current_lines).strip()
            current_heading = heading_match.group(1).strip().lower()
            current_lines = []
        elif current_heading is not None:
            current_lines.append(line)

    if current_heading is not None:
        sections[current_heading] = "\n".join(current_lines).strip()

    return sections


def extract_tool_calls() -> list[dict]:
    """Parse the agent transcript and return all Bash tool calls in order.

    Each entry is {"command": "…"} for Bash tool_use blocks.
    """
    transcripts = sorted(AGENT_LOG_DIR.glob("*.txt"))
    assert transcripts, f"No agent transcripts found under {AGENT_LOG_DIR}"

    calls = []
    for transcript in transcripts:
        for line in transcript.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            if obj.get("type") == "item.completed":
                item = obj.get("item", {})
                if item.get("type") == "command_execution":
                    cmd = item.get("command", "")
                    if cmd:
                        calls.append({"command": cmd})
                    continue
            if obj.get("type") != "assistant":
                continue
            msg = obj.get("message", {})
            for block in msg.get("content", []):
                if block.get("type") == "tool_use" and block.get("name") == "Bash":
                    cmd = block.get("input", {}).get("command", "")
                    if cmd:
                        calls.append({"command": cmd})
    return calls


def find_spektacular_calls(tool_calls: list[dict]) -> list[str]:
    """Return an ordered list of spektacular spec commands from tool calls.

    Returns entries like "spec new" or "spec goto overview".
    """
    results = []
    for call in tool_calls:
        cmd = call["command"]
        if "spektacular spec new" in cmd:
            results.append("spec new")
        elif "spektacular spec goto" in cmd:
            normalized = cmd.replace('\\"', '"').replace("\\'", "'")
            m = re.search(r'["\']step["\']\s*:\s*["\']([a-z_]+)["\']', normalized)
            if m:
                results.append(f"spec goto {m.group(1)}")
                continue

            for step in EXPECTED_STEP_ORDER:
                if step == "new":
                    continue
                if re.search(rf"\b{re.escape(step)}\b", normalized):
                    results.append(f"spec goto {step}")
                    break
    return results


def extract_builtin_file_edits() -> list[dict]:
    """Return every built-in file-mutation tool call (Write/Edit/...) in order.

    Each entry is {"name": <tool>, "file_path": <path>}. These are the tools the
    agent must NOT use on the spec file — only Claude-style `tool_use` blocks
    carry them.
    """
    edits = []
    for obj in iter_transcript_objects():
        if obj.get("type") != "assistant":
            continue
        for block in obj.get("message", {}).get("content", []):
            if block.get("type") != "tool_use":
                continue
            if block.get("name") not in BUILTIN_FILE_TOOLS:
                continue
            inp = block.get("input", {})
            path = inp.get("file_path") or inp.get("notebook_path") or ""
            edits.append({"name": block["name"], "file_path": path})
    return edits


def iter_transcript_objects():
    """Yield every JSON object across all agent transcripts, in order."""
    for transcript in sorted(AGENT_LOG_DIR.glob("*.txt")):
        for line in transcript.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue


def agent_result_events() -> list[dict]:
    """Return all `result`-type events from the transcript, in order."""
    return [o for o in iter_transcript_objects() if o.get("type") == "result"]


def agent_auth_failure():
    """Return the first transcript object signalling an Anthropic API
    authentication failure (HTTP 401), or None if there is none."""
    for obj in iter_transcript_objects():
        if obj.get("error") == "authentication_failed":
            return obj
        if obj.get("api_error_status") == 401:
            return obj
    return None


# ---------------------------------------------------------------------------
# Agent-execution preflight — runs first
# ---------------------------------------------------------------------------

class TestAgentExecution:
    """Preflight: the Claude Code agent authenticated and actually ran.

    Defined first so it runs before any workflow assertion. If the agent
    never started — e.g. an invalid ANTHROPIC_API_KEY — then config.yaml,
    state.json and every spec file will be missing, and the downstream
    failures are misleading ("config.yaml missing — init not run" when the
    real cause is a 401). When this class fails, read it first: every other
    failure in the run is a consequence.
    """

    def test_transcript_exists(self):
        transcripts = sorted(AGENT_LOG_DIR.glob("*.txt"))
        assert transcripts, (
            f"No agent transcript under {AGENT_LOG_DIR} — the agent never ran."
        )

    def test_agent_authenticated(self):
        failure = agent_auth_failure()
        assert failure is None, (
            "Agent failed to authenticate with the Anthropic API "
            f"(api_error_status={failure.get('api_error_status')}): "
            f"{failure.get('result') or failure.get('error')!r}. "
            "Set a valid ANTHROPIC_API_KEY before running harbor — every "
            "other failure in this run is a consequence of this."
        )

    def test_agent_run_succeeded(self):
        results = agent_result_events()
        assert results, (
            "No `result` event in the transcript — the agent did not finish."
        )
        final = results[-1]
        assert not final.get("is_error"), (
            f"Agent run ended in error: {final.get('result')!r} "
            f"(api_error_status={final.get('api_error_status')}, "
            f"num_turns={final.get('num_turns')})."
        )

    def test_agent_did_work(self):
        calls = extract_tool_calls()
        assert calls, (
            "Agent produced no tool calls — it never ran 'spektacular init' "
            "or any workflow command. Check the transcript for an earlier "
            "failure (often an auth error)."
        )


# ---------------------------------------------------------------------------
# Workflow-level tests
# ---------------------------------------------------------------------------

class TestWorkflow:
    """Overall workflow integrity checks."""

    def test_project_initialized(self):
        """spektacular init was run — config.yaml exists."""
        assert (SPEK_DIR / "config.yaml").exists(), "config.yaml missing — init not run"

    def test_spec_file_exists(self):
        """The spec markdown file was created."""
        path = spec_file_path()
        assert path.exists(), f"Spec file not found at {path}"

    def test_workflow_reached_finished(self):
        """Workflow current_step is finished or done."""
        state = load_state()
        assert state["current_step"] in ("finished", "done"), (
            f"Workflow did not finish, stuck at: {state['current_step']}"
        )

    def test_all_steps_completed(self):
        """Every expected step appears in completed_steps."""
        state = load_state()
        completed = set(state.get("completed_steps", []))
        missing = [s for s in EXPECTED_STEP_ORDER if s not in completed]
        assert not missing, f"Steps not completed: {missing}"

    def test_steps_executed_in_order(self):
        """completed_steps matches the order defined by the state machine."""
        state = load_state()
        completed = state.get("completed_steps", [])
        assert completed == EXPECTED_STEP_ORDER, (
            f"Steps out of order.\n"
            f"  Expected: {EXPECTED_STEP_ORDER}\n"
            f"  Got:      {completed}"
        )

    def test_no_placeholder_text(self):
        """Spec contains no TODO/TBD/FIXME placeholder markers."""
        content = load_spec()
        placeholder_markers = ["TODO", "TBD", "FIXME", "placeholder", "[insert"]
        found = [m for m in placeholder_markers if m in content]
        assert not found, f"Found placeholder markers in spec: {found}"


# ---------------------------------------------------------------------------
# Per-step behaviour tests
# ---------------------------------------------------------------------------

class TestNewStep:
    """The agent called spektacular spec new to create the spec."""

    def test_step_completed(self):
        state = load_state()
        assert "new" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec new" in calls, (
            f"Agent did not call 'spektacular spec new'. Calls: {calls}"
        )


class TestInterviewStep:
    """The agent completed the interview step before drafting any section."""

    def test_step_completed(self):
        state = load_state()
        assert "interview" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto interview" in calls, (
            f"Agent did not call 'spektacular spec goto' with step 'interview'. Calls: {calls}"
        )


class TestOverviewStep:
    """The agent completed the overview step with meaningful content."""

    def test_step_completed(self):
        state = load_state()
        assert "overview" in state.get("completed_steps", [])

    def test_tool_called(self):
        """Overview is reached after the interview step — verify it was called."""
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto interview" in calls, (
            f"Agent did not call 'spektacular spec goto' with step 'interview' (which transitions to overview). Calls: {calls}"
        )

    def test_section_has_content(self):
        sections = parse_sections(load_spec())
        assert "overview" in sections, "Overview section not found in spec"
        assert len(sections["overview"]) >= MIN_SECTION_LENGTH, (
            f"Overview too short ({len(sections['overview'])} chars)"
        )

    def test_content_is_relevant(self):
        sections = parse_sections(load_spec())
        content = sections.get("overview", "").lower()
        assert any(term in content for term in ["jwt", "token", "auth"]), (
            "Overview does not mention JWT, token, or auth"
        )


class TestRequirementsStep:
    """The agent completed the requirements step with meaningful content."""

    def test_step_completed(self):
        state = load_state()
        assert "requirements" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto requirements" in calls, (
            f"Agent did not call 'spektacular spec goto' for requirements. Calls: {calls}"
        )

    def test_section_has_content(self):
        sections = parse_sections(load_spec())
        assert "requirements" in sections, "Requirements section not found in spec"
        assert len(sections["requirements"]) >= MIN_SECTION_LENGTH, (
            f"Requirements too short ({len(sections['requirements'])} chars)"
        )

    def test_has_multiple_requirements(self):
        sections = parse_sections(load_spec())
        content = sections.get("requirements", "")
        items = re.findall(r"^[-*]\s|\[.\]", content, re.MULTILINE)
        assert len(items) >= 3, (
            f"Expected at least 3 requirement items, found {len(items)}"
        )


class TestAcceptanceCriteriaStep:
    """The agent completed the acceptance criteria step with meaningful content."""

    def test_step_completed(self):
        state = load_state()
        assert "acceptance_criteria" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto acceptance_criteria" in calls, (
            f"Agent did not call 'spektacular spec goto' for acceptance_criteria. Calls: {calls}"
        )

    def test_section_has_content(self):
        sections = parse_sections(load_spec())
        assert "acceptance criteria" in sections, "Acceptance Criteria section not found"
        assert len(sections["acceptance criteria"]) >= MIN_SECTION_LENGTH, (
            f"Acceptance Criteria too short ({len(sections['acceptance criteria'])} chars)"
        )

    def test_has_verifiable_criteria(self):
        sections = parse_sections(load_spec())
        content = sections.get("acceptance criteria", "")
        items = re.findall(r"^[-*]\s|\[.\]", content, re.MULTILINE)
        assert len(items) >= 3, (
            f"Expected at least 3 acceptance criteria, found {len(items)}"
        )


class TestConstraintsStep:
    """The agent completed the constraints step with meaningful content."""

    def test_step_completed(self):
        state = load_state()
        assert "constraints" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto constraints" in calls, (
            f"Agent did not call 'spektacular spec goto' for constraints. Calls: {calls}"
        )

    def test_section_has_content(self):
        sections = parse_sections(load_spec())
        assert "constraints" in sections, "Constraints section not found"
        assert len(sections["constraints"]) >= MIN_SECTION_LENGTH, (
            f"Constraints too short ({len(sections['constraints'])} chars)"
        )


class TestCrossSectionAmendment:
    """A scripted rejection during Constraints surfaces a Requirements gap
    (token revocation) that the agent must fold into the already-confirmed
    Requirements section — without the user re-visiting or re-confirming
    Requirements in the moment. This is the automated proof that
    cross-section amendment actually fires, not just that it is described in
    the templates. See instruction.md's "A rejection that reveals a missing
    requirement" section for the scripted scenario.
    """

    def test_requirements_mentions_revocation(self):
        """The final spec's Requirements section reflects the correction
        surfaced while reviewing Constraints, even though revocation was
        never mentioned when Requirements was originally drafted."""
        sections = parse_sections(load_spec())
        content = sections.get("requirements", "").lower()
        # Match the word stem, not one inflection: the agent may write
        # "revocation", "revoke", "revoked" or "revoking" and all satisfy the
        # scenario equally. A needle of "revoc" alone silently fails a run whose
        # requirement is titled "revoke refresh tokens", which is a correct
        # outcome.
        assert re.search(r"revo[ck]", content), (
            "Requirements does not mention token revocation — the "
            "cross-section amendment triggered by the scripted Constraints "
            "rejection did not land in the Requirements working file. "
            f"Requirements content: {sections.get('requirements', '')!r}"
        )

    def test_requirements_not_reconfirmed_after_constraints(self):
        """The agent must not call 'spec goto requirements' a second time —
        the amendment updates the working file directly, with no fresh
        confirmation gate reopening the Requirements step itself."""
        calls = find_spektacular_calls(extract_tool_calls())
        requirements_gotos = [c for c in calls if c == "spec goto requirements"]
        assert len(requirements_gotos) <= 1, (
            "Agent re-entered the requirements step after amending it "
            f"cross-section — expected at most one 'spec goto requirements' "
            f"call, found {len(requirements_gotos)}. Calls: {calls}"
        )


class TestTechnicalApproachStep:
    """The agent completed the technical approach step with meaningful content."""

    def test_step_completed(self):
        state = load_state()
        assert "technical_approach" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto technical_approach" in calls, (
            f"Agent did not call 'spektacular spec goto' for technical_approach. Calls: {calls}"
        )

    def test_section_has_content(self):
        sections = parse_sections(load_spec())
        assert "technical approach" in sections, "Technical Approach section not found"
        assert len(sections["technical approach"]) >= MIN_SECTION_LENGTH, (
            f"Technical Approach too short ({len(sections['technical approach'])} chars)"
        )

    def test_content_is_technical(self):
        sections = parse_sections(load_spec())
        content = sections.get("technical approach", "").lower()
        assert any(term in content for term in [
            "api", "endpoint", "database", "service", "key", "token", "jwt",
            "architecture", "migration", "schema",
        ]), "Technical Approach lacks technical content"


class TestSuccessMetricsStep:
    """The agent completed the success metrics step with meaningful content."""

    def test_step_completed(self):
        state = load_state()
        assert "success_metrics" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto success_metrics" in calls, (
            f"Agent did not call 'spektacular spec goto' for success_metrics. Calls: {calls}"
        )

    def test_section_has_content(self):
        sections = parse_sections(load_spec())
        assert "success metrics" in sections, "Success Metrics section not found"
        assert len(sections["success metrics"]) >= MIN_SECTION_LENGTH, (
            f"Success Metrics too short ({len(sections['success metrics'])} chars)"
        )


class TestNonGoalsStep:
    """The agent completed the non-goals step with meaningful content."""

    def test_step_completed(self):
        state = load_state()
        assert "non_goals" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto non_goals" in calls, (
            f"Agent did not call 'spektacular spec goto' for non_goals. Calls: {calls}"
        )

    def test_section_has_content(self):
        sections = parse_sections(load_spec())
        assert "non-goals" in sections, "Non-Goals section not found"
        assert len(sections["non-goals"]) >= MIN_SECTION_LENGTH, (
            f"Non-Goals too short ({len(sections['non-goals'])} chars)"
        )


class TestVerificationStep:
    """The agent completed the verification step."""

    def test_step_completed(self):
        state = load_state()
        assert "verification" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto verification" in calls, (
            f"Agent did not call 'spektacular spec goto' for verification. Calls: {calls}"
        )


class TestSplitStep:
    """The agent ran the split check on the complete spec.

    The scenario is one coupled feature, so the check must not lead to a
    split: the workflow passes through ``split`` and finishes with no epic
    stored.
    """

    def test_step_completed(self):
        state = load_state()
        assert "split" in state.get("completed_steps", [])

    def test_tool_called(self):
        calls = find_spektacular_calls(extract_tool_calls())
        assert "spec goto split" in calls, (
            f"Agent did not call 'spektacular spec goto' for split. Calls: {calls}"
        )

    def test_single_feature_writes_no_epic(self):
        epics_dir = SPEK_DIR / "epics"
        stored = sorted(p.name for p in epics_dir.glob("*.md")) if epics_dir.exists() else []
        assert not stored, (
            f"A single coupled feature must finish without an epic, found: {stored}"
        )


class TestSpecFileCommands:
    """The spec file was written through the spektacular CLI, not built-in tools.

    Spektacular owns the spec file. The completed spec must be committed with
    `spektacular spec file write`, which routes the write through the CLI into
    the configured spec directory. The agent must never create or edit a file
    in the spec directory with the built-in Write/Edit tools — those bypass the
    CLI entirely.
    """

    def test_spec_file_write_command_used(self):
        """The agent committed the spec via `spektacular spec file write`."""
        commands = [c["command"] for c in extract_tool_calls()]
        used = [c for c in commands if "spektacular spec file write" in c]
        assert used, (
            "Agent never ran 'spektacular spec file write' — the completed "
            "spec must be committed to the store through the CLI rather than "
            "written to disk directly."
        )

    def test_spec_file_not_written_with_builtin_tools(self):
        """No Write/Edit tool call targeted a file in the spec directory.

        The scratch file used to stage the spec (.spektacular/tmp/...) is fine;
        only writes into the spec directory itself are a violation.
        """
        spec_name = spec_file_path().name
        offending = [
            e for e in extract_builtin_file_edits()
            if ".spektacular/specs/" in e["file_path"]
            or e["file_path"].endswith(f"/{spec_name}")
            or e["file_path"] == spec_name
        ]
        assert not offending, (
            "Agent used built-in file tools on the spec file instead of "
            f"'spektacular spec file write': {offending}"
        )
