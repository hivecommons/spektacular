#!/usr/bin/env python3
"""No-model runtime checks against the installed omp release.

Build Spektacular, then: python3 tests/omp_smoke.py /absolute/path/to/spektacular
Requires omp on PATH. Uses temporary projects and isolated user settings.
"""
import json
import os
import re
from pathlib import Path
import selectors
import subprocess
import sys
import tempfile
import time

SKILLS = ["spek-new", "spek-plan", "spek-implement", "spek-knowledge", "spek-manage-repos", "spek-design"]
CONFLICTS = [".github/copilot-instructions.md", ".claude/CLAUDE.md", ".gemini/GEMINI.md", ".agents/AGENTS.md", ".agent/AGENTS.md", ".omp/AGENTS.md"]


class RPC:
    def __init__(self, project, user):
        env = dict(os.environ, PI_CODING_AGENT_DIR=str(user), ANTHROPIC_API_KEY="placeholder-not-a-key", ANTHROPIC_BASE_URL="http://127.0.0.1:9")
        self.process = subprocess.Popen(["omp", "--mode", "rpc", "--no-session", "--no-ui", "--provider", "anthropic", "--model", "claude-sonnet-4-6"], cwd=project, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        self.selector = selectors.DefaultSelector()
        self.selector.register(self.process.stdout, selectors.EVENT_READ)
        self.buffer = b""
        self.serial = 0

    def receive(self):
        deadline = time.monotonic() + 30
        while b"\n" not in self.buffer:
            if not self.selector.select(max(0, deadline - time.monotonic())):
                raise TimeoutError("omp RPC did not respond")
            chunk = os.read(self.process.stdout.fileno(), 65536)
            if not chunk:
                raise RuntimeError("omp exited before responding")
            self.buffer += chunk
        line, self.buffer = self.buffer.split(b"\n", 1)
        return json.loads(line)

    def send(self, kind, **payload):
        self.serial += 1
        ident = str(self.serial)
        self.process.stdin.write((json.dumps(dict(id=ident, type=kind, **payload)) + "\n").encode())
        self.process.stdin.flush()
        return ident

    def request(self, kind, **payload):
        ident = self.send(kind, **payload)
        while True:
            frame = self.receive()
            if frame.get("type") == "response" and frame.get("id") == ident:
                assert frame.get("success", True), frame
                return frame.get("data")

    def close(self):
        self.process.terminate()
        try:
            self.process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait()
        self.selector.close()
        self.process.stdin.close()
        self.process.stdout.close()


def init(binary, project, agent):
    subprocess.run([binary, "init", agent], cwd=project, check=True, stdout=subprocess.DEVNULL)


def check(project, user, marker=None, arguments=False):
    rpc = RPC(project, user)
    try:
        while rpc.receive().get("type") != "ready":
            pass
        prompt = "\n".join(rpc.request("get_state")["systemPrompt"])
        for rule in (project / ".omp/rules").glob("spek-*.md"):
            text = rule.read_text().split("---\n", 2)[2].strip()
            assert prompt.count(text) == 1, (project, rule.name, prompt.count(text))
        if marker:
            assert marker in prompt, (project, "project instructions missing")
        commands = rpc.request("get_available_commands")["commands"]
        names = [command["name"] for command in commands]
        for skill in SKILLS:
            assert names.count(skill) == 1, (project, skill, names)
            assert names.count("skill:" + skill) == 1, (project, skill, names)
        if arguments:
            for skill in SKILLS:
                text = 'add user auth  to the admin pages "quoted" $value\nwith another line  '
                rpc.send("prompt", message="/" + skill + " " + text)
                # The user message is recorded before any model request. The
                # address above is a closed local port; no credentials or model
                # service are involved. Wait for the recorded message event.
                while True:
                    event = rpc.receive()
                    if event.get("type") == "message_start" and event.get("message", {}).get("role") == "user":
                        content = event["message"]["content"]
                        body = content if isinstance(content, str) else "\n".join(c.get("text", "") for c in content)
                        assert body == "Run the `" + skill + "` skill.\n\n" + text, body
                        break
                rpc.request("abort")
    finally:
        rpc.close()


def main():
    binary = str(Path(sys.argv[1]).resolve())
    print(subprocess.check_output(["omp", "--version"], text=True).strip(), flush=True)
    with tempfile.TemporaryDirectory(prefix="spektacular-omp-") as tmp:
        root = Path(tmp)
        user = root / "user"
        user.mkdir()
        cases = 0
        def project(name):
            path = root / name
            path.mkdir()
            return path
        plain = project("plain")
        init(binary, plain, "omp")
        check(plain, user, arguments=True)
        init(binary, plain, "omp")
        check(plain, user)
        cases += 2
        # Include project-authored AGENTS sections interleaved with ours.
        agents = plain / "AGENTS.md"
        agents.write_text(agents.read_text().replace("## Memory & Context", "## Private rules\nKeep private instructions.\n\n## Memory & Context"))
        init(binary, plain, "omp")
        check(plain, user, "Keep private instructions.")
        cases += 1
        # Migration must refresh all owned omp files, including native handlers,
        # and keep user files byte-for-byte. Compare with this version's init.
        custom = plain / ".omp/rules/custom.md"
        custom.write_text("---\nalwaysApply: true\n---\nMy private rule.\n")
        expected = {path.relative_to(plain): path.read_bytes() for path in (plain / ".omp").rglob("*") if path.is_file()}
        for rel in expected:
            if rel != Path(".omp/rules/custom.md"):
                (plain / rel).write_text("stale workaround\n")
        config = plain / ".spektacular/config.yaml"
        config.write_text(re.sub(r"(?m)^skills_version:.*$", "skills_version: 0.1.0", config.read_text()))
        subprocess.run([binary, "migrate"], cwd=plain, check=True, stdout=subprocess.DEVNULL)
        for rel, content in expected.items():
            assert (plain / rel).read_bytes() == content, rel
        check(plain, user, "Keep private instructions.")
        cases += 1
        for index, conflict in enumerate(CONFLICTS):
            for when in ("before", "after"):
                path = project(f"conflict-{index}-{when}")
                target = path / conflict
                marker = f"Project instruction marker {index} {when}."
                if when == "after":
                    init(binary, path, "omp")
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(marker)
                if when == "before":
                    init(binary, path, "omp")
                check(path, user, marker)
                assert target.read_text() == marker
                cases += 1
        for agent in ("claude", "bob", "codex"):
            for order in ((agent, "omp"), ("omp", agent)):
                path = project("shared-" + "-".join(order))
                for selected in order:
                    init(binary, path, selected)
                check(path, user)
                cases += 1
        (user / "config.yml").write_text("disabledProviders: [claude, codex, agents]\n")
        check(plain, user, "Keep private instructions.")
        cases += 1
        print(f"PASS: {cases} instruction/skill/command cases; all six command inputs preserved")


if __name__ == "__main__":
    main()
