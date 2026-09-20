"""Verify that the spektacular plan workflow completed successfully.

Per-step pass/fail reporting for the full plan workflow, driven by five
layers of assertions:

  1. Workflow integrity — state reached finished, completed_steps matches
     the hand-maintained EXPECTED_STEP_ORDER, no placeholder markers in
     any artefact.
  2. Plan CLI invocation — for every step, the agent invoked the matching
     `plan new` / `plan goto` Bash call during that step's window.
  3. Skill retrieval and sub-agent spawning — for every (step, skill) pair
     in EXPECTED_SKILLS_PER_STEP, the agent retrieved that skill during
     that step's window; for every step in EXPECTED_SPAWN_STEPS, the agent
     spawned at least one sub-agent during that step's window.
  4. Artefact content and instruction `next_step` validity — plan.md,
     context.md, research.md exist with substantive on-topic content, and
     every rendered instruction's next_step directive references a valid
     state-machine step.
  5. Plan-file CLI usage — the plan documents were committed with
     `spektacular plan file write` and never written or edited with the
     built-in Write/Edit tools.
  6. Convention-aware planning — the discovery step read the seeded
     convention through `spektacular knowledge always-applied`
     (CONVENTIONS_READ_COMMAND) within its window, and plan.md's
     ## Conventions section reflects the seeded convention's distinctive
     token (SEEDED_CONVENTION_TOKEN).

All expectation maps (step order, skills per step, sub-agent spawn steps,
expected plan sections, and the convention oracles CONVENTIONS_READ_COMMAND /
SEEDED_CONVENTION_TOKEN) are hand-maintained in this module — they are the
independent behavioural oracle. Do NOT derive them from `spektacular plan
steps`, from the `templates/steps/plan/*.md` files, or from the seeded
convention file at runtime: that would make the test tautological (asserting
"the agent did what the state machine / templates told it to do" is a closed
loop, not a behavioural check).

When a legitimate change lands in `internal/steps/plan/steps.go` Steps() or
in `templates/steps/plan/*.md`, update the corresponding map below in the
same commit.
"""

import json
import re
from dataclasses import dataclass
from pathlib import Path

import pytest

# ---------------------------------------------------------------------------
# Paths and constants
# ---------------------------------------------------------------------------

PROJECT_DIR = Path("/app")
SPEK_DIR = PROJECT_DIR / ".spektacular"
TRANSCRIPT = Path("/logs/agent/claude-code.txt")

# Hand-maintained canonical step order — the behavioural oracle for
# state-machine correctness. When a legitimate state-machine change lands
# (add/rename/reorder a step), update this list AND
# internal/steps/plan/steps.go Steps() in the same commit.
EXPECTED_STEP_ORDER = [
    "new",
    "overview",
    "discovery",
    "architecture",
    "components",
    "data_structures",
    "implementation_detail",
    "dependencies",
    "testing_approach",
    "milestones",
    "phases",
    "open_questions",
    "out_of_scope",
    "assemble",
    "verification",
    "write_plan",
    "write_context",
    "write_research",
    "walkthrough",
    "finished",
]

# Hand-maintained skill retrievals the agent is expected to make during
# each step. Independent oracle — when a template under
# templates/steps/plan/ changes its skill references, this map must be
# updated in the same commit.
EXPECTED_SKILLS_PER_STEP = {
    "discovery": frozenset({"spawn-planning-agents"}),
    "phases": frozenset({"spawn-implementation-agents"}),
    "assemble": frozenset(
        {
            "gather-project-metadata",
            "determine-feature-slug",
        }
    ),
}

# Hand-maintained set of steps whose templates direct the agent to spawn
# sub-agents. Independent oracle — update when template language changes.
EXPECTED_SPAWN_STEPS = frozenset({"discovery"})

MIN_SECTION_LENGTH = 100

# Hand-maintained oracle for convention-aware planning. The environment
# (environment/Dockerfile + environment/auth-audit-logging.md) seeds a single
# distinctive always-apply convention under .spektacular/knowledge/conventions/.
# CONVENTIONS_READ_COMMAND is the CLI substring the discovery step must invoke
# to read it; SEEDED_CONVENTION_TOKEN is a distinctive anchor unlikely to
# appear incidentally in a JWT plan, so finding it in plan.md's ## Conventions
# section proves the seeded convention was digested into the plan rather than a
# command merely having run. Update both here in the same commit if the seeded
# convention or the discovery instruction changes.
CONVENTIONS_READ_COMMAND = "knowledge always-applied"
SEEDED_CONVENTION_TOKEN = "AUTH_AUDIT_V2"

# Hand-maintained oracle for the design-reference obligation. The discovery
# step tells the agent, unconditionally, to ask what design documents the spec
# references before designing anything; DESIGN_REF_LIST_COMMAND is the CLI
# substring that obligation must produce inside the discovery window.
#
# There is deliberately no content half to pair with this, unlike the
# convention oracle above. This environment seeds no design source and the spec
# carries no design references, so the command returns an empty list and there
# is nothing for the plan to digest. That is the realistic shape for a project
# that does not use design documents, and it is the case worth protecting: the
# obligation must fire and degrade cleanly, not be skipped because it happens to
# have nothing to report. Update this literal in the same commit as any change
# to the discovery step's design instruction.
DESIGN_REF_LIST_COMMAND = "design ref list"

