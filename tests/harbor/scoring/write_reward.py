"""Turn a verifier's pytest JUnit XML into harbor's named rewards.

Each suite's test.sh used to write a single reward of 1 only when every check
passed, so a run that passed 30 of 54 checks scored the same as one that passed
none. This script reads the JUnit XML pytest wrote and writes
/logs/verifier/reward.json, which harbor prefers over reward.txt when both
exist and reports key by key:

    reward          fraction of checks that passed, 0..1
    all_passed      1 when no check failed, else 0 (the old reward.txt value)
    checks_passed   number of checks that passed
    checks_total    number of checks that ran
    group/<name>    fraction passed within one test class, e.g.
                    group/one_question_per_exchange for
                    TestOneQuestionPerExchange

Skipped checks are left out of every count: a check skipped because it does
not apply to the scenario is neither a pass nor a failure. A collection or
fixture error counts as a failed check.

Usage, from a suite's test.sh:

    python3 /tests/write_reward.py /logs/verifier/junit.xml /logs/verifier/reward.json

The canonical copy lives in tests/harbor/scoring/; the Makefile copies it into
each suite's tests/ directory before a run, so the verifier image finds it at
/tests/write_reward.py. test.sh still writes reward.txt first, so a run whose
JUnit XML is missing or unreadable falls back to the old pass/fail reward.
"""

import json
import re
import sys
import xml.etree.ElementTree as ET
from collections import defaultdict


def group_name(classname: str) -> str | None:
    """The check group a test belongs to: its test class, in snake case.

    pytest's JUnit classname is the dotted module path, with the class name
    appended when the test is a method. A module-level test function has no
    class, so it belongs to no group and counts only towards the overall
    reward.
    """
    last = classname.rsplit(".", 1)[-1]
    if not last.startswith("Test"):
        return None
    name = last[len("Test"):] or last
    return re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "_", name).lower()


def outcome(testcase: ET.Element) -> str:
    """passed, failed or skipped, from the child elements pytest writes."""
    tags = {child.tag for child in testcase}
    if "failure" in tags or "error" in tags:
        return "failed"
    if "skipped" in tags:
        return "skipped"
    return "passed"


def rewards_from_junit(xml_text: str) -> dict[str, float | int]:
    root = ET.fromstring(xml_text)
    totals: dict[str | None, list[int]] = defaultdict(lambda: [0, 0])
    for testcase in root.iter("testcase"):
        result = outcome(testcase)
        if result == "skipped":
            continue
        passed = 1 if result == "passed" else 0
        group = group_name(testcase.get("classname", ""))
        for key in {None, group}:
            totals[key][0] += passed
            totals[key][1] += 1

    passed, total = totals[None]
    rewards: dict[str, float | int] = {
        "reward": passed / total if total else 0.0,
        "all_passed": 1 if total and passed == total else 0,
        "checks_passed": passed,
        "checks_total": total,
    }
    for group in sorted(g for g in totals if g is not None):
        group_passed, group_total = totals[group]
        rewards[f"group/{group}"] = group_passed / group_total
    return rewards


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print(f"usage: {argv[0]} <junit.xml> <reward.json>", file=sys.stderr)
        return 2
    with open(argv[1], encoding="utf-8") as f:
        rewards = rewards_from_junit(f.read())
    with open(argv[2], "w", encoding="utf-8") as f:
        json.dump(rewards, f, indent=2)
        f.write("\n")
    print(
        f"score {rewards['reward']:.3f} "
        f"({rewards['checks_passed']}/{rewards['checks_total']} checks passed)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
