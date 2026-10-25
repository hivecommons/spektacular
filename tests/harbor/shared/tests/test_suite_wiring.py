"""Static self-check for how the metrics recorder is wired into every suite.

Every harbor suite renders into tests/harbor/.build/, copies the shared
helpers (tests/harbor/shared/*.py, metrics.py among them) into its rendered
tests directory and writes build.json there through the Makefile's
`write_build_info` macro. Its test.sh runs pytest with JUnit XML, writes the
reward from pytest's exit status alone, and only then runs the recorder,
ignoring its failure. Its image installs python3 so the recorder can run.

Everything here only reads the Makefile, each suite's tests/test.sh and its
environment/Dockerfile as text, against hand-written expectations per suite.
Nothing renders a suite, runs make or runs harbor: a render target ends in a
real, costly harbor trial.

This file never runs in a harbor container. It lives under shared/tests/ so
the Makefile, which copies shared/*.py into each image, leaves it out. Run it
locally with

    uvx --with pytest pytest -p no:cacheprovider --import-mode=importlib tests/harbor/shared/tests/test_suite_wiring.py
"""

import re
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[4]
HARBOR = ROOT / "tests" / "harbor"
MAKEFILE = ROOT / "Makefile"

SHARED_CP = "cp tests/harbor/shared/*.py"
PYTEST_JUNIT = "--junitxml=/logs/verifier/junit.xml"
RECORDER = "python3 /tests/metrics.py || true"
REWARD_BLOCK = [
    "if [ $? -eq 0 ]; then",
    "echo 1 > /logs/verifier/reward.txt",
    "else",
    "echo 0 > /logs/verifier/reward.txt",
    "fi",
]

# suite -> (render target, rendered tests dir as the recipe spells it,
#           scenario argument to write_build_info as the recipe spells it)
SUITES = {
    "spec-workflow": (
        "_harbor-test-spec",
        "tests/harbor/.build/spec-workflow-$(AGENT)/tests",
        "",
    ),
    "repo-workflow": (
        "_harbor-test-repo",
        "tests/harbor/.build/repo-workflow-$(SCENARIO)/tests",
        "$(SCENARIO)",
    ),
    "plan-workflow": (
        "_harbor-test-plan",
        "tests/harbor/.build/plan-workflow-$(AGENT)/tests",
        "",
    ),
    "implement-workflow": (
        "harbor-test-implement",
        "tests/harbor/.build/implement-workflow-$(AGENT)/tests",
        "",
    ),
    "epic-workflow": (
        "_harbor-test-epic",
        "tests/harbor/.build/epic-workflow-$(AGENT)/tests",
        "",
    ),
    "standing-rules": (
        "_harbor-test-standing-rules",
        "tests/harbor/.build/standing-rules-$(AGENT)/tests",
        "",
    ),
    "implement-epic-workflow": (
        "_harbor-test-implement-epic",
        "$(IMPLEMENT_EPIC_BUILD)/tests",
        "$(patsubst -%,%,$(VARIANT))",
    ),
    "goal-check-workflow": (
        "_harbor-test-goal-check",
        "$(GOAL_CHECK_BUILD)/tests",
        "$(SCENARIO)",
    ),
    "background-child": (
        "_harbor-test-background-child",
        "tests/harbor/.build/background-child-$(AGENT)/tests",
        "",
    ),
}

# Make variables a recipe above names its build directory through.
BUILD_VARS = {
    "IMPLEMENT_EPIC_BUILD": "tests/harbor/.build/implement-epic-workflow-$(AGENT)$(VARIANT)",
    "GOAL_CHECK_BUILD": "tests/harbor/.build/goal-check-workflow-$(AGENT)-$(SCENARIO)",
}


def makefile_lines():
    return MAKEFILE.read_text().splitlines()


def recipe(target):
    """The recipe of `target` as logical commands, continuations joined.

    The recipe is the lines after the `target:` rule line up to the next line
    that is neither indented nor blank. A target-specific variable line
    (`target: VAR = value`) is not the rule line.
    """
    lines = makefile_lines()
    rule = re.compile(rf"^{re.escape(target)}:(?!.*=)")
    starts = [i for i, line in enumerate(lines) if rule.match(line)]
    assert len(starts) == 1, f"expected one rule line for {target}, found {len(starts)}"
    body = []
    for line in lines[starts[0] + 1:]:
        if line.strip() and not line[0].isspace():
            break
        body.append(line)
    commands, current = [], ""
    for line in body:
        if not line.strip():
            continue
        current += " " + line.strip()
        if current.endswith("\\"):
            current = current[:-1]
            continue
        commands.append(" ".join(current.split()))
        current = ""
    if current:
        commands.append(" ".join(current.split()))
    return commands


def build_info_call(tests_dir, suite, scenario):
    return f"$(call write_build_info,{tests_dir},{suite},{scenario})"


# Acceptance 1: rendering a suite leaves the shared helpers and build info in
# its tests directory.