# Hand-maintained oracle for the assumption log folded into research.md's
# ## Drafting assumptions section. Each recorded judgement call is a `### `
# entry carrying all three bullets below; a run that genuinely made no
# judgement calls records the exact fallback line instead. These literals
# mirror the write_research template's contract — when that template's
# assumption-log format changes, update these in the same commit. Never
# derive them from the template at runtime.
ASSUMPTION_ENTRY_MARKERS = ("**Decision**", "**Rationale**", "**Rejected**")
NO_ASSUMPTIONS_FALLBACK = "No drafting assumptions were recorded."

# Hand-maintained oracle for the artefact status lifecycle. Committed plan
# documents carry `document_status: draft` YAML frontmatter until the finished
# step stamps them `document_status: final` after walkthrough sign-off. Update
# these literals in the same commit as any frontmatter schema change.
#
# The key is `document_status` and the vocabulary is draft/final/superseded/
# archived. The earlier `status: in-progress` / `status: completed` shape these
# literals carried was renamed by the document-status-vocabulary work and this
# oracle was not updated with it, which is exactly the drift this file's
# hand-maintained expectations are prone to: nothing fails at rename time, the
# next person to run the suite pays for it.
STATUS_DRAFT = "draft"
STATUS_FINAL = "final"
STATUS_KEY = "document_status"

# Built-in agent tools that mutate files directly, bypassing the spektacular
# CLI. The plan documents must never be written or edited with these.
BUILTIN_FILE_TOOLS = frozenset({"Write", "Edit", "MultiEdit", "NotebookEdit"})

# Hand-maintained list of scaffold slot strings that must be replaced when
# the agent fills the scaffolds. An occurrence of any of these substrings in
# a written artefact means the agent left a slot unfilled. These are literal
# strings copied from templates/scaffold/{plan,context,research}.md — when
# the scaffolds change, this list must be updated in the same commit.
#
# Matching is case-sensitive and substring-based, which is fine because
# these are distinctive angle-bracketed slot labels that don't appear in
# legitimate prose.
SCAFFOLD_LEFTOVERS = (
    # plan.md scaffold
    "<user-facing title>",
    "<short title>",
    "<one-paragraph description",
    "<outcome statement in plain language>",
    # context.md scaffold
    "<title matching plan.md>",
    "<file:line>",
    "<description of change>",
    "<approach>",
    # research.md scaffold
    "### Option A: <name>",
    "<reason with citation>",
    "<path:line>",
    "<one-line summary of what was learned>",
)

# Section headings (lowercase) expected to appear in plan.md after the
# verification step writes it.
EXPECTED_PLAN_SECTIONS = (
    "overview",
    "conventions",
    "architecture & design decisions",
    "component breakdown",
    "data structures & interfaces",
    "implementation detail",
    "dependencies",
    "testing approach",
    "milestones & phases",
    "open questions",
    "out of scope",
)

PLAN_GOTO_STEP_RE = re.compile(r'"step"\s*:\s*"([a-z_]+)"')
INSTRUCTION_NEXT_STEP_RE = re.compile(
    r"plan\s+goto\s+--data\s+'\{\"step\":\"([a-z_]+)\"\}'"
)


# ---------------------------------------------------------------------------
# Data classes
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ToolCall:
    index: int
    type: str  # "Bash" | "Skill" | "Task" | "Agent" | "AskUserQuestion"
    name: str
    input: dict
    tool_use_id: str


@dataclass(frozen=True)
class StepWindow:
    step: str
    start: int  # inclusive
    end: int  # exclusive


# ---------------------------------------------------------------------------
# State / artefact helpers
# ---------------------------------------------------------------------------


def find_plan_state_file() -> Path:
    """Locate the unified .spektacular/state.json."""
    state_file = SPEK_DIR / "state.json"
    assert state_file.exists(), f"No state file found at {state_file}"
    return state_file


def find_plan_name() -> str:
    state = load_state()
    name = (state.get("data") or {}).get("name")
    assert name, "state.json has no data.name — cannot resolve plan name"
    return name


def load_state() -> dict:
    return json.loads(find_plan_state_file().read_text())


def plan_artefact_paths() -> tuple:
    name = find_plan_name()
    base = SPEK_DIR / "plans" / name
    return base / "plan.md", base / "context.md", base / "research.md"


def parse_sections(text: str) -> dict:
    """Split markdown into sections keyed by lowercase heading.

    HTML comments are stripped so they do not count toward content length.
    `##` headings become section keys.
    """
    cleaned = re.sub(r"<!--.*?-->", "", text, flags=re.DOTALL)
    sections: dict = {}
    current = None
    lines: list = []
    for line in cleaned.splitlines():
        m = re.match(r"^#{2}\s+(.+)", line)
        if m:
            if current is not None:
                sections[current] = "\n".join(lines).strip()
            current = m.group(1).strip().lower()
            lines = []
        elif current is not None:
            lines.append(line)
    if current is not None:
        sections[current] = "\n".join(lines).strip()
    return sections


FRONTMATTER_RE = re.compile(r"\A---\n(.*?)\n---[ \t]*\n", re.DOTALL)
FRONTMATTER_LINE_RE = re.compile(r"^([A-Za-z0-9_-]+):\s*(.*)$")


