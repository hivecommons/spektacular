"""Self-check for the transcript rules in test_repo_workflow.py.

A verifier that reads a transcript can be wrong in a way no harbor run will
ever reveal: a rule whose predicate can never return a violation passes every
run and proves nothing. So each transcript rule is exercised here against two
small synthetic transcripts — one that honours it and one that breaks it —
and both directions are asserted.

This file never runs in the harbor container. It needs no container, no
agent, no /app and no /logs: it builds transcript objects in memory and calls
the pure functions directly. Run it locally with

    python3 -m pytest tests/harbor/repo-workflow/tests/test_verifier_selfcheck.py -v

The Makefile deliberately copies only test_repo_workflow.py into the built
task, so this file stays out of the image.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import test_repo_workflow as rw  # noqa: E402


# ---------------------------------------------------------------------------
# Synthetic transcript builders
#
# These mimic the Claude-style transcript shape the verifier reads: one JSON
# object per line, `assistant` objects carrying either a text block or a Bash
# tool_use block.
# ---------------------------------------------------------------------------

def text(body: str) -> dict:
    """An assistant turn of prose — what the user is shown."""
    return {"type": "assistant", "message": {"content": [{"type": "text", "text": body}]}}


def bash(command: str) -> dict:
    """An assistant turn that runs a shell command — working detail, never
    shown to the user."""
    return {
        "type": "assistant",
        "message": {
            "content": [
                {"type": "tool_use", "name": "Bash", "input": {"command": command}}
            ]
        },
    }


def tool_result(body: str) -> dict:
    """A tool result coming back to the agent. Spektacular's rendered step
    instructions arrive this way and are full of the vocabulary the agent must
    not repeat, which is exactly why the rules must not scan them."""
    return {
        "type": "user",
        "message": {
            "content": [{"type": "tool_result", "content": body}]
        },
    }


def goto(payload: str) -> dict:
    return bash(f"spektacular repo goto --data '{payload}'")


Q_LOCATE = "Which folder on this machine is the repo you'd like to add?"
Q_NAME = "It calls itself `harbour`. Shall I add it under the name harbour?"
Q_DESC = (
    "I'd summarise it as: Tide-gate scheduling for dockside cranes. "
    "Does that read right to you?"
)
Q_ROLE = "It looks like an application to me rather than a library. Agreed?"
Q_TAGS = "I'd file it under typescript and scheduling. Any others you'd add?"
Q_CONFIRM = (
    "I'll add harbour and create a folder at /srv/harbour to keep this "
    "project's files for it. Nothing else in that repo is touched. "
    "Shall I go ahead?"
)


def good_one_at_a_time() -> list[dict]:
    """A transcript that honours every rule: one cold question, then four
    proposals one per exchange, then the confirmation."""
    return [
        text("I'll initialize the project first."),
        bash("spektacular init claude"),
        text(Q_LOCATE),
        text("The repo is at /srv/harbour."),
        goto('{"step":"name","location":"/srv/harbour"}'),
        tool_result("Record the footprint via repo.yaml. Run repo goto --data …"),
        text(Q_NAME),
        text("Yes, harbour is right."),
        goto('{"step":"description","name":"harbour"}'),
        text(Q_DESC),
        text("That's it exactly."),
        goto('{"step":"role","description":"Tide-gate scheduling for dockside cranes"}'),
        text(Q_ROLE),
        text("Agreed."),
        goto('{"step":"tags","role":"application"}'),
        text(Q_TAGS),
        text("Those two are fine."),
        goto('{"step":"placement","tags":["typescript","scheduling"]}'),
        goto('{"step":"confirm","placement":"inside"}'),
        text(Q_CONFIRM),
        text("Go ahead."),
        goto('{"step":"register"}'),
        goto('{"step":"finished"}'),
    ]


def good_delegated() -> list[dict]:
    """A transcript that honours the delegated path: the user hands the
    remaining details over in reply to the first proposal, and is not asked
    about them again."""
    return [
        text(Q_LOCATE),
        text("The repo is at /srv/harbour."),
        goto('{"step":"name","location":"/srv/harbour"}'),
        text(Q_NAME + " If you'd rather not go through the rest one at a time, "
                      "I can fill them in myself."),
        text("just use what you think"),
        goto(
            '{"step":"placement","name":"harbour",'
            '"description":"Tide-gate scheduling for dockside cranes",'
            '"role":"application","tags":["typescript","scheduling"]}'
        ),
        goto('{"step":"confirm","placement":"inside"}'),
        text(Q_CONFIRM),
        text("Go ahead."),
        goto('{"step":"register"}'),
        goto('{"step":"finished"}'),
    ]


def replace(objs: list[dict], old: dict, new) -> list[dict]:
    """Return objs with the first occurrence of `old` replaced by `new`
    (a single object, a list of objects, or None to drop it)."""
    out: list[dict] = []
    done = False
    for obj in objs:
        if not done and obj == old:
            done = True
            if new is None:
                continue
            out.extend(new if isinstance(new, list) else [new])
            continue
        out.append(obj)
    assert done, "fixture edit did not match anything — the fixture drifted"
    return out


# ---------------------------------------------------------------------------
# Baseline: the honouring transcripts must satisfy every rule at once.
# ---------------------------------------------------------------------------

def test_good_transcript_satisfies_every_rule():
    objs = good_one_at_a_time()
    assert len(rw.questions_before_first_goto(objs)) == 1
    assert rw.proposal_precedes_payload(objs) == []
    assert rw.question_order_violations(objs) == []
    assert rw.banned_terms_in_assistant_text(objs) == []
    assert rw.repo_add_commands(objs) == []
    _, confirm = rw.confirmation_turn(objs)
    assert confirm == Q_CONFIRM
    assert rw.command_text_in(confirm) == []


def test_good_delegated_transcript_satisfies_its_rules():
    objs = good_delegated()
    assert len(rw.questions_before_first_goto(objs)) == 1
    assert rw.proposal_precedes_payload(objs) == []
    assert rw.delegation_handover_index(objs) is not None
    assert rw.questions_after_delegation(objs) == []
    assert rw.banned_terms_in_assistant_text(objs) == []


# ---------------------------------------------------------------------------
# Rule: only the repository is asked for cold
# ---------------------------------------------------------------------------

def test_cold_question_rule_fails_on_a_second_cold_question():
    broken = replace(
        good_one_at_a_time(),
        text(Q_LOCATE),
        [text(Q_LOCATE), text("And what would you like to call it?")],
    )
    questions = rw.questions_before_first_goto(broken)
    assert len(questions) == 2, questions


def test_cold_question_rule_fails_when_nothing_was_asked():
    broken = replace(good_one_at_a_time(), text(Q_LOCATE), None)
    assert rw.questions_before_first_goto(broken) == []
    assert not rw.location_given_to_new(broken)


def known_location() -> list[dict]:
    """The good transcript with the location the user already named passed to
    `repo new`, so the locate question and its answer are gone."""
    objs = replace(good_one_at_a_time(), text(Q_LOCATE),
                   bash("""spektacular repo new --data '{"location":"/srv/harbour"}'"""))
    return replace(objs, text("The repo is at /srv/harbour."), None)


def test_cold_question_rule_accepts_no_question_for_a_known_location():
    objs = known_location()
    assert rw.location_given_to_new(objs)
    assert rw.questions_before_first_goto(objs) == []


def test_cold_question_rule_fails_on_a_question_after_a_known_location():
    broken = replace(known_location(), bash("spektacular init claude"),
                     [bash("spektacular init claude"), text("And what would you like to call it?")])
    assert rw.location_given_to_new(broken)
    assert len(rw.questions_before_first_goto(broken)) == 1


# ---------------------------------------------------------------------------
# Rule: a value must be proposed in the question that precedes it
# ---------------------------------------------------------------------------

def test_proposal_rule_fails_when_the_question_states_no_value():
    broken = replace(
        good_one_at_a_time(),
        text(Q_ROLE),
        text("What sort of thing would you say this repo is?"),
    )
    violations = rw.proposal_precedes_payload(broken)
    assert violations, "an empty-handed question recorded 'application' unchallenged"
    assert any("application" in v for v in violations), violations


def test_proposal_rule_fails_when_a_recorded_tag_was_never_offered():
    broken = replace(
        good_one_at_a_time(),
        goto('{"step":"placement","tags":["typescript","scheduling"]}'),
        goto('{"step":"placement","tags":["typescript","tide-tables"]}'),
    )
    violations = rw.proposal_precedes_payload(broken)
    assert any("tide-tables" in v for v in violations), violations


def test_proposal_rule_fails_when_no_question_preceded_the_value():
    broken = [
        goto('{"step":"name","location":"/srv/harbour"}'),
        goto('{"step":"description","name":"harbour"}'),
    ]
    violations = rw.proposal_precedes_payload(broken)
    assert any("no question before it" in v for v in violations), violations


def test_proposal_rule_ignores_the_delegated_fields():
    """The delegated hand-over carries description, role and tags that were
    never put to the user — by design. Only `name` is checked there, so the
    rule must stay quiet about the other three."""
    assert rw.proposal_precedes_payload(good_delegated()) == []


def test_proposal_rule_still_checks_the_name_when_delegating():
    broken = replace(
        good_delegated(),
        goto(
            '{"step":"placement","name":"harbour",'
            '"description":"Tide-gate scheduling for dockside cranes",'
            '"role":"application","tags":["typescript","scheduling"]}'
        ),
        goto(
            '{"step":"placement","name":"tidegate",'
            '"description":"Tide-gate scheduling for dockside cranes",'
            '"role":"application","tags":["typescript","scheduling"]}'
        ),
    )
    violations = rw.proposal_precedes_payload(broken)
    assert any("tidegate" in v for v in violations), violations


# ---------------------------------------------------------------------------
# Rule: one question per exchange, in order
# ---------------------------------------------------------------------------

def test_order_rule_fails_when_one_turn_asks_two_things():
    """Dropping the role question leaves the description turn as the last
    question before both the description and role calls — which is what a
    single message asking about both looks like from the transcript."""
    broken = replace(good_one_at_a_time(), text(Q_ROLE), None)
    violations = rw.question_order_violations(broken)
    assert any("single assistant turn" in v for v in violations), violations


def test_order_rule_fails_when_one_call_records_two_values():
    broken = replace(
        good_one_at_a_time(),
        goto('{"step":"role","description":"Tide-gate scheduling for dockside cranes"}'),
        goto(
            '{"step":"tags","description":"Tide-gate scheduling for dockside cranes",'
            '"role":"application"}'
        ),
    )
    violations = rw.question_order_violations(broken)
    assert any("together" in v for v in violations), violations


def test_order_rule_fails_when_the_questions_arrive_out_of_order():
    objs = good_one_at_a_time()
    broken = replace(
        objs,
        goto('{"step":"role","description":"Tide-gate scheduling for dockside cranes"}'),
        goto('{"step":"role","role":"application"}'),
    )
    broken = replace(
        broken,
        goto('{"step":"tags","role":"application"}'),
        goto('{"step":"tags","description":"Tide-gate scheduling for dockside cranes"}'),
    )
    violations = rw.question_order_violations(broken)
    assert any("recorded in the order" in v for v in violations), violations


def test_order_rule_fails_when_the_name_question_also_raises_tags():
    broken = replace(
        good_one_at_a_time(),
        text(Q_NAME),
        text(
            "It calls itself `harbour`. Shall I add it under the name harbour, "
            "and shall I use the tags typescript and scheduling?"
        ),
    )
    violations = rw.question_order_violations(broken)
    assert any("also raised tags" in v for v in violations), violations


# ---------------------------------------------------------------------------
# Rule: no internal vocabulary reaches the user
# ---------------------------------------------------------------------------

def test_vocabulary_rule_fails_on_a_leaked_term():
    broken = replace(
        good_one_at_a_time(),
        text(Q_CONFIRM),
        text(
            "I'll create harbour's footprint at /srv/harbour and write its "
            "repo.yaml. Shall I go ahead?"
        ),
    )
    found = rw.banned_terms_in_assistant_text(broken)
    labels = {term for term, _ in found}
    assert "footprint" in labels and "repo.yaml" in labels, found


def test_vocabulary_rule_fails_on_a_leaked_command():
    broken = replace(
        good_one_at_a_time(),
        text(Q_NAME),
        text("Running `repo goto --data '{\"step\":\"description\"}'` now — "
             "is harbour the right name?"),
    )
    labels = {term for term, _ in rw.banned_terms_in_assistant_text(broken)}
    assert {"repo goto", "--data"} <= labels, labels


def test_vocabulary_rule_ignores_commands_the_agent_runs():
    """THE boundary this rule lives or dies on. Every honouring transcript
    already runs `repo goto --data …` and reads back instructions naming
    footprints and repo.yaml; if the scan reached tool inputs or tool results
    it would fail every single run."""
    objs = good_one_at_a_time()
    commands = " ".join(
        e["command"] for e in rw.transcript_events(objs) if e["kind"] == "bash"
    )
    assert "--data" in commands and "repo goto" in commands
    assert any(o.get("type") == "user" for o in objs), "fixture has no tool results"
    assert rw.banned_terms_in_assistant_text(objs) == []


def test_vocabulary_rule_ignores_the_report_after_the_artifacts_copy():
    objs = good_one_at_a_time() + [
        bash("cp -r /app/.spektacular /logs/artifacts/spektacular"),
        text("The repo was registered through the guided flow, not via `spektacular repo add`."),
    ]
    assert rw.banned_terms_in_assistant_text(objs) == []


def test_vocabulary_rule_still_scans_the_conversation_before_the_copy():
    objs = good_one_at_a_time() + [
        text("I could have used repo add instead."),
        bash("cp -r /app/.spektacular /logs/artifacts/spektacular"),
    ]
    assert any(t == "repo add" for t, _ in rw.banned_terms_in_assistant_text(objs))


# ---------------------------------------------------------------------------
# Rule: the guided flow, not `repo add`
# ---------------------------------------------------------------------------

def test_repo_add_rule_fails_when_the_direct_command_is_used():
    broken = good_one_at_a_time() + [
        bash("spektacular repo add --data '{\"name\":\"harbour\",\"location\":\"/srv/harbour\"}'")
    ]
    assert rw.repo_add_commands(broken), "a direct `repo add` went unnoticed"


def test_repo_add_rule_ignores_the_words_in_prose():
    """Only commands are scanned here, so the phrase appearing in prose is
    the vocabulary rule's business, not this one's."""
    objs = good_one_at_a_time() + [text("I could have used repo add instead.")]
    assert rw.repo_add_commands(objs) == []
    assert any(t == "repo add" for t, _ in rw.banned_terms_in_assistant_text(objs))


# ---------------------------------------------------------------------------
# Rule: the confirmation names repo, folder and code location, quotes no command
# ---------------------------------------------------------------------------

def test_confirmation_rule_fails_when_a_command_is_quoted():
    broken = replace(
        good_one_at_a_time(),
        text(Q_CONFIRM),
        text("I'll run spektacular repo goto to finish up. Shall I go ahead?"),
    )
    _, confirm = rw.confirmation_turn(broken)
    assert rw.command_text_in(confirm), confirm


def test_confirmation_rule_fails_when_the_path_is_missing():
    broken = replace(
        good_one_at_a_time(),
        text(Q_CONFIRM),
        text("All set — shall I go ahead and add harbour?"),
    )
    _, confirm = rw.confirmation_turn(broken)
    assert rw.normalize("/srv/harbour") not in rw.normalize(confirm)


# ---------------------------------------------------------------------------
# Rule: delegation ends the questioning
# ---------------------------------------------------------------------------

def test_delegation_rule_fails_when_questioning_continues():
    broken = replace(
        good_delegated(),
        goto('{"step":"confirm","placement":"inside"}'),
        [
            text("Before I go on — are typescript and scheduling the right tags?"),
            text("Fine."),
            goto('{"step":"confirm","placement":"inside"}'),
        ],
    )
    offenders = rw.questions_after_delegation(broken)
    assert offenders, "a question after the hand-over went unnoticed"


def test_delegation_rule_is_silent_when_nothing_was_delegated():
    assert rw.delegation_handover_index(good_one_at_a_time()) is None
    assert rw.questions_after_delegation(good_one_at_a_time()) == []


# ---------------------------------------------------------------------------
# The hand-rolled YAML readers
#
# They stand in for PyYAML, which the image does not have, so they get their
# own coverage rather than being trusted.
# ---------------------------------------------------------------------------

CONFIG_YAML = """name: app
command: spektacular
agent: claude
spec:
    provider: file
    config:
        directory: .spektacular/specs
