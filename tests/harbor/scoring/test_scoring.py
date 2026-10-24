"""Tests for write_reward.py and report.py.

Neither script needs a container or an agent, so these run locally:

    python3 -m pytest tests/harbor/scoring -q    # or: make harbor-scoring-test

The JUnit XML comes from a real pytest run over a small synthetic suite, so a
change in what pytest writes shows up here rather than in a harbor run.
"""

import json
import subprocess
import sys
import textwrap
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent))

import report  # noqa: E402
import write_reward  # noqa: E402


SYNTHETIC_SUITE = textwrap.dedent(
    """
    import pytest

    @pytest.fixture
    def broken():
        raise RuntimeError("fixture failed")

    class TestOneQuestionPerExchange:
        def test_a(self):
            pass

        def test_b(self):
            assert False

    class TestNoInternalVocabulary:
        @pytest.mark.parametrize("word", ["step", "fsm", "goto"])
        def test_word(self, word):
            assert word != "goto"

        @pytest.mark.skip(reason="not this scenario")
        def test_skipped(self):
            pass

        def test_errors(self, broken):
            pass

    def test_module_level():
        pass
    """
)


def run_pytest_junit(tmp_path: Path, source: str) -> str:
    suite = tmp_path / "test_synthetic.py"
    suite.write_text(source)
    junit = tmp_path / "junit.xml"
    subprocess.run(
        [sys.executable, "-m", "pytest", "-q", "-p", "no:cacheprovider", str(suite), f"--junitxml={junit}"],
        cwd=tmp_path,
        capture_output=True,
    )
    return junit.read_text()


# ---------------------------------------------------------------------------
# write_reward.py
# ---------------------------------------------------------------------------


def test_group_name():
    assert write_reward.group_name("test_repo_workflow.TestOneQuestionPerExchange") == "one_question_per_exchange"
    assert write_reward.group_name("test_x.TestNoInternalVocabulary") == "no_internal_vocabulary"
    assert write_reward.group_name("test_x.TestPlanMdContent") == "plan_md_content"
    assert write_reward.group_name("test_implement_workflow") is None


def test_rewards_count_fraction_of_checks(tmp_path):
    rewards = write_reward.rewards_from_junit(run_pytest_junit(tmp_path, SYNTHETIC_SUITE))

    # 7 checks ran (the skip is left out): test_a, test_word[step],
    # test_word[fsm] and test_module_level passed; test_b, test_word[goto] and
    # the fixture error failed.
    assert rewards["checks_total"] == 7
    assert rewards["checks_passed"] == 4
    assert rewards["reward"] == pytest.approx(4 / 7)
    assert rewards["all_passed"] == 0
    assert rewards["group/one_question_per_exchange"] == pytest.approx(1 / 2)
    assert rewards["group/no_internal_vocabulary"] == pytest.approx(2 / 4)
    assert not any("module_level" in key or key == "group/" for key in rewards)


def test_rewards_all_passed(tmp_path):
    rewards = write_reward.rewards_from_junit(
        run_pytest_junit(tmp_path, "class TestA:\n    def test_x(self):\n        pass\n")
    )
    assert rewards == {
        "reward": 1.0,
        "all_passed": 1,
        "checks_passed": 1,
        "checks_total": 1,
        "group/a": 1.0,
    }


def test_collection_error_scores_zero(tmp_path):
    rewards = write_reward.rewards_from_junit(run_pytest_junit(tmp_path, "import no_such_module\n"))
    assert rewards["reward"] == 0.0
    assert rewards["all_passed"] == 0


def test_no_checks_scores_zero():
    rewards = write_reward.rewards_from_junit("<testsuites><testsuite/></testsuites>")
    assert rewards["reward"] == 0.0
    assert rewards["all_passed"] == 0
    assert rewards["checks_total"] == 0


def test_main_writes_reward_json(tmp_path):
    junit = tmp_path / "junit.xml"
    junit.write_text(run_pytest_junit(tmp_path, SYNTHETIC_SUITE))
    out = tmp_path / "reward.json"
    assert write_reward.main(["write_reward.py", str(junit), str(out)]) == 0
    rewards = json.loads(out.read_text())
    # harbor rejects anything but finite numbers.
    assert all(isinstance(v, (int, float)) for v in rewards.values())
    assert rewards["checks_passed"] == 4


def test_main_fails_without_junit(tmp_path):
    with pytest.raises(FileNotFoundError):
        write_reward.main(["write_reward.py", str(tmp_path / "missing.xml"), str(tmp_path / "r.json")])


# ---------------------------------------------------------------------------
# report.py
# ---------------------------------------------------------------------------


def claude_log(cost: float, turns: int) -> str:
    events = [
        {"type": "system", "subtype": "init"},
        {"type": "assistant", "message": {"content": []}},
        {
            "type": "result",
            "subtype": "success",
            "total_cost_usd": cost,
            "num_turns": turns,
            "duration_ms": 1000,
            "usage": {
                "input_tokens": 100,
                "cache_creation_input_tokens": 200,
                "cache_read_input_tokens": 700,
                "output_tokens": 50,
            },
        },
    ]
    return "not json\n" + "\n".join(json.dumps(e) for e in events) + "\n"


CODEX_LOG = "\n".join(
    json.dumps(e)
    for e in [
        {"type": "thread.started", "thread_id": "t"},
        {"type": "turn.completed", "usage": {"input_tokens": 1000, "cached_input_tokens": 800, "output_tokens": 40}},
        {"type": "turn.completed", "usage": {"input_tokens": 500, "cached_input_tokens": 0, "output_tokens": 10}},
    ]
)