def parse_frontmatter(text: str) -> dict:
    """Parse a leading `---` YAML frontmatter block into a flat dict.

    Hand-rolled on purpose — the committed artefacts only use simple
    `key: value` / `key: "value"` scalar lines, so a yaml dependency would
    be overkill for this dependency-free module. Surrounding single or
    double quotes on values are stripped. Returns {} when the text has no
    leading frontmatter block.
    """
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


# ---------------------------------------------------------------------------
# Transcript extraction
# ---------------------------------------------------------------------------


def _iter_transcript_objects():
    assert TRANSCRIPT.exists(), f"Agent transcript not found at {TRANSCRIPT}"
    for line in TRANSCRIPT.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            yield json.loads(line)
        except json.JSONDecodeError:
            continue


def extract_tool_calls() -> list:
    """Return an ordered list of ToolCall entries from the transcript.

    Captures Bash, Skill, Task, Agent, and AskUserQuestion tool_use blocks.
    Preserves transcript order so step-window attribution works. Every
    consumer filters by call.type or by Bash command content, so the
    AskUserQuestion entries only surface in tests that ask for them.
    """
    calls: list = []
    for obj in _iter_transcript_objects():
        if obj.get("type") != "assistant":
            continue
        msg = obj.get("message", {})
        for block in msg.get("content", []):
            if block.get("type") != "tool_use":
                continue
            name = block.get("name", "")
            if name in ("Bash", "Skill", "Task", "Agent", "AskUserQuestion"):
                calls.append(
                    ToolCall(
                        index=len(calls),
                        type=name,
                        name=name,
                        input=block.get("input", {}) or {},
                        tool_use_id=block.get("id", "") or "",
                    )
                )
    return calls


def extract_builtin_file_edits() -> list:
    """Return every built-in file-mutation tool call (Write/Edit/...) in order.

    Each entry is {"name": <tool>, "file_path": <path>}. These are the tools
    the agent must NOT use on the plan documents — `extract_tool_calls` does
    not capture them, so this walks the transcript separately.
    """
    edits: list = []
    for obj in _iter_transcript_objects():
        if obj.get("type") != "assistant":
            continue
        for block in obj.get("message", {}).get("content", []):
            if block.get("type") != "tool_use":
                continue
            if block.get("name") not in BUILTIN_FILE_TOOLS:
                continue
            inp = block.get("input", {}) or {}
            path = inp.get("file_path") or inp.get("notebook_path") or ""
            edits.append({"name": block["name"], "file_path": path})
    return edits


def extract_tool_results() -> dict:
    """Return a map of tool_use_id → result text extracted from user turns."""
    results: dict = {}
    for obj in _iter_transcript_objects():
        if obj.get("type") != "user":
            continue
        msg = obj.get("message", {})
        for block in msg.get("content", []):
            if block.get("type") != "tool_result":
                continue
            tid = block.get("tool_use_id", "")
            if not tid:
                continue
            content = block.get("content", "")
            if isinstance(content, list):
                text = "".join(
                    (part.get("text", "") if isinstance(part, dict) else str(part))
                    for part in content
                )
            else:
                text = str(content)
            results[tid] = text
    return results


# ---------------------------------------------------------------------------
# Plan-CLI call detection and step windowing
# ---------------------------------------------------------------------------


def _bash_command(call: ToolCall) -> str:
    return call.input.get("command", "") if call.type == "Bash" else ""


def _is_plan_new_call(cmd: str) -> bool:
    return ("spektacular plan new" in cmd) or (
        "plan new --data" in cmd and "spektacular" in cmd
    )


def _is_plan_goto_call(cmd: str) -> bool:
    return "plan goto" in cmd and "spektacular" in cmd


def find_plan_cli_calls(calls: list) -> list:
    """Return an ordered list of step names touched by plan new / plan goto calls.

    `plan new` emits the implicit "new" step. `plan goto --data '{"step":"X"}'`
    emits "X".
    """
    results: list = []
    for call in calls:
        cmd = _bash_command(call)
        if not cmd:
            continue
        if _is_plan_new_call(cmd):
            results.append("new")
        elif _is_plan_goto_call(cmd):
            m = PLAN_GOTO_STEP_RE.search(cmd)
            if m:
                results.append(m.group(1))
    return results


def resolve_step_windows(calls: list) -> dict:
    """Build step-attribution windows over the tool-call list.

    Each transition call (`plan new` or `plan goto`) opens a window keyed by
    the step it transitions INTO. The window ends at the next transition
    call, or at len(calls) for the last one. The implicit `overview`
    transition inside `plan new` is given a shared window with `new` so
    assertions against `overview` still resolve.
    """
    transitions: list = []  # (call_index, step_name)
    for call in calls:
        cmd = _bash_command(call)
        if not cmd:
            continue
        if _is_plan_new_call(cmd):
            transitions.append((call.index, "new"))
        elif _is_plan_goto_call(cmd):
            m = PLAN_GOTO_STEP_RE.search(cmd)
            if m:
                transitions.append((call.index, m.group(1)))

    windows: dict = {}
    for i, (start_idx, step) in enumerate(transitions):
        end = transitions[i + 1][0] if i + 1 < len(transitions) else len(calls)
        windows[step] = StepWindow(step=step, start=start_idx, end=end)

    # `overview` is entered automatically by `plan new`. If it has no
    # explicit goto, share `new`'s window so expectations still attach.
    if "new" in windows and "overview" not in windows:
        nw = windows["new"]
        windows["overview"] = StepWindow(
            step="overview",
            start=nw.start,
            end=nw.end,
        )
    return windows