def test_repo_root_is_the_checkout():
    assert MAKEFILE.is_file()
    assert (HARBOR / "shared" / "metrics.py").is_file()
    assert sorted(SUITES) == sorted(
        p.name for p in HARBOR.iterdir() if (p / "tests" / "test.sh").is_file()
    )


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_render_target_copies_shared_helpers(suite):
    target, tests_dir, _ = SUITES[suite]
    cmds = recipe(target)
    assert f"{SHARED_CP} {tests_dir}/" in cmds


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_render_target_writes_build_info_for_its_suite(suite):
    target, tests_dir, scenario = SUITES[suite]
    cmds = recipe(target)
    calls = [c for c in cmds if "write_build_info" in c]
    assert calls == [build_info_call(tests_dir, suite, scenario)]
    # build.json lands after the tests directory is in place and after any
    # rm -rf of the build, so nothing later removes it.
    at = cmds.index(calls[0])
    assert cmds.index(f"{SHARED_CP} {tests_dir}/") < at
    assert not any("rm -rf" in c for c in cmds[at + 1:])


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_render_target_tests_dir_is_inside_the_suites_build(suite):
    _, tests_dir, _ = SUITES[suite]
    for name, value in BUILD_VARS.items():
        tests_dir = tests_dir.replace(f"$({name})", value)
    assert tests_dir.startswith(f"tests/harbor/.build/{suite}-")
    assert tests_dir.endswith("/tests")


def test_build_vars_match_the_makefile():
    text = MAKEFILE.read_text()
    for name, value in BUILD_VARS.items():
        assert re.search(rf"^{name} = {re.escape(value)}$", text, re.M), name


def test_write_build_info_writes_build_json_with_suite_and_scenario():
    text = MAKEFILE.read_text()
    m = re.search(r"^define write_build_info\n(.*?)^endef$", text, re.M | re.S)
    assert m, "write_build_info macro is missing"
    body = m.group(1)
    assert "> $(1)/build.json" in body
    assert "'$(2)'" in body
    assert "$(if $(3),\"$(3)\",null)" in body
    assert "$(AGENT)" in body


def test_shared_copy_is_never_recursive():
    # The self-checks under shared/tests/ must stay out of every image.
    copies = [
        line
        for line in makefile_lines()
        if "tests/harbor/shared" in line and line.startswith("\t") and line.strip().startswith("cp ")
    ]
    assert len(copies) == len(SUITES)
    for line in copies:
        assert line.strip().startswith(f"{SHARED_CP} "), line
        assert "shared/tests" not in line, line


# Acceptance 2: the reward is 1 exactly when every check passes, and the
# recorder runs only after it, with its failure ignored.


def script_lines(suite):
    return (HARBOR / suite / "tests" / "test.sh").read_text().splitlines()


def commands(suite):
    """test.sh's non-blank, non-comment lines, stripped, with line numbers."""
    return [
        (i, line.strip())
        for i, line in enumerate(script_lines(suite))
        if line.strip() and not line.strip().startswith("#")
    ]


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_one_pytest_line_with_junit_xml(suite):
    pytests = [c for _, c in commands(suite) if re.match(r"pytest\b", c)]
    assert len(pytests) == 1, pytests
    assert PYTEST_JUNIT in pytests[0].split()
    assert "||" not in pytests[0] and "&&" not in pytests[0] and "|" not in pytests[0]


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_reward_block_directly_follows_pytest(suite):
    cmds = [c for _, c in commands(suite)]
    at = next(i for i, c in enumerate(cmds) if re.match(r"pytest\b", c))
    assert cmds[at + 1:at + 1 + len(REWARD_BLOCK)] == REWARD_BLOCK


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_recorder_is_last_and_after_the_reward(suite):
    cmds = [c for _, c in commands(suite)]
    at = next(i for i, c in enumerate(cmds) if re.match(r"pytest\b", c))
    after_block = cmds[at + 1 + len(REWARD_BLOCK):]
    assert after_block == [RECORDER]
    assert cmds.count(RECORDER) == 1


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_only_the_reward_block_writes_reward(suite):
    writers = [c for _, c in commands(suite) if "reward.txt" in c]
    assert writers == [REWARD_BLOCK[1], REWARD_BLOCK[3]]


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_script_does_not_exit_early(suite):
    # set -e, or set -o errexit, would end the script on pytest's failure
    # before the reward of 0 is written.
    for _, c in commands(suite):
        assert not re.match(r"set\s+(-\w*e|-o\s+errexit)", c), c
        assert not re.match(r"exit\b", c), c


# Acceptance 3: the implement suite runs from a rendered copy with the same
# agent as before.


def test_implement_runs_the_rendered_copy_with_claude_code():
    assert "harbor-test-implement: AGENT = claude" in makefile_lines()
    runs = [c for c in recipe("harbor-test-implement") if "harbor run" in c]
    assert len(runs) == 1
    args = runs[0].split()
    assert args[args.index("-p") + 1] == "tests/harbor/.build/implement-workflow-$(AGENT)"
    assert args[args.index("-a") + 1] == "claude-code"
    assert "-p tests/harbor/implement-workflow " not in MAKEFILE.read_text()


# Acceptance 4: every suite image can run the recorder.


