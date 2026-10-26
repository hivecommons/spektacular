---
created_date: "2026-10-24"
document_status: final
closed_date: "2026-10-24"
---

# Context: 000071_harbor-child-agents

## Current State Analysis

- **Run metrics (000070) record only child counts.** `tests/harbor/shared/metrics.py` writes `metrics.json` (`SCHEMA_VERSION = 1`, L46). Its `subagents` block holds started/completed/failed/stopped counts and a flat `durations_s` list (L58, L444). The record has no per-child identity, context, placement or hand-back, and no idle time. `tests/harbor/shared/transcripts.py:75-100` reads top-level launches only, and never reads `description`, `run_in_background`, timestamps or the `task_started`/`task_updated` events. `report.py` prints one "Sub-agents:" line (L303-309).
- **Claude's log already carries everything the spec asks for** (verified on the 2026-10-23 runs): launch input, `task_started` (background, depth, parent), `task_updated` (end time), `task_notification` (status, the agent's own duration, the hand-back), child messages inline with `parent_tool_use_id`, turn `result` events with `origin`, and `ScheduleWakeup` calls. Bob's trajectory carries launch kind, description and prompt only. Codex carries no child signal.
- **Delegation is never checked.**
  - The spec suite only checks that `verification` completed (`test_spec_workflow.py:559-570`).
  - The plan suite *fails* runs whose discovery used no child (`test_plan_workflow.py:108,936-953`), against the spec's constraint.
  - The goal-check suite checks the reviewer's input and model, but not the test and verify children on its retry.
  - The epic suites read no transcript at all.
  - On 2026-10-23, Sonnet 5.5 skipped the spec review child. Both 4.6 and 5.5 skipped the implement `test` and `verify` children in every implement-epic lane.
- **Two checks misread command text.**
  - The plan walkthrough check needs each goto's tool-result JSON (`test_plan_workflow.py:1322-1342,1394-1438`), so piping or chaining hides it.
  - The implement changelog checks match exact substrings (`test_implement_workflow.py:98-104,332-370`), so line continuations, reordered flags or loops miss.
  - The session log (`internal/sessionlog/record.go:18-56`) records every call's argv, exit code and resulting step. Both s55 reference runs did the right thing there.
- **The headless trigger.** The epic suites' instructions say "This session is headless" (`epic-workflow/instruction.md:53`, `implement-epic-workflow/instruction.md:54`). That activates the epic skills' rule to hold children in the foreground, which rests on the unverified claim that "headless Claude Code stops one after ten minutes" (`spek-plan-epic/SKILL.md:46`, `spek-implement-epic/SKILL.md:58`; pinned by `templates/plan_epic_skill_test.go:113-114,151`, `templates/implement_epic_skill_test.go:134-135,208`). Sonnet 5.5 started background children anyway and scheduled a 1200 s `ScheduleWakeup`, which kept the session alive about 16 minutes after its work was done.
- **The implement suite is seeded past `test`, `verify` and `check_goal`** (`implement-workflow/environment/state.json`). The goal-check suite is seeded at `check_goal`, and its unmet scenario retries `implement → test → verify → check_goal`.

## Per-Task Technical Notes

### Requirement-to-repo-and-files resolution

Every requirement lands in the `spektacular` repo (root `/home/nicj/code/github.com/hivecommons/spektacular`). The `docs` repo is untouched (the website docs are a non-goal).

| Requirement | Files |
|---|---|
| Every child agent is recorded; what each child was asked; how each child ran and ended | `tests/harbor/shared/transcripts.py`, `tests/harbor/shared/metrics.py` |
| Children placed in the workflow; suggested delegation is recorded | `tests/harbor/shared/metrics.py` (placement, per-step counts) |
| Children shown in the report | `tests/harbor/shared/report.py` |
| Unknown, not missing, for other agents | `tests/harbor/shared/transcripts.py`, `tests/harbor/shared/metrics.py` |
| Required delegation is enforced; children's context is checked | `tests/harbor/shared/delegation.py` (new); suite verifiers `spec-workflow`, `goal-check-workflow`, `epic-workflow`, `implement-epic-workflow`; `templates/steps/spec/08-verification.md`, `templates/steps/implement/04-test.md`, `05-verify.md`; `templates/skills/workflows/spek-implement-epic/SKILL.md` |
| The headless background-child limit is settled | `tests/harbor/background-child/` (new), `Makefile`, both epic `SKILL.md` |
| Epic suites run children as a real session does; being told no one answers does not force foreground | both epic `SKILL.md`, both epic verifiers and `instruction.md` |
| No timed wake-ups while waiting on children | both epic `SKILL.md`; `tests/harbor/shared/delegation.py`; both epic verifiers |
| Idle time is measured and flagged | `tests/harbor/shared/metrics.py`, `tests/harbor/shared/report.py` |
| Walkthrough check reads every workflow advance | `tests/harbor/shared/sessionlog.py`, `tests/harbor/plan-workflow/tests/test_plan_workflow.py` |
| Per-repo changelog check reads every write | `tests/harbor/shared/sessionlog.py`, `tests/harbor/implement-workflow/tests/test_implement_workflow.py` |
| Constraint: every suite says headless / no one answers | `tests/harbor/plan-workflow/instruction.md`, `tests/harbor/implement-workflow/instruction.md` |