# ---------------------------------------------------------------------------
# Parameter expansion and caches
# ---------------------------------------------------------------------------

# Lazy-populated caches so transcript parsing happens once per test session.
_CACHE: dict = {}


def _skill_params() -> list:
    return sorted(
        (step, skill)
        for step, skills in EXPECTED_SKILLS_PER_STEP.items()
        for skill in skills
    )


def _spawn_params() -> list:
    return sorted(EXPECTED_SPAWN_STEPS)


def _calls_cache() -> list:
    if "calls" not in _CACHE:
        _CACHE["calls"] = extract_tool_calls()
    return _CACHE["calls"]


def _windows_cache() -> dict:
    if "windows" not in _CACHE:
        _CACHE["windows"] = resolve_step_windows(_calls_cache())
    return _CACHE["windows"]


def _results_cache() -> dict:
    if "results" not in _CACHE:
        _CACHE["results"] = extract_tool_results()
    return _CACHE["results"]


# ---------------------------------------------------------------------------
# Agent-execution preflight
# ---------------------------------------------------------------------------


def _agent_result_events() -> list:
    """Return all `result`-type events from the transcript, in order."""
    return [o for o in _iter_transcript_objects() if o.get("type") == "result"]


def _agent_auth_failure():
    """Return the first transcript object signalling an Anthropic API
    authentication failure (HTTP 401), or None if there is none."""
    for obj in _iter_transcript_objects():
        if obj.get("error") == "authentication_failed":
            return obj
        if obj.get("api_error_status") == 401:
            return obj
    return None


@pytest.fixture(scope="session", autouse=True)
def _abort_on_failed_agent_run():
    """Stop the whole session immediately when the agent never ran or failed to
    authenticate, instead of letting ~80 downstream tests fail with misleading
    errors.

    An auth failure (e.g. a missing or invalid ANTHROPIC_API_KEY) means no
    artefacts are ever produced, so every artefact/state/CLI assertion fails
    with a symptom ("config.yaml missing", "state.json missing", …) that buries
    the real cause. This autouse session fixture trips ONLY on the fatal
    preflight conditions — no transcript, an auth failure, or an agent run that
    did not finish — and calls pytest.exit so the run reports one clear reason
    and a non-zero status. A genuine workflow regression (the agent ran but did
    the wrong thing) does NOT trip this, so the full suite still reports every
    such failure.
    """
    if not TRANSCRIPT.exists():
        pytest.exit(
            f"Agent transcript not found at {TRANSCRIPT} — the agent never ran. "
            "Aborting before the misleading downstream cascade.",
            returncode=1,
        )
    failure = _agent_auth_failure()
    if failure is not None:
        pytest.exit(
            "Agent failed to authenticate with the Anthropic API "
            f"(api_error_status={failure.get('api_error_status')}): "
            f"{failure.get('result') or failure.get('error')!r}. "
            "Set a valid ANTHROPIC_API_KEY before running harbor. Aborting "
            "before the misleading downstream cascade.",
            returncode=1,
        )
    results = _agent_result_events()
    if not results or results[-1].get("is_error"):
        detail = results[-1].get("result") if results else "no result event"
        pytest.exit(
            f"Agent run did not finish successfully ({detail!r}). Aborting "
            "before the misleading downstream cascade.",
            returncode=1,
        )


# ---------------------------------------------------------------------------
# Agent-execution preflight tests — run first
# ---------------------------------------------------------------------------


class TestAgentExecution:
    """Preflight: the Claude Code agent authenticated and actually ran.

    Defined first so it runs before any workflow assertion. If the agent
    never started — e.g. an invalid ANTHROPIC_API_KEY — then config.yaml,
    state.json and every artefact will be missing, and the downstream
    failures are misleading ("config.yaml missing" when the real cause is a
    401). When this class fails, read it first: every other failure in the
    run is a consequence.
    """

    def test_transcript_exists(self):
        assert TRANSCRIPT.exists(), (
            f"Agent transcript not found at {TRANSCRIPT} — the agent never ran."
        )

    def test_agent_authenticated(self):
        failure = _agent_auth_failure()
        assert failure is None, (
            "Agent failed to authenticate with the Anthropic API "
            f"(api_error_status={failure.get('api_error_status')}): "
            f"{failure.get('result') or failure.get('error')!r}. "
            "Set a valid ANTHROPIC_API_KEY before running harbor — every "
            "other failure in this run is a consequence of this."
        )

    def test_agent_run_succeeded(self):
        results = _agent_result_events()
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
            "Agent produced no Bash/Skill/Task tool calls — it never ran "
            "`spektacular init` or any workflow command. Check the transcript "
            "for an earlier failure (often an auth error)."
        )


# ---------------------------------------------------------------------------
# Workflow-level tests
# ---------------------------------------------------------------------------