@pytest.mark.parametrize("suite", sorted(SUITES))
def test_image_installs_python3(suite):
    text = (HARBOR / suite / "environment" / "Dockerfile").read_text()
    joined = re.sub(r"\\\n", " ", text)
    installs = [line for line in joined.splitlines() if "apt-get install" in line]
    assert installs, "no apt-get install in the Dockerfile"
    packages = " ".join(line.split("apt-get install", 1)[1].split("&&")[0] for line in installs)
    assert "python3" in packages.split()


# Acceptance 5: every suite's task tells the agent the session is headless and
# no one answers between its turns, and the plan suite's status wording
# matches the document status its verifier checks.

# Every suite shares these: the heading of its non-interactive section and two
# phrases its body must carry. Hand-written, never read from an instruction.
NON_INTERACTIVE_HEADING = "## You are running non-interactively"
NO_HUMAN = "no human at the other end"
NOTHING_ANSWERS = "nothing will answer"

# The plan verifier checks `document_status` with values draft and final.
PLAN_STATUS_WORDING = ("`document_status: draft`", "`document_status: final`")
STALE_PLAN_STATUS_WORDING = ("status: in-progress", "status: completed")

INSTRUCTION_SUITES = sorted(
    p.name for p in HARBOR.iterdir() if (p / "instruction.md").is_file()
)


def instruction(suite):
    return (HARBOR / suite / "instruction.md").read_text()


def section(text, heading):
    """The lines under `heading` up to the next `## ` heading, or None."""
    lines = text.splitlines()
    if heading not in lines:
        return None
    body = []
    for line in lines[lines.index(heading) + 1:]:
        if line.startswith("## "):
            break
        body.append(line)
    return "\n".join(body)


def test_every_suite_has_an_instruction():
    expected = set(SUITES) | {"goal-check-workflow", "implement-epic-workflow"}
    assert expected <= set(INSTRUCTION_SUITES)


@pytest.mark.parametrize("suite", INSTRUCTION_SUITES)
def test_instruction_says_no_one_answers(suite):
    body = section(instruction(suite), NON_INTERACTIVE_HEADING)
    assert body is not None, f"{suite} has no {NON_INTERACTIVE_HEADING!r} section"
    # Phrases may wrap across lines, and may open a sentence.
    flat = " ".join(body.split()).lower()
    assert NO_HUMAN in flat
    assert NOTHING_ANSWERS in flat


def test_plan_instruction_uses_the_verified_document_status():
    text = " ".join(instruction("plan-workflow").split())
    for wording in PLAN_STATUS_WORDING:
        assert wording in text, wording
    for wording in STALE_PLAN_STATUS_WORDING:
        assert wording not in text, wording


# --- Spektacular's logs: the default level, collected with the trial ---------

LEVEL_SETTING = re.compile(r"(?:set_log_level|level:)\s*['\"]?(\w+)")
ENVIRONMENT_FILES = ("seed/seed.sh", "Dockerfile", "config.yaml")


def environment_files(suite):
    env = HARBOR / suite / "environment"
    return [env / rel for rel in ENVIRONMENT_FILES if (env / rel).is_file()]


def log_levels_set(text: str) -> list[str]:
    """Every log level a seed script, Dockerfile or seeded config sets, but
    the `off` a seed switches to while it seeds."""
    code = "\n".join(line for line in text.splitlines() if not line.lstrip().startswith("#"))
    return [m[1] for m in LEVEL_SETTING.finditer(code) if m[1] not in ("off", "level")]


@pytest.mark.parametrize("suite", SUITES)
def test_seeded_configs_use_the_default_level(suite):
    for path in environment_files(suite):
        levels = log_levels_set(path.read_text())
        assert set(levels) <= {"standard"}, f"{path.relative_to(ROOT)} sets log level {levels}, not the default"
        assert "SPEKTACULAR_SESSION_LOG" not in path.read_text(), path


def test_a_seeded_verbose_level_is_caught():
    assert log_levels_set("set_log_level verbose\n") == ["verbose"]
    assert log_levels_set("printf 'schema: 5\\nlog:\\n    level: verbose\\n'") == ["verbose"]
    assert log_levels_set("log:\n    level: verbose\n") == ["verbose"]
    assert log_levels_set("set_log_level off\nset_log_level standard\n") == ["standard"]
    assert log_levels_set("# set_log_level rewrites log.level\n") == []


def copies_the_binary(suite) -> bool:
    return "COPY spektacular /usr/local/bin/spektacular" in (HARBOR / suite / "environment" / "Dockerfile").read_text()


@pytest.mark.parametrize("suite", SUITES)
def test_spektacular_suites_collect_their_logs(suite):
    if not copies_the_binary(suite):
        pytest.skip(f"{suite} runs no spektacular, so it has no logs")
    toml = (HARBOR / suite / "task.toml").read_text()
    assert re.search(r'\[\[artifacts\]\]\s*\nsource = "/app/\.spektacular"', toml), (
        f"{suite}'s task.toml does not collect /app/.spektacular, so its logs would not reach the trial's artifacts"
    )


def test_every_suite_but_the_probe_runs_spektacular():
    assert [s for s in SUITES if not copies_the_binary(s)] == ["background-child"]
