#!/usr/bin/env python3
"""Offline discovery smoke test against an installed Copilot CLI.

Build Spektacular first, then run:
  python3 tests/copilot-discovery.py /absolute/path/to/spektacular

No model call or credentials are needed. Prints the tested Copilot version.
This does not test ACP commands, assembled model instructions, or a workflow.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

SPEK = str(Path(sys.argv[1]).resolve())
SKILLS = {"spek-new", "spek-plan", "spek-implement", "spek-knowledge",
          "spek-manage-repos", "spek-design"}
print(subprocess.check_output(["copilot", "--version"], text=True).strip())

with tempfile.TemporaryDirectory() as temporary:
    root = Path(temporary)
    env = dict(os.environ, COPILOT_HOME=str(root / "home"), COPILOT_OFFLINE="true")
    for other in (None, "claude", "bob", "codex"):
        for copilot_first in (False, True):
            project = root / f"{other}-{copilot_first}"
            project.mkdir()
            agents = ["copilot"] if other is None else (
                ["copilot", other] if copilot_first else [other, "copilot"])
            for agent in agents:
                subprocess.run([SPEK, "init", agent], cwd=project, env=env,
                               stdout=subprocess.DEVNULL, check=True)
            # Competing versions must not displace Copilot's copy.
            for folder in (".claude/skills", ".agents/skills"):
                skill = project / folder / "spek-new/SKILL.md"
                if skill.exists():
                    skill.write_text(skill.read_text() + "\nCOMPETING COPY\n")
            found = json.loads(subprocess.check_output(
                ["copilot", "skill", "list", "--json"], cwd=project, env=env, text=True))
            workflows = [s for s in found if s["name"] in SKILLS]
            assert len(workflows) == 8, workflows
            assert {s["name"] for s in workflows} == SKILLS
            for skill in workflows:
                assert Path(skill["path"]) == project / ".github/skills" / skill["name"], skill
            instructions = json.loads(subprocess.check_output(
                ["copilot", "instruction", "list", "--json"], cwd=project, env=env, text=True))
            assert any(i["sourcePath"] == "AGENTS.md" for i in instructions), instructions
            print(f"PASS: {agents}: eight native skills, AGENTS.md discovered")