class TestWorkflow:
    """Overall workflow integrity checks."""

    def test_project_initialized(self):
        assert (SPEK_DIR / "config.yaml").exists(), (
            "config.yaml missing — `spektacular init` was not run"
        )

    def test_state_file_exists(self):
        find_plan_state_file()

    def test_workflow_reached_finished(self):
        state = load_state()
        assert state.get("current_step") == "finished", (
            f"Workflow did not finish; current_step={state.get('current_step')}"
        )

    def test_all_steps_completed(self):
        state = load_state()
        completed = set(state.get("completed_steps", []))
        missing = [s for s in EXPECTED_STEP_ORDER if s not in completed]
        assert not missing, f"Steps not completed: {missing}"

    def test_steps_executed_in_order(self):
        """completed_steps == EXPECTED_STEP_ORDER catches state-machine
        reordering because the agent follows the state machine's order, so
        any reorder in Steps() surfaces as a diff here.
        """
        state = load_state()
        completed = state.get("completed_steps", [])
        assert completed == EXPECTED_STEP_ORDER, (
            f"Steps out of order.\n"
            f"  Expected: {EXPECTED_STEP_ORDER}\n"
            f"  Got:      {completed}"
        )

    def test_no_unfilled_scaffold_slots(self):
        """Every scaffold slot must be replaced with real content.

        This catches partial fills where the agent left a `<placeholder>`
        span from the scaffold unchanged. It does NOT scan for words like
        "TODO" or "TBD" in prose — those are legitimate when discussing
        genuine implementation uncertainty.
        """
        offenders = []
        for path in plan_artefact_paths():
            if not path.exists():
                continue
            text = path.read_text()
            for slot in SCAFFOLD_LEFTOVERS:
                if slot in text:
                    offenders.append((path.name, slot))
        assert not offenders, (
            f"Unfilled scaffold slots in artefacts: {offenders}"
        )

    def test_artefact_files_exist(self):
        for path in plan_artefact_paths():
            assert path.exists(), f"Artefact file missing: {path}"


class TestPlanFileCommands:
    """The plan documents were written through the spektacular CLI, not built-in tools.

    Spektacular owns the plan documents. plan.md, context.md and research.md
    must each be committed with `spektacular plan file write`, which routes the
    write through the CLI into the configured plan directory. The agent must
    never create or edit a file in the plan directory with the built-in
    Write/Edit tools — those bypass the CLI entirely.
    """

    def test_plan_file_write_command_used(self):
        """The agent committed the plan documents via `spektacular plan file write`."""
        used = [
            _bash_command(c)
            for c in _calls_cache()
            if "spektacular plan file write" in _bash_command(c)
        ]
        assert used, (
            "Agent never ran 'spektacular plan file write' — the plan "
            "documents must be committed to the store through the CLI rather "
            "than written to disk directly."
        )

    def test_plan_documents_not_written_with_builtin_tools(self):
        """No Write/Edit tool call targeted a file in the plan directory.

        The scratch files used to stage documents (.spektacular/tmp/...) are
        fine; only writes into the plan directory itself are a violation.
        """
        offending = [
            e for e in extract_builtin_file_edits()
            if ".spektacular/plans/" in e["file_path"]
        ]
        assert not offending, (
            "Agent used built-in file tools on the plan documents instead of "
            f"'spektacular plan file write': {offending}"
        )


# ---------------------------------------------------------------------------
# Per-step CLI-call tests
# ---------------------------------------------------------------------------


class TestPlanCLICalls:
    """For every expected step, the agent invoked the matching plan CLI call."""

    @pytest.mark.parametrize("step", EXPECTED_STEP_ORDER)
    def test_step_completed(self, step):
        state = load_state()
        assert step in state.get("completed_steps", []), (
            f"Step '{step}' not in completed_steps"
        )

    @pytest.mark.parametrize("step", EXPECTED_STEP_ORDER)
    def test_cli_call_for_step(self, step):
        calls = _calls_cache()
        observed = find_plan_cli_calls(calls)
        if step == "overview":
            # Overview is reached implicitly by `plan new`; accept either
            # an explicit goto or the implicit `plan new` transition.
            assert "new" in observed or "overview" in observed, (
                f"Neither 'plan new' nor 'plan goto overview' observed; got: {observed}"
            )
        else:
            assert step in observed, (
                f"No plan CLI call observed for step '{step}'; got: {observed}"
            )


# ---------------------------------------------------------------------------
# Template-driven skill-retrieval tests
# ---------------------------------------------------------------------------


class TestSkillRetrieval:
    """Every skill a step template references is retrieved during that step."""

    @pytest.mark.parametrize("step,skill", _skill_params())
    def test_skill_retrieved_in_step(self, step, skill):
        windows = _windows_cache()
        calls = _calls_cache()
        window = windows.get(step)
        assert window is not None, (
            f"Step '{step}' was never entered — cannot verify skill '{skill}' retrieval"
        )
        window_calls = calls[window.start : window.end]
        retrieved = False
        for c in window_calls:
            if c.type == "Skill":
                skill_name = c.input.get("skill") or c.input.get("name") or ""
                if skill_name == skill:
                    retrieved = True
                    break
            elif c.type == "Bash":
                cmd = c.input.get("command", "")
                if re.search(rf"\bskill\s+{re.escape(skill)}\b", cmd):
                    retrieved = True
                    break
        assert retrieved, (
            f"Step '{step}' did not retrieve skill '{skill}' — checked "
            f"{len(window_calls)} tool calls in the step's window"
        )


