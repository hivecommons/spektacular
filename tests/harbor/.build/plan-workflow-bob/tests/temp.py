from test_plan_workflow import parse_sections
import os

with open(
    "/home/nicj/code/github.com/jumppad-labs/spektacular/tests/harbor/jobs/2026-04-13__17-31-23/plan-workflow__W6tgdNf/artifacts/spektacular/plans/user-auth/plan.md",
    "r",
) as f:
    doc = f.read()
    sections = parse_sections(doc)
    print(sections.keys())