repos:
    - name: app
      location: .
    - name: harbour
      location: ../../srv/harbour/.spektacular
"""

REPO_YAML = """description: Tide-gate scheduling for dockside cranes
role: application
tags:
    - typescript
    - scheduling
source:
    provider: file
    config:
        location: ..
"""


def test_config_repo_entries_reads_the_registry():
    entries = rw.config_repo_entries(CONFIG_YAML)
    assert entries == [
        {"name": "app", "location": "."},
        {"name": "harbour", "location": "../../srv/harbour/.spektacular"},
    ]


def test_config_repo_entries_ignores_other_nested_blocks():
    """`spec:` also has a nested `config:` with a `location`-shaped key; only
    the repos block may contribute entries."""
    assert len(rw.config_repo_entries(CONFIG_YAML)) == 2


def test_yaml_top_level_reads_scalars_and_lists():
    parsed = rw.yaml_top_level(REPO_YAML)
    assert parsed["description"] == "Tide-gate scheduling for dockside cranes"
    assert parsed["role"] == "application"
    assert parsed["tags"] == ["typescript", "scheduling"]


def test_yaml_top_level_rejoins_a_wrapped_scalar():
    wrapped = (
        "description: Tide-gate scheduling for dockside cranes, planning crane\n"
        "    movements against tide windows\n"
        "role: application\n"
    )
    parsed = rw.yaml_top_level(wrapped)
    assert parsed["description"] == (
        "Tide-gate scheduling for dockside cranes, planning crane movements "
        "against tide windows"
    )
    assert parsed["role"] == "application"


def test_yaml_top_level_reports_a_missing_description_as_empty():
    parsed = rw.yaml_top_level("role: application\n")
    assert parsed.get("description", "") == ""