# ---------------------------------------------------------------------------
# Template-driven sub-agent spawn tests
# ---------------------------------------------------------------------------


class TestSubAgentSpawning:
    """Steps whose templates expect sub-agent spawning actually spawn one."""

    @pytest.mark.parametrize("step", _spawn_params())
    def test_subagent_spawned_in_step(self, step):
        windows = _windows_cache()
        calls = _calls_cache()
        window = windows.get(step)
        assert window is not None, (
            f"Step '{step}' was never entered — cannot verify sub-agent spawning"
        )
        window_calls = calls[window.start : window.end]
        spawned = any(c.type in ("Task", "Agent") for c in window_calls)
        assert spawned, (
            f"Step '{step}' expected sub-agent spawning but found no "
            f"Task/Agent tool_use in its window "
            f"({len(window_calls)} tool calls)"
        )


# ---------------------------------------------------------------------------
# Autonomous drafting tests
# ---------------------------------------------------------------------------


class TestAutonomousDrafting:
    """The drafting segment (overview → write_research) ran autonomously.

    The reworked plan workflow drafts without pausing for user confirmation:
    judgement calls are recorded in the assumption log instead of being posed
    as questions, and the single legitimate user interaction is the
    walkthrough's sign-off. AskUserQuestion is the structured tool through
    which the agent poses a question to the user, so its presence in a
    drafting-step window is the behavioural signal that autonomy was broken.
    """

    def test_no_confirmation_questions_during_drafting(self):
        """No AskUserQuestion call falls inside any drafting-step window.

        Hand-maintained oracle: every step window except "walkthrough" and
        "finished" belongs to the drafting segment and must contain zero
        AskUserQuestion tool_use blocks. The walkthrough window is exempt
        because posing the sign-off question there is the step's whole job
        (and "finished" follows it). This checks only the structured
        question tool — free-prose questions in assistant text are
        deliberately out of scope, since assistant text is unconstrained
        and the automated run self-answers.
        """
        windows = _windows_cache()
        calls = _calls_cache()
        offenders = []
        for step, window in windows.items():
            if step in ("walkthrough", "finished"):
                continue
            for c in calls[window.start : window.end]:
                if c.type == "AskUserQuestion":
                    offenders.append((step, c.index))
        assert not offenders, (
            "AskUserQuestion posed during drafting steps (step, call index): "
            f"{offenders} — the drafting segment must run autonomously, "
            "recording judgement calls in the assumption log instead of "
            "asking the user."
        )

    def test_drafting_assumptions_populated(self):
        """research.md's ## Drafting assumptions section records the
        judgement calls made while drafting autonomously.

        The section must exist and be non-empty, and take exactly one of
        two shapes: at least one `### ` entry carrying all three
        hand-maintained bullets (**Decision** / **Rationale** / **Rejected**),
        or — only when genuinely no judgement calls were made — the exact
        fallback line. When entries exist the fallback line must be absent,
        so the section cannot claim emptiness while also holding entries.
        A typical JWT-plan run involves real judgement calls (algorithm,
        expiry, storage), so the populated shape is the expected outcome.
        """
        _, _, research_path = plan_artefact_paths()
        assert research_path.exists(), f"research.md missing at {research_path}"
        sections = parse_sections(research_path.read_text())
        assert "drafting assumptions" in sections, (
            "research.md has no ## Drafting assumptions section; "
            f"found sections: {sorted(sections.keys())}"
        )
        section = sections["drafting assumptions"]
        assert section.strip(), (
            "research.md's ## Drafting assumptions section is empty — it "
            "must hold recorded judgement calls or the explicit fallback "
            f"line {NO_ASSUMPTIONS_FALLBACK!r}."
        )
        entries = re.split(r"^###\s+", section, flags=re.MULTILINE)[1:]
        if entries:
            complete = [
                e
                for e in entries
                if all(marker in e for marker in ASSUMPTION_ENTRY_MARKERS)
            ]
            assert complete, (
                f"None of the {len(entries)} '### ' assumption entries "
                "carries all three required bullets "
                f"{ASSUMPTION_ENTRY_MARKERS}. Section content: "
                f"{section[:500]!r}"
            )
            assert NO_ASSUMPTIONS_FALLBACK not in section, (
                "## Drafting assumptions contains both '### ' entries and "
                f"the fallback line {NO_ASSUMPTIONS_FALLBACK!r} — the "
                "fallback is only valid when no judgement calls were made."
            )
        else:
            assert section.strip() == NO_ASSUMPTIONS_FALLBACK, (
                "## Drafting assumptions has no '### ' entries and is not "
                f"the exact fallback line {NO_ASSUMPTIONS_FALLBACK!r}. "
                f"Section content: {section[:500]!r}"
            )


# ---------------------------------------------------------------------------
# Convention-aware planning tests
# ---------------------------------------------------------------------------