Shared self-checks run with `uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests` (README L593). Go contract tests run with `go test ./...`. Files under `tests/harbor/jobs/` are gitignored and partly root-owned. Read only `agent/claude-code.txt`, `artifacts/spektacular/debug/*.jsonl` and `verifier/*`, never `agent/sessions/`.

### Task: Read every child agent from each agent log

**File changes**:
- `tests/harbor/shared/transcripts.py:35-46`: keep `SUBAGENT_TOOLS` and `SubagentLaunch` unchanged, because `test_transcripts_summary.py:429-447` pins them by equality and `goal-check-workflow/tests/test_verifier_selfcheck.py:148-153` builds them positionally. After the dataclasses at L162-180, add `Child` (fields as in plan.md § Data Structures), `Turn(ended_at, woken_by)` and `Wakeup(at, tool, detail)`.
- `tests/harbor/shared/transcripts.py` (new `claude_children(objs)` after `claude_launch_root` L357-376). It:
  - walks every assistant object, at any depth, for `tool_use` blocks named in `SUBAGENT_TOOLS`. The launch time is the enclosing object's `timestamp`. `input.prompt`, `input.description` and `input.subagent_type` (absent means `"general-purpose"`) give the context; keep `input.run_in_background` only as a fallback.
  - joins `system/task_started` on `tool_use_id`, keeping only `task_type == "local_agent"`. Background `local_bash` tasks also emit it (s55 implement-epic `bt5kegl7r`). From it take `task_id`, `is_backgrounded` (the authoritative background value), `spawn_depth` and `parent_task_id`.
  - sets the parent from the issuing assistant object's `parent_tool_use_id` (the parent child's launch id), cross-checked with `parent_task_id`.
  - takes the end from `system/task_updated` (joined by `task_id`): `patch.end_time` epoch ms as UTC, and `patch.status`/`patch.error`.
  - takes the ending from `system/task_notification` (joined by `tool_use_id`). `status` maps `completed→completed`, `failed→failed`, `killed→stopped`, and no notification gives `no_hand_back`. `usage.duration_ms/1000` gives `duration_s`. A missing value gives `None` and `Unknown("subagents.children[<id>].duration_s", "claude reported no duration for a failed child")`. `summary` gives the hand-back first non-empty line. When there is no notification but there is a foreground `tool_result`, strip the `[Subagent hand-back] … The report follows:` frame and the 2-space indent first.
  - takes models from `message.model` of assistant objects whose `parent_tool_use_id` equals the child's id, and output from the sum of `claude_messages` chars (L299) for those messages.
  - excludes launches with no `task_started` (refused or invalid `subagent_type`; s55 implement-epic has 15 calls and 14 started). The total must equal `result.subagent_stats.spawned` when present.