def write_trial(
    job: Path,
    name: str,
    *,
    task: str = "spec-workflow-claude",
    agent: str = "claude-code",
    rewards: dict | None,
    log_name: str | None = "claude-code.txt",
    log: str | None = None,
    agent_result: dict | None = None,
    minutes: int = 10,
) -> None:
    trial = job / name
    (trial / "agent").mkdir(parents=True)
    if log_name and log is not None:
        (trial / "agent" / log_name).write_text(log)
    result = {
        "task_name": task,
        "trial_name": name,
        "agent_info": {"name": agent, "version": "1", "model_info": {"name": "claude-sonnet-4-6"}},
        "agent_result": agent_result,
        "verifier_result": None if rewards is None else {"rewards": rewards},
        "agent_execution": {
            "started_at": "2026-10-24T10:00:00+00:00",
            "finished_at": f"2026-10-24T10:{minutes:02d}:00+00:00",
        },
    }
    (trial / "result.json").write_text(json.dumps(result))


@pytest.fixture
def job(tmp_path):
    job = tmp_path / "jobs" / "2026-10-24__10-00-00"
    job.mkdir(parents=True)
    # Job-level result.json: not a trial, must be ignored.
    (job / "result.json").write_text(json.dumps({"id": "job", "stats": {}}))
    write_trial(job, "spec__a", rewards={"reward": 1.0, "all_passed": 1, "group/x": 1.0}, log=claude_log(2.0, 10))
    write_trial(job, "spec__b", rewards={"reward": 0.5, "all_passed": 0, "group/x": 0.5}, log=claude_log(4.0, 20), minutes=20)
    write_trial(job, "spec__c", rewards={"reward": 1.0, "all_passed": 1, "group/x": 1.0}, log=claude_log(3.0, 30))
    # Crashed before the verifier ran: no rewards, no log.
    write_trial(job, "spec__d", rewards=None, log=None)
    return job


def test_report_summarises_runs(job):
    rows = [report.summary_row(s, 3) for s in report.summarise(report.load_trials([job]))]
    assert len(rows) == 1
    row = rows[0]
    assert row["runs"] == 4
    assert row["pass_rate"] == pytest.approx(2 / 4)
    # C(2,3) / C(4,3) = 0
    assert row["pass^3"] == 0
    assert row["score"] == pytest.approx((1 + 0.5 + 1 + 0) / 4)
    assert row["cost_usd"] == pytest.approx(3.0)
    assert row["turns"] == pytest.approx(20)
    assert row["input_tokens"] == pytest.approx(1000)
    assert row["output_tokens"] == pytest.approx(50)
    assert row["minutes"] == pytest.approx((10 + 20 + 10 + 10) / 4)
    # The crashed run scores 0 on every group too.
    assert row["groups"] == {"x": pytest.approx((1 + 0.5 + 1 + 0) / 4)}


def test_pass_hat_k():
    assert report.pass_hat_k(5, 5, 3) == 1
    assert report.pass_hat_k(5, 4, 1) == pytest.approx(0.8)
    assert report.pass_hat_k(5, 4, 2) == pytest.approx(6 / 10)
    assert report.pass_hat_k(2, 2, 3) is None


def test_codex_usage_sums_turns_and_has_no_cost(tmp_path):
    job = tmp_path / "job"
    write_trial(job, "t", agent="codex", rewards={"reward": 1.0}, log_name="codex.txt", log=CODEX_LOG)
    (trial,) = report.load_trials([job])
    assert trial.usage.input_tokens == 1500
    assert trial.usage.output_tokens == 50
    assert trial.usage.cost_usd is None
    assert trial.usage.turns is None


def test_falls_back_to_harbor_agent_result(tmp_path):
    job = tmp_path / "job"
    write_trial(
        job,
        "t",
        rewards={"reward": 1.0},
        log=None,
        agent_result={"n_input_tokens": 10, "n_cache_tokens": 5, "n_output_tokens": 3, "cost_usd": 0.25},
    )
    (trial,) = report.load_trials([job])
    assert trial.usage.cost_usd == 0.25
    assert trial.usage.input_tokens == 15
    assert trial.usage.output_tokens == 3


def test_pass_fail_runs_from_before_reward_json(tmp_path):
    job = tmp_path / "job"
    write_trial(job, "pass", rewards={"reward": 1.0}, log=None)
    write_trial(job, "fail", rewards={"reward": 0.0}, log=None)
    rows = [report.summary_row(s, 2) for s in report.summarise(report.load_trials([job]))]
    assert rows[0]["pass_rate"] == 0.5
    assert rows[0]["groups"] == {}


def test_group_skipped_in_a_run_is_left_out_of_its_mean(tmp_path):
    job = tmp_path / "job"
    write_trial(job, "a", rewards={"reward": 1.0, "all_passed": 1, "checks_total": 2, "group/x": 1.0}, log=None)
    write_trial(job, "b", rewards={"reward": 1.0, "all_passed": 1, "checks_total": 1}, log=None)
    rows = [report.summary_row(s, 2) for s in report.summarise(report.load_trials([job]))]
    assert rows[0]["groups"] == {"x": 1.0}


def test_parent_directory_and_duplicates(job):
    # Pointing at jobs/ and at the job itself finds each trial once.
    trials = report.load_trials([job.parent, job])
    assert len(trials) == 4


def test_main_table_and_json(job, capsys):
    assert report.main([str(job)]) == 0
    out = capsys.readouterr().out
    assert "spec-workflow-claude" in out
    assert "$3.00" in out
    assert "score by check group" in out

    assert report.main(["--json", "--k", "2", str(job)]) == 0
    rows = json.loads(capsys.readouterr().out)
    assert rows[0]["pass^2"] == pytest.approx(1 / 6)


def test_main_no_trials(tmp_path, capsys):
    assert report.main([str(tmp_path)]) == 1
    assert "no harbor trials" in capsys.readouterr().err