class TestConventionAwarePlanning:
    """The discovery step reads the seeded convention and the finished plan's
    ## Conventions section reflects it.

    Two independent checks: (1) a behavioural check that the conventions CLI
    verb ran inside the discovery window, and (2) a content check that the
    seeded convention's distinctive token reached plan.md. Both oracles are
    hand-maintained (CONVENTIONS_READ_COMMAND, SEEDED_CONVENTION_TOKEN) — they
    are never derived from the templates or state machine at runtime.
    """

    def test_conventions_read_during_discovery(self):
        windows = _windows_cache()
        calls = _calls_cache()
        window = windows.get("discovery")
        assert window is not None, (
            "Discovery step was never entered — cannot verify conventions were read"
        )
        window_calls = calls[window.start : window.end]
        read = any(
            CONVENTIONS_READ_COMMAND in _bash_command(c) for c in window_calls
        )
        assert read, (
            "Discovery step did not run "
            f"'spektacular {CONVENTIONS_READ_COMMAND}' — checked "
            f"{len(window_calls)} tool calls in the step's window. The plan "
            "workflow must load all conventions in full during discovery."
        )

    def test_design_references_resolved_during_discovery(self):
        """Discovery asks what designs the spec references, before designing.

        A behavioural check only: with no design source seeded the command
        returns an empty list, so there is no content half to assert. What
        matters is that the obligation fired at all — an agent that skips it
        would plan a spec carrying references without ever reading them, which
        is the failure this instruction exists to prevent.
        """
        windows = _windows_cache()
        calls = _calls_cache()
        window = windows.get("discovery")
        assert window is not None, (
            "Discovery step was never entered — cannot verify design "
            "references were resolved"
        )
        window_calls = calls[window.start : window.end]
        ran = any(
            DESIGN_REF_LIST_COMMAND in _bash_command(c) for c in window_calls
        )
        assert ran, (
            "Discovery step did not run "
            f"'spektacular {DESIGN_REF_LIST_COMMAND}' — checked "
            f"{len(window_calls)} tool calls in the step's window. The plan "
            "workflow must resolve every design reference a spec carries "
            "before any design work begins."
        )

    def test_conventions_section_reflects_seeded_convention(self):
        sections = _plan_sections()
        content = sections.get("conventions", "")
        assert SEEDED_CONVENTION_TOKEN in content, (
            f"plan.md's ## Conventions section does not mention "
            f"'{SEEDED_CONVENTION_TOKEN}', the distinctive token from the "
            "seeded auth convention. The relevant convention was not digested "
            f"into the plan. Section content: {content[:300]!r}"
        )


# ---------------------------------------------------------------------------
# Artefact content tests
# ---------------------------------------------------------------------------


def _plan_sections():
    plan_path, _, _ = plan_artefact_paths()
    if not plan_path.exists():
        return {}
    return parse_sections(plan_path.read_text())


class TestPlanMdContent:
    """plan.md contains every expected section with substantive content."""

    @pytest.mark.parametrize("section", EXPECTED_PLAN_SECTIONS)
    def test_section_present(self, section):
        sections = _plan_sections()
        assert section in sections, (
            f"Section '{section}' missing from plan.md; "
            f"found sections: {sorted(sections.keys())}"
        )

    @pytest.mark.parametrize("section", EXPECTED_PLAN_SECTIONS)
    def test_section_has_content(self, section):
        sections = _plan_sections()
        content = sections.get(section, "")
        assert len(content) >= MIN_SECTION_LENGTH, (
            f"Section '{section}' too short ({len(content)} chars, "
            f"need ≥ {MIN_SECTION_LENGTH})"
        )


class TestContextAndResearch:
    """context.md and research.md exist with substantive content."""

    def test_context_md_has_content(self):
        _, context_path, _ = plan_artefact_paths()
        assert context_path.exists(), f"context.md missing at {context_path}"
        content = context_path.read_text()
        # Arbitrary but nontrivial floor: at least 500 chars of real content.
        cleaned = re.sub(r"<!--.*?-->", "", content, flags=re.DOTALL)
        assert len(cleaned.strip()) >= 500, (
            f"context.md too short ({len(cleaned.strip())} chars)"
        )

    def test_research_md_has_content(self):
        _, _, research_path = plan_artefact_paths()
        assert research_path.exists(), f"research.md missing at {research_path}"
        content = research_path.read_text()
        cleaned = re.sub(r"<!--.*?-->", "", content, flags=re.DOTALL)
        assert len(cleaned.strip()) >= 500, (
            f"research.md too short ({len(cleaned.strip())} chars)"
        )

    def test_research_names_alternatives(self):
        """The research log should enumerate at least one rejected option."""
        _, _, research_path = plan_artefact_paths()
        if not research_path.exists():
            pytest.skip("research.md missing")
        text = research_path.read_text().lower()
        assert "reject" in text or "alternative" in text, (
            "research.md does not mention 'rejected' or 'alternative' "
            "— the decision log is likely incomplete"
        )


# ---------------------------------------------------------------------------
# Artefact status-lifecycle tests
# ---------------------------------------------------------------------------

# Matches a `document_status:` frontmatter line inside a tool-result text.
# Tolerates both raw markdown (`document_status: draft` /
# `document_status: "draft"`) and JSON-escaped embedding
# (`document_status: \"draft\"`) since plan CLI output may wrap the document
# in a JSON payload.
RESULT_STATUS_RE = re.compile(r"document_status:\s*\\?[\"']?(draft|final|superseded|archived)")