- `tests/harbor/shared/transcripts.py` (new `claude_turns(objs)`): one `Turn` per `result` event. `woken_by` is `"task-notification"` when the result (or the user/init that opened the turn) carries `origin.kind == "task-notification"`, `"wakeup"` after a `command_lifecycle` with `wakeup_source`, and `"start"` for the first. `ended_at` is the last timestamped object before the `result`, because `result` has no timestamp (gotcha *harbor-agent-log-fields*).
- `tests/harbor/shared/transcripts.py` (new `claude_wakeups(objs)`): main-agent (no `parent_tool_use_id`) `tool_use` named `ScheduleWakeup` or `CronCreate`, or `Bash` with `input.run_in_background` true whose command contains `sleep`. Record the timestamp, tool and a short detail (`delaySeconds`/cron, or the command's first 80 chars). Reference: s55 epic line 43 `ScheduleWakeup delaySeconds 1200`; s55 implement-epic `sleep 240; cd /app; spektacular status …`.
- `tests/harbor/shared/transcripts.py:443-455` (new `atif_children(traj)` beside `atif_subagents`):
  - **Bob:** one `Child` per step whose `function_name` starts `"Running subagent"`. Kind is `arguments.name`, prompt is `arguments.description`, and description is the `function_name` suffix. Every other field is `None`, with one agent-wide `Unknown("subagents.children.<field>", "bob logs no sub-agent <field>")` per field.
  - **Codex:** returns `None` plus `Unknown("subagents.children", "codex reports no sub-agents")`.
- `tests/harbor/shared/tests/test_transcripts_summary.py:75-206`: extend the builders with `task_started(tool_use_id, task_id, background, depth, parent_task_id=None, task_type="local_agent", prompt=None)`, `task_updated(task_id, status, end_ms, error=None)`, `notification(…, summary=…)`, `schedule_wakeup`, `origin` on `result`. Add cases for:
  - nested children, with the parent named;
  - a refused launch;
  - a background bash task excluded;
  - a background child whose `tool_result` returns immediately (not ended);
  - a failed child without a duration (unknown);
  - a killed child (`stopped`);
  - no notification (`no_hand_back`);
  - foreground framed hand-back stripping;
  - prompt identity and length;
  - Bob and Codex unknowns;
  - turns with a notice-driven wake;
  - wake-ups, with sleep detection both ways.

  Keep L429-447 unchanged and passing.

**Complexity**: Medium
**Token estimate**: ~45k tokens
**Agent strategy**: 2 parallel agents: one for the Claude readers (`claude_children`, `claude_turns`, `claude_wakeups`) with their self-checks, one for `atif_children` with its Bob/Codex self-checks. Integrate sequentially, then run the whole shared self-check suite.

### Task: Record children step counts and idle time

**File changes**:
- `tests/harbor/shared/metrics.py:46`: `SCHEMA_VERSION = 2`. At L58, keep `SUBAGENT_FIELDS` for the version-1 counts and add `CHILD_FIELDS = ("id","parent","kind","description","background","spec","step","launched_at","ended_at","duration_s","ending","hand_back","models","output","prompt","prompt_chars")`.
- `tests/harbor/shared/metrics.py:101-110` (`AgentLog`): add `children: list[Child] | None`, `turns: list[Turn]` and `wakeups: list[Wakeup]`. Fill them in `read_agent_log` L139-182:
  - **claude path:** `tx.claude_children`, `claude_turns`, `claude_wakeups`.
  - **atif path:** `tx.atif_children`.
  - **codex-rollout path (L173):** `None` with "codex reports no sub-agents".
  - **`missing_log` (L113-118):** `None` with its reason.
- `tests/harbor/shared/metrics.py:246-340`:
  - make `read_steps` return the windows keyed by lane `(kind, name)` as well as the step dicts, instead of discarding them.
  - add `place_children(children, windows_by_lane)`:
    - **Spec:** `lane_for(child.prompt, lane names)` (L254-259). If that names no spec, take the spec of the top-level ancestor, walking `parent` links. In a single-lane run, use the lane.
    - **Step:** `window_of(windows[lane], launched_at)` (L246-251). `None` means the child was started outside any step.
    - For Bob, which has no timestamps, place by order using the trajectory's `goto_step` boundaries, the same as `step_breakdown` L262-331 does for messages.
  - add `step_child_counts`, which writes `children` into each step dict built at L262-331. Write `None` plus `Unknown("steps.children", …)` when children are unknown.
  - add `idle_seconds(instants)`, the longest gap between consecutive sorted instants. Instants are every Claude message timestamp (main and children, via `claude_messages`), each child's `ended_at`, and each session-log `Event.at` and `Event.end` (`SessionLog.all_events()`, `shared/sessionlog.py:266`). For Codex, use ATIF step timestamps plus the session log. For Bob, use the shell-log `ts` (`bob_log_span_s` source, L346) plus the session log. Fewer than two instants gives `None` and `Unknown("totals.idle_s", …)`.
- `tests/harbor/shared/metrics.py:401-446` (`build_record`):
  - add `totals.idle_s` and `subagents.children`, a list of dicts in `CHILD_FIELDS` order, with `prompt_chars = len(prompt)`, ISO-Z times and `models` sorted. Write `None` when unknown.
  - each new piece goes through its own `gather("subagents.children", …)` and `gather("totals.idle_s", …)` (L406-413), so a crash yields `null` plus a "recorder error" unknown.
- `tests/harbor/shared/tests/test_metrics.py`:
  - update `TOP_KEYS`/`SUBAGENT_KEYS` (L36-49), `shape()` (L510), `schema_version == 2` (L550), the whole-record equality at L637-661, the exact unknown lists at L562-567 and L884-896/944-948, and `record["subagents"] ==` at L560/792/943/959.
  - extend `claude_two_lane_log` (L366) with `task_started`/`task_updated`/`summary`.
  - add cases for:
    - placement by spec (own prompt and ancestor fallback) and by step (a goal reviewer at `check_goal`);
    - orchestrator launches with `step` null;
    - per-step counts including zero;
    - idle on fixed timestamps;
    - Bob and Codex unknowns without zeros;
    - a raising child reader leaving the reward untouched.

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Single agent, sequential. Placement, counts and record assembly share the windows plumbing, and the shape-test updates must be done together.

### Task: Check in the trimmed 2026-10-23 reference runs

**File changes**:
- `tests/harbor/shared/tests/trim_reference.py` (new, under `shared/tests/` so the Makefile's non-recursive `cp tests/harbor/shared/*.py` never ships it). Usage: `python3 trim_reference.py <trial-dir> <name>`. It writes `tests/harbor/shared/tests/data/<name>/`:
  - **`claude-code.txt`:** every line kept. For each object keep `type`, `subtype`, `timestamp`, `parent_tool_use_id`, `uuid`, `session_id`, `origin`, `message.model`, `message.id`, and `tool_use` id/name. Keep the input in full for `Agent`/`Task`/`ScheduleWakeup`/`CronCreate`, plus Bash `command`/`run_in_background`. Keep all `system/task_*`, `background_tasks_changed`, `init` (with `tools` dropped), `command_lifecycle` and `result` fields except `result`'s text. Replace every other text value with `"x" * len(original)`, so character counts are unchanged.
  - **`debug/*.jsonl`:** keep `seq`, `timestamp`, `session_id`, `command`, `duration_ms`, `exit_code`, `state_before`, `state_after` and `advanced`. Reduce `response` to `{"step": …}` when parseable, otherwise drop it.
  - **`build.json`:** synthesised from the job's `config.json` agent and suite.
  - **Self-verifying:** after writing, recompute on both copies the child list (count, prompts, durations, endings), `idle_seconds`, `sessionlog.plan_steps_entered` and `changelog_writes_by_repo`. Exit non-zero, deleting the output, on any difference.
- `tests/harbor/shared/tests/data/` (new): `m70-epic` from `tests/harbor/jobs/m70-claude-epic/epic-workflow-claude__Kkwgusn`, `s55-epic` from `s55-claude-epic/epic-workflow-claude__XLCYHsp`, `s55-plan` from `s55-claude-plan/plan-workflow-claude__52vDZP6`, and `s55-implement` from `s55-claude-implement/implement-workflow-claude__QQaSJXT`. Each directory has a `README` line naming its source job and date. If a source job is missing on disk, STOP and ask.
- `tests/harbor/shared/tests/test_reference_runs.py` (new):
  - **s55-epic:** child count equals `subagent_stats.spawned` (3); every prompt is identical to the launch input and `prompt_chars` equals its length (1723/1705/1759); durations are 109.6/107.6/137.0 s from the notifications; endings are all `completed`, with hand-back first lines `DONE: <spec>`; `background` is true for storage and help and false for commands; `idle_s >= 900` and `idle_s / wall_time_s > 0.25`. Expected ~942 s of ~1249 s.
  - **m70-epic:** `idle_s / wall_time_s < 0.25` (~88 of ~992 s).
  - **Trim check:** re-running the trim on the checked-in data is idempotent.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential: script, then generate, then tests. The data must come from the local jobs.

### Task: Show children and contexts in the report

**File changes**:
- `tests/harbor/shared/report.py:30-34`: add `IDLE_SHARE = 0.25` beside `DOMINANT_STEP_SHARE`.
- `tests/harbor/shared/report.py:188-217` (`TABLE_HEADERS`, `table()`): add an `idle` column (minutes via `minutes()` L148-153), with an `◀ idle` mark when `idle_s / run_wall_time(run) > IDLE_SHARE`, using `run_wall_time` L246-252.
- `tests/harbor/shared/report.py:269-316` (`breakdown`):
  - after the "Sub-agents:" line (L303-309), add `Idle: <s> of <wall> (<share>)` with the idle mark;
  - then a `render(["child","kind","spec","step","bg","seconds","ending","context"], rows, right={5,7})` table, one row per child. `child` is a short id with a `└` indent per depth; `step` prints `-` when null and not unknown; unknowns print `unknown`;
  - for a version-1 record (no `children` key), print `Children: not recorded (schema 1)`.
- `tests/harbor/shared/report.py:433-474` (`main`): add `--prompts`. It is valid only with `--run`; otherwise print a usage message and return 2, following the comparison-mode checks at L441-448. With `--run`, print after the breakdown a `── <id> <kind> <spec>/<step> (<chars> chars)` header followed by the full prompt, for each child.
- `tests/harbor/shared/tests/test_report.py`:
  - update the goldens `A1_BREAKDOWN` (L232) and `NULL_BREAKDOWN` (L578), and `PLAN_GROUP`/`SPEC_GROUP` (L215-230) for the idle column;
  - the constant pins at L686-694 gain `IDLE_SHARE`;
  - `make_record` (L74) gains children and `idle_s`;
  - add cases for the children table, `--prompts` output, `--prompts` without `--run` (exit 2, L1021-1045 pattern), the idle flag at 0.26 versus 0.25 versus 0.24, a version-1 record, and unknown children.

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Document the new record fields and report option

**File changes**:
- `README.md:476`: the job layout line is unchanged.
- `README.md:497-539` (`#### Run metrics`): `schema_version` becomes "currently `2`" (L512).
  - The `totals` row gains `idle_s`.
  - The `steps` row gains `children`.
  - The `subagents` row gains `children`, with each child key in backticks, the `ending` values, the `step: null` meaning, and that `durations_s` is kept for version-1 readers.
  - The `| Agent |` unknowns table gains Bob (child kind, description and prompt only) and Codex (`children` null).
  - Add a note on the idle flag threshold, with the constant at the top of `report.py`.
- `README.md:541-596` (`#### Comparing runs`): add a bash block with `make harbor-report ARGS='--run <trial> --prompts'`.
- `tests/harbor/shared/tests/test_readme_metrics.py:25-37`: add the new keys to `TOP_LEVEL_KEYS`/`SUBAGENTS_KEYS`, add an assertion that `--prompts` appears in a bash block, and add a `totals` `idle_s` check.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Judge the plan walkthrough from the session log

**File changes**:
- `tests/harbor/shared/sessionlog.py:134-228` (after `Lane`): add `plan_steps_entered(events) -> list[str]`, the `step_after` (`state_after.current_step`) of each `ok` event that is `plan new` or `plan goto` (`Event.is_verb`, `.ok`, `.step_after` at L43-92), in order.
- `tests/harbor/plan-workflow/tests/test_plan_workflow.py:1394-1438` (`test_walkthrough_rendered_before_finished`): replace the `_step_payload` (L1322-1342) scan with `SessionLog(DEBUG_DIR, kinds=("plan",)).lane("plan", PLAN_NAME).events` passed to `plan_steps_entered`. Assert that both `walkthrough` and `finished` were entered, and the walkthrough first. Import from the copied `sessionlog.py` (the Makefile already copies `shared/*.py` at L187).
- `tests/harbor/plan-workflow/tests/test_plan_workflow.py:108,936-953`: `EXPECTED_SPAWN_STEPS`/`TestSubAgentSpawning` becomes record-only. Remove the failing assertion, and replace it with one that, for Claude, the metrics placement shows a `children` count for discovery (any value, including 0). Or delete the class, with a comment pointing to the record. Never fail on zero.
- `tests/harbor/plan-workflow/tests/temp.py`: delete. It is a stray debug script pointing at an old checkout.
- `tests/harbor/shared/tests/test_sessionlog_rules.py` (new):
  - `plan_steps_entered` on `tests/harbor/shared/tests/data/s55-plan/debug/*_plan_*.jsonl` contains `walkthrough` before `finished`;
  - a synthetic lane `write_research → finished` fails;
  - failed gotos (exit 1) are not counted.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Judge per-repo changelog writes from the session log

**File changes**:
- `tests/harbor/shared/sessionlog.py` (beside `plan_steps_entered`): add `changelog_writes_by_repo(events) -> dict[str | None, int]`. It counts `ok` events with `is_verb("changelog","file","write")`. The key is the value after `--repo` in `command`, or `None` when there is no `--repo` (the project record). `--repo=<v>` is accepted too.
- `tests/harbor/implement-workflow/tests/test_implement_workflow.py:98-104,332-370`: replace the `PROJECT_WRITE_SUBSTR`/`COLOCATED_WRITE_SUBSTR`/`EXTERNAL_WRITE_SUBSTR` command-text matches with `changelog_writes_by_repo(SessionLog(DEBUG_DIR, kinds=("implement",)).lane("implement", PLAN_NAME).events)`. Assert `None` (project), `COLOCATED_REPO_NAME` (`auth`) and `EXTERNAL_REPO_NAME` (`docs`) each have at least 1. The Makefile `harbor-test-implement` already copies `shared/*.py` (L210).
- `tests/harbor/shared/tests/test_sessionlog_rules.py`:
  - on `data/s55-implement/debug/*.jsonl`, all three keys are present (seq 14-16 exit 0), while the earlier failed writes, seq 5-9 with exit 1, are not counted;
  - a synthetic log without the `docs` write fails that key only;
  - argument order and `--repo=` variants are covered.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Tell the plan and implement agents that no one answers

**File changes**:
- `tests/harbor/plan-workflow/instruction.md:55-59`: before the walkthrough paragraph, add a `## You are running non-interactively` section mirroring `tests/harbor/spec-workflow/instruction.md:35-45`: "This session is headless: there is no human at the other end, and no one answers between your turns … **Never end your turn waiting for a reply** …". At L88-89, fix the stale `status: in-progress`/`status: completed` to `document_status: draft`/`final`, matching what the verifier checks.
- `tests/harbor/implement-workflow/instruction.md:46-50`: add the same section.
- `tests/harbor/shared/tests/test_suite_wiring.py` (new parametrised check over `SUITES` L44, using the suite's `instruction.md`): every instruction contains "no one" and "between your turns" (or "headless"), so a future suite cannot drop the wording.

**Complexity**: Low
**Token estimate**: ~6k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Make required delegation explicit in the step instructions

**File changes**:
- `templates/steps/spec/08-verification.md:25-36`: after "**Do not review the spec yourself.**" (L27), add: "If your agent can start sub-agents, this step is done only by one: reviewing the staged file in your own context does not complete it." Keep the L29-34 brief (path `.spektacular/tmp/spec_template.md`, "review only what is written in that file") and the fallback for agents without a sub-agent tool, unchanged.
- `templates/steps/implement/04-test.md:3,22`: strengthen L3 to "This step runs in a **sub-agent**. If your agent can start sub-agents, launch one before writing any test; tests written in your own context do not complete this step." The hand-off at L16 (criteria and the task's `context.md` section) is unchanged.
- `templates/steps/implement/05-verify.md:3,20`: the same pattern: "If your agent can start sub-agents, the verification commands run in one; running them in your own context does not complete this step."
- `internal/steps/implement/steps_test.go:319-340`: pin the two new sentences for `test` and `verify`.
- `internal/steps/spec/steps_test.go` (or a new template test beside it): pin the new verification sentence and the existing "review only what is written in that file". No test covers that wording today.
- `internal/agent/instruction_surface_test.go:32-46`: no change needed. Confirm the new text uses no banned substring.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution. Run `go test ./...` after editing. Never re-init this repo to see the rendered result (convention *plans never change the active install*).

### Task: Add worktree paths and the no-wake-up rule to the epic skills

**File changes**:
- `templates/skills/workflows/spek-plan-epic/SKILL.md:42-48` (after the "still running" paragraph at L48): add a paragraph: "**Never schedule a timed wake-up while children run.** Do not use a wake-up, timer, sleep or recurring job to check on children: their completion notices wake you. A pending wake-up keeps the session alive after the work is done."
- `templates/skills/workflows/spek-implement-epic/SKILL.md:56-60`: the same paragraph.
- `templates/skills/workflows/spek-implement-epic/SKILL.md:64-73` (the child-prompt list): add an item: "the spec's code locations: every repo's worktree path that `{{command}} epic worktree --data '{"spec":"<spec>"}'` reported, and that the child works only there". It goes before the store-access rule, and is linked from the worktree step at L46-52.
- `templates/plan_epic_skill_test.go` and `templates/implement_epic_skill_test.go`: pin the new phrases. Leave the headless phrases at plan :113-114,151 and implement :134-135,208 untouched. Update the "ask" allowlists (plan :140-194, implement :180-245) if the new text contains a word starting with "ask".

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Add the delegation rules and their oracle

**File changes**:
- `tests/harbor/shared/delegation.py` (new, stdlib only, imports `transcripts`, `metrics` and `sessionlog`):
  - **The oracle:** `REQUIRED`, `SUGGESTED`, `PER_SPEC_CHILD` and `REQUIRED_CONTEXT`, as in plan.md § Data Structures. The context phrase groups are:
    - **epic-plan:** the spec name, `/app`, and `"orchestrated":true` (whitespace-insensitive).
    - **epic-implement:** the same, plus every worktree path from the lane's successful `epic worktree` session-log response.
    - **epic-resume:** "resume". It applies only when the lane has `resumes` or a second child (`Lane.children > 1`, `shared/sessionlog.py:134-228`).
    - **spec-review:** `.spektacular/tmp/spec_template.md` and "review only what is written".
  - **The rules:** `missing_required`, `context_gaps` and `wakeups_while_children_run`, each returning `Failure(rule, subject, detail)`. A child counts as running from `launched_at` to `ended_at`, or to the run end when that is unknown. `epic_children_backgrounded` and `epic_children_foreground_together` are added later, by the last task.
  - **Required-step matching:** a step window counts as satisfied when a placed child's step is that step in that lane, at any depth. For an epic lane, the depth-1 child that worked the lane is the per-spec child.
  - **Claude guard:** `is_claude(build_info, agent_log)` returns true only when `build.json` says `agent == "claude"` and a Claude transcript was read.
- `tests/harbor/shared/tests/test_delegation.py` (new):
  - both directions for each `REQUIRED` entry;
  - `SUGGESTED` never failing (a discovery with 0 children passes);
  - each context gap named;
  - the wake-up rule on `data/s55-epic` (fails, naming `ScheduleWakeup delaySeconds=1200`) and `data/m70-epic` (passes);
  - **the oracle tie:** read `templates/steps/spec/08-verification.md`, `templates/steps/implement/04-test.md`, `05-verify.md` and `05b-check_goal.md`, and assert each contains its required-delegation marker sentence. Scan `templates/steps/**/*.md` for the marker "does not complete this step"/"does not complete it" and assert the set of files carrying it maps exactly onto `REQUIRED`. `05b-check_goal.md:56` "never as yourself" counts for `check_goal`.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential. The rules and oracle are tightly coupled, and the reference data fixes expectations.

### Task: Enforce delegation in the spec and goal-check suites

**File changes**:
- `tests/harbor/spec-workflow/tests/test_spec_workflow.py:559-570` (`TestVerificationStep`): add `test_review_was_a_briefed_child`. With the Claude guard (otherwise `pytest.skip`), read `/logs/agent/claude-code.txt` through `transcripts.read_jsonl`, place children with `metrics.read_steps` and `metrics.place_children` over `/app/.spektacular/debug`, then assert `missing_required` and `context_gaps` (spec-review) are empty. The failure message lists each `Failure`. The Makefile already copies `shared/*.py` (L115); confirm `delegation.py` arrives via the same glob.
- `tests/harbor/goal-check-workflow/tests/test_goal_check.py:292-347`: add `rule_required_children`, using `missing_required` for `check_goal`, plus `test` and `verify` for each window those steps opened in the retry (scenario `unmet`). It sits beside `rule_reviewer_input`/`rule_cheaper_reviewer` and follows the `launches_or_skip` Claude-only pattern.
- `tests/harbor/goal-check-workflow/tests/test_verifier_selfcheck.py:148-166`: add both-direction cases for `rule_required_children`, with synthetic `task_started` lines for the reviewer.
- `tests/harbor/spec-workflow/solution/solve.sh`: no change. The oracle has no transcript, so the Claude guard skips.

**Complexity**: Low
**Token estimate**: ~18k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Enforce delegation and wake-up rules in the epic suites

**File changes**:
- `tests/harbor/epic-workflow/tests/test_epic_workflow.py:110-163` (`TestOrchestration`): add a `TestChildren` class, Claude-guarded and skipping for others, so the agent-independent checks stay as they are:
  - every spec lane has a depth-1 child placed on it (`PER_SPEC_CHILD`);
  - `context_gaps` with the epic-plan groups (and epic-resume where it applies) is empty;
  - `wakeups_while_children_run` is empty.
  
  Update the module docstring (L6-11), which says the verifier never reads a transcript, to "except the Claude-only child checks".
- `tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py:209-281`: the same class with the epic-implement groups (worktree paths read from the session log's `epic worktree` responses for each spec), plus `missing_required` for every `test`, `verify` and `check_goal` window in every spec lane. Update the docstring at L7-10.
- Knowledge entry `architecture/testing-architecture.md` (repo store `spektacular`, the "epic suite is the agent-independent one" paragraph): the user decided on 2026-10-24 to add the Claude-only checks and update this entry. Through the `spek-knowledge` skill (`spektacular knowledge read`, then `spektacular knowledge write --from <staged file>`), revise the paragraph to say the epic verifiers' shared checks read only CLI state and the session log and serve every agent, while a Claude-only class also reads the transcript for child-agent checks and skips for other agents. Never edit the file directly.
- `tests/harbor/shared/tests/test_delegation.py`: add synthetic epic-implement cases: a lane with no `test` child fails, naming spec and step, and a child prompt without the worktree path fails.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: 2 parallel agents, one per epic verifier, after a shared look at `delegation.py`. Integrate by running both suites' self-checks and `test_delegation.py`.

### Task: Add the background-child probe task

**File changes**:
- `tests/harbor/background-child/` (new), modelled on `tests/harbor/standing-rules/`, the smallest suite:
  - **`instruction.md`:** headless, no one answers. "Start exactly one sub-agent **in the background** whose only job is to run `sleep 300` three times in sequence (each as its own shell call with a timeout above 300 s) and then reply `DONE: probe`. Then end your turn without waiting. When its completion notice arrives, write the child's reply to `/app/probe.txt` and stop. Never schedule a wake-up."
  - **`task.toml`:** agent timeout ~30 min.
  - **`environment/Dockerfile`:** a minimal base with `python3` and `pytest`, and `SPEKTACULAR_SESSION_LOG=on` for wiring parity.
  - **`tests/test.sh`:** follows the pinned test.sh shape (`test_suite_wiring.py:221-252`).
  - **`tests/test_background_child.py`:** Claude-guarded. It uses `claude_children` and `claude_turns`. It asserts exactly one background child (`background is True`) and that `duration_s > 600` or `ending == "stopped"`. It never asserts which outcome happened, and prints `PROBE OUTCOME: <ending> after <duration_s> s; notice-woken turn: <bool>`.
- `Makefile` (after `harbor-test-goal-check` L350-371): `harbor-test-background-child` / `_harbor-test-background-child`, rendering into `tests/harbor/.build/background-child-claude`, with `cp tests/harbor/shared/*.py <tests>/` and `$(call write_build_info,…)`, following the render pattern at L234-252.
- `tests/harbor/shared/tests/test_suite_wiring.py:44,140`: add the probe to `SUITES`. The copy count at L198 follows.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Run the probe and record the ten-minute outcome

**File changes**:
- Run `make harbor-test-background-child`, which needs Docker and the machine's Claude login, as every Claude harbor target does. If either is missing, or the run fails for an auth reason (the token rotation noted in the spec's non-goals), STOP and ask. Never assume an outcome.
- `tests/harbor/shared/tests/data/probe-background-child/` (new): produced by `trim_reference.py` from the collected job.
- `tests/harbor/shared/tests/test_reference_runs.py`: add a case asserting the probe fixture's single child is background with `duration_s > 600` or `ending == "stopped"`, and pinning the observed ending.
- `tests/harbor/shared/tests/data/probe-background-child/OUTCOME`: one line, `completed <duration_s> s <date> <claude-code version>` or `stopped …`. The next task reads it. Also record it in the implement lane's working notes.

**Complexity**: Low
**Token estimate**: ~8k tokens (plus ~20 min of wall-clock run time)
**Agent strategy**: Single agent, sequential execution.

### Task: Rewrite the epic headless guidance to the probe outcome

**File changes**:
- `templates/skills/workflows/spek-plan-epic/SKILL.md:44-46`, and `:77` (fresh child in the foreground):
  - **If the outcome is `completed`:** replace the headless paragraph with: "**Run children in the background, even in a headless session.** Headless means only that no one answers between your turns. Background children keep running after you end your turn, and each one's completion notice starts your next turn (verified on <date>: a background child ran <duration> under headless Claude Code and completed). So start ready specs in the background, end your turn, and handle each hand-back when its notice arrives." At L77, drop "In a headless session, always start the fresh child, in the foreground", keeping the "do one or the other, never both" rule.
  - **If the outcome is `stopped`:** keep the foreground workaround. Replace the unverified "stops one after ten minutes" with the recorded figure and date, and add that being told no one answers is a reason to keep children in your turn only because of that limit.
- `templates/skills/workflows/spek-implement-epic/SKILL.md:56-58,102`: the same change.
- `templates/plan_epic_skill_test.go:113-114,151` and `templates/implement_epic_skill_test.go:134-135,208`: replace the pinned headless phrases with the new ones, and assert the retired phrases are absent ("keep every child inside your turn" and the foreground-in-headless sentence when the outcome is `completed`). Keep the "ask" allowlists (plan :140-194, implement :180-245) in step. The cut-off resume pins at plan :446-455 and implement :417-440 are unchanged.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution. Read `data/probe-background-child/OUTCOME` first.

### Task: Check that the epic suites run children as a real session does

**File changes**:
- `tests/harbor/shared/delegation.py`: add `epic_children_backgrounded(placed, turns, lanes)`. It requires that:
  - every lane's depth-1 child has `background True`;
  - at least one `Turn` ended while a depth-1 child was running;
  - at least one turn has `woken_by == "task-notification"`.
  
  Add `epic_children_foreground_together` for the `stopped` outcome: every lane's depth-1 child has `background False`, and the ready children are launched from the same assistant message. Choose which one applies by reading the probe's OUTCOME, kept as a module constant set by the previous task.
- `tests/harbor/epic-workflow/tests/test_epic_workflow.py` and `tests/harbor/implement-epic-workflow/tests/test_implement_epic_workflow.py` (`TestChildren`, from the epic-suites delegation task): add `test_children_run_as_a_real_session_does`, calling the chosen rule.
- `tests/harbor/epic-workflow/instruction.md:53-61` and `tests/harbor/implement-epic-workflow/instruction.md:54-62`: keep "This session is headless: no one replies between your turns" and "Never end your turn waiting for a reply". Add, only if the outcome is `completed`, "Ending your turn while your children run is expected: their completion notices start your next turn." Without it, "Never end your turn waiting for a reply" could be read as forbidding that.
- `tests/harbor/shared/tests/test_delegation.py`: both directions for both rules. `data/m70-epic` (all foreground) fails `epic_children_backgrounded`, and a synthetic background run with a notice-driven turn passes.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

Per task:
- **Read every child agent from each agent log:**
  - synthetic stream-json and trajectory cases in `shared/tests/test_transcripts_summary.py`;
  - the existing `SubagentLaunch` pins stay green.
- **Record children step counts and idle time:**
  - `shared/tests/test_metrics.py` shape and placement cases, updated for schema 2;
  - the broken-section case proves the reward is untouched.
- **Check in the trimmed 2026-10-23 reference runs:** `shared/tests/test_reference_runs.py` proves the child-record and idle acceptance criteria on real runs. The trim script self-verifies on write.
- **Show children and contexts in the report:** `shared/tests/test_report.py` goldens, the `--prompts` usage error and the idle threshold edges.
- **Document the new record fields and report option:** `shared/tests/test_readme_metrics.py`.
- **Judge the plan walkthrough from the session log** and **Judge per-repo changelog writes from the session log:** `shared/tests/test_sessionlog_rules.py` on the s55 plan and implement reference data, plus synthetic failing lanes.
- **Tell the plan and implement agents that no one answers:** the suite-wiring self-check asserts the wording in every suite.
- **Make required delegation explicit in the step instructions** and **Add worktree paths and the no-wake-up rule to the epic skills:** Go phrase pins in `internal/steps/implement/steps_test.go`, a spec verification template test, and `templates/{plan,implement}_epic_skill_test.go`, via `go test ./...`.
- **Add the delegation rules and their oracle:**
  - `shared/tests/test_delegation.py`: both directions per rule, plus the reference-run wake-up case;
  - the oracle-to-template tie.
- **Enforce delegation in the spec and goal-check suites:** `goal-check-workflow/tests/test_verifier_selfcheck.py` cases for the new rule. The spec suite's rule is proven by `test_delegation.py`.
- **Enforce delegation and wake-up rules in the epic suites:** synthetic epic cases in `test_delegation.py`.
- **Add the background-child probe task:** the suite-wiring self-check covers the target. The verifier prints the outcome without asserting which one.
- **Run the probe and record the ten-minute outcome:** the reference-run case pins the probe fixture.
- **Rewrite the epic headless guidance to the probe outcome:** Go skill tests pin the new wording and assert the retired phrases are absent.
- **Check that the epic suites run children as a real session does:** both directions per rule in `test_delegation.py`. The m70 epic data, all foreground, fails the background rule.

End-to-end confirmation is the manual harbor batch listed in plan.md's Testing Approach. That section also maps every success metric and manual review.

## Project References

- **Spec:** `000071_harbor-child-agents` (final, 2026-10-23). No design references.
- **Prior plan:** `000070_harbor-run-metrics`, the recorder and report architecture this extends.
- **Knowledge (repo store `spektacular`):**
  - `architecture/testing-architecture.md`: the hand-maintained oracles and harbor coupling rules. Its epic-suite paragraph is updated by the epic-suites delegation task, per the user's 2026-10-24 decision.
  - `gotchas/harbor-agent-log-fields.md`: no timestamps on system events; failed children may lack a duration.
  - `gotchas/harbor-agent-logs-and-artifacts.md`: the root-owned `agent/sessions`, and the session log carried out per agent.
  - Conventions: plans-never-change-the-active-install, tests-must-pass-for-done, tests-must-not-depend-on-order, store-files-must-be-written-through-the-cli.
- **Repo root:** `spektacular`, at `/home/nicj/code/github.com/hivecommons/spektacular`. All tasks.
- **Reference runs (gitignored, local):** `tests/harbor/jobs/m70-claude-epic`, `s55-claude-epic`, `s55-claude-plan`, `s55-claude-implement`. Also used for exploration: `s55-claude-implement-epic`, `m70-claude-implement-epic`, `s55-claude-spec`, `m70-claude-spec`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Most tasks are Low. The reader, recorder, reference-data, delegation-rules and epic-suite tasks are Medium. Keep the reference transcripts out of context: inspect them with short `python3` filters, never by reading whole files (the epic transcripts are around 1 MB each).

## Migration Notes

- `metrics.json` moves to `schema_version: 2`. The report keeps reading version-1 records, and old job directories need no rewrite.
- No change to this repo's `.spektacular/` configuration or installed skills. The template changes reach installed skills only through a later `make install-local` and the version check, between workflows.

## Performance Considerations

- Full child prompts make `metrics.json` larger: about 1.5–5.5k characters per child, up to roughly 17 children in an implement-epic run, so tens of kilobytes. That is acceptable, and the report still reads only records.
- Building the child list is linear in the transcript size. Placement and idle time are a sort plus a scan.
- The probe adds one roughly 15–20 minute harbor run, only when explicitly invoked.