class TestArtefactStatusLifecycle:
    """Committed plan documents move draft → final at the right time.

    Docs are committed with `document_status: draft` frontmatter and stamped
    `document_status: final` only by the finished step, after walkthrough
    sign-off. Two checks: (a) the final on-disk artefacts are final, and
    (b) during the walkthrough window the docs still read as draft —
    proving the final stamp gated on sign-off rather than being applied at
    commit time.
    """

    def test_artefacts_final_after_finished(self):
        """Each committed artefact ends with frontmatter document_status: final."""
        for path in plan_artefact_paths():
            assert path.exists(), f"Artefact file missing: {path}"
            fm = parse_frontmatter(path.read_text())
            assert fm.get(STATUS_KEY) == STATUS_FINAL, (
                f"{path.name} frontmatter {STATUS_KEY} is {fm.get(STATUS_KEY)!r}, "
                f"expected {STATUS_FINAL!r} — the finished step must "
                "stamp every committed document final."
            )

    def test_docs_draft_during_walkthrough(self):
        """Docs read during the walkthrough still carry document_status: draft.

        The walkthrough template instructs the agent to read the committed
        documents back with `plan file read`, so the walkthrough window
        reliably contains such reads and their results include the
        frontmatter. Every status observed there must be draft: a final
        status during the walkthrough would mean the documents were
        stamped before sign-off, i.e. the gating is broken.
        """
        windows = _windows_cache()
        calls = _calls_cache()
        results = _results_cache()
        window = windows.get("walkthrough")
        assert window is not None, (
            "Walkthrough step was never entered — cannot verify the "
            "draft status of the documents under review."
        )
        observed = []  # (call index, status value)
        for c in calls[window.start : window.end]:
            cmd = _bash_command(c)
            if "plan file read" not in cmd:
                continue
            for status in RESULT_STATUS_RE.findall(
                results.get(c.tool_use_id, "")
            ):
                observed.append((c.index, status))
        assert observed, (
            "No `plan file read` result inside the walkthrough window "
            "contained a status: frontmatter line — the walkthrough must "
            "read the committed documents back for review."
        )
        offenders = [o for o in observed if o[1] != STATUS_DRAFT]
        assert not offenders, (
            "Documents read during the walkthrough already carried a "
            f"non-draft status (call index, status): {offenders} — "
            "the final stamp must wait for walkthrough sign-off."
        )


# ---------------------------------------------------------------------------
# Instruction next_step validity invariant
# ---------------------------------------------------------------------------


class TestInstructionNextStepValidity:
    """Every rendered instruction's next_step directive must reference a
    step name the state machine actually accepts.

    This is the formal guard for the discovery → approach drift class of
    bug. Each plan-CLI Bash call's tool_result is parsed as JSON and the
    `instruction` string is scanned for `plan goto --data '{"step":"X"}'`;
    X must appear in EXPECTED_STEP_ORDER.
    """

    def test_every_rendered_next_step_is_valid(self):
        calls = _calls_cache()
        results = _results_cache()
        valid = set(EXPECTED_STEP_ORDER)
        offenders = []
        for call in calls:
            if call.type != "Bash":
                continue
            cmd = _bash_command(call)
            if not (_is_plan_new_call(cmd) or _is_plan_goto_call(cmd)):
                continue
            result_text = results.get(call.tool_use_id, "")
            if not result_text:
                continue
            try:
                payload = json.loads(result_text)
            except json.JSONDecodeError:
                # Not all stdout lines are pure JSON (error paths, wrapped
                # harness output). Skip.
                continue
            instruction = (
                payload.get("instruction", "") if isinstance(payload, dict) else ""
            )
            if not instruction:
                continue
            m = INSTRUCTION_NEXT_STEP_RE.search(instruction)
            if not m:
                continue
            next_step = m.group(1)
            if next_step not in valid:
                offenders.append(
                    {
                        "from_step": payload.get("step"),
                        "rendered_next_step": next_step,
                    }
                )
        assert not offenders, (
            f"Rendered instructions reference invalid next steps: {offenders}"
        )

    def test_walkthrough_rendered_before_finished(self):
        """The `finished` instruction must only be rendered after a
        `walkthrough` instruction.

        Transcript-level guard for the mandatory plan-walkthrough review:
        the ordered sequence of `step` values from successful plan
        new/goto results must contain "walkthrough", and its first
        occurrence must precede the first occurrence of "finished". The
        step names here are hand-maintained oracle values, not derived
        from the state machine.
        """
        calls = _calls_cache()
        results = _results_cache()
        rendered_steps = []
        for call in calls:
            if call.type != "Bash":
                continue
            cmd = _bash_command(call)
            if not (_is_plan_new_call(cmd) or _is_plan_goto_call(cmd)):
                continue
            result_text = results.get(call.tool_use_id, "")
            if not result_text:
                continue
            try:
                payload = json.loads(result_text)
            except json.JSONDecodeError:
                # Not all stdout lines are pure JSON (error paths, wrapped
                # harness output). Skip.
                continue
            step = payload.get("step") if isinstance(payload, dict) else None
            if step:
                rendered_steps.append(step)
        assert "walkthrough" in rendered_steps, (
            "No walkthrough instruction was rendered "
            f"(rendered steps: {rendered_steps})"
        )
        assert "finished" in rendered_steps, (
            "No finished instruction was rendered "
            f"(rendered steps: {rendered_steps})"
        )
        assert rendered_steps.index("walkthrough") < rendered_steps.index(
            "finished"
        ), (
            "The finished instruction was rendered before any walkthrough "
            f"instruction (rendered steps: {rendered_steps})"
        )
