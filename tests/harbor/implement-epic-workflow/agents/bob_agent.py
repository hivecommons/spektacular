"""Harbor agent for IBM Bob: harbor's ACP agent, signed in as you.

Bob authenticates the same way the Claude targets do: with the login already
on the machine running harbor, not a key you configure. Before Bob starts,
the Bob Shell login under ~/.bob/settings is uploaded into the container's
own ~/.bob/settings. It is uploaded at run time, never baked into an image,
and the task artifacts collect only ~/.bob/logs, never ~/.bob/settings.

Setting BOBSHELL_API_KEY (or BOB_API_KEY) in your environment uses the key
instead and uploads nothing, for a machine with no interactive login.

Setting BOB_MAX_TURNS writes that value as `session.maxTurns` into the
container's Bob settings before Bob starts; 0 means no limit. Without it the
container's Bob runs with Bob's default of 100 turns per task, sub-agents
included, after which Bob ends the task silently.

The container's Bob may refresh the session it was given. If IBM rotates the
refresh token when it does, the login on this machine stops working and Bob
asks you to sign in again the next time you run it here.

Bob is installed and launched from the registry entry beside this file;
harbor loads the class with `-a bob_agent:BobAgent` and this directory on
PYTHONPATH.
"""

from __future__ import annotations

import json
import os
import shlex
from pathlib import Path

from harbor.agents.installed.acp import AcpAgent
from harbor.environments.base import BaseEnvironment

REGISTRY_ENTRY = Path(__file__).with_name("bob.json")
LOGIN_FILE = Path.home() / ".bob" / "settings" / "auth-secrets.json"
API_KEY_ENV = ("BOBSHELL_API_KEY", "BOB_API_KEY")


class BobAgent(AcpAgent):
    def __init__(self, *args, **kwargs):
        kwargs.setdefault("registry_entry_path", str(REGISTRY_ENTRY))
        super().__init__(*args, **kwargs)

    async def setup(self, environment: BaseEnvironment) -> None:
        await super().setup(environment)
        await self._apply_turn_limit(environment)
        if any(os.environ.get(name) for name in API_KEY_ENV):
            return
        if not LOGIN_FILE.is_file():
            raise RuntimeError(
                f"no Bob login at {LOGIN_FILE}: run `bob` on this machine and sign "
                "in, or set BOBSHELL_API_KEY"
            )

        home = await self._agent_home(environment)
        settings = f"{home}/.bob/settings"
        target = f"{settings}/auth-secrets.json"
        await self.exec_as_agent(
            environment,
            f"mkdir -p {shlex.quote(settings)} && chmod 700 {shlex.quote(home)}/.bob {shlex.quote(settings)}",
        )
        await environment.upload_file(LOGIN_FILE, target)
        # The upload lands as root; hand it to the agent user, readable by it alone.
        await self.exec_as_root(
            environment,
            f"chown \"$(stat -c %u:%g {shlex.quote(home)})\" {shlex.quote(target)} "
            f"&& chmod 600 {shlex.quote(target)}",
        )

    async def _apply_turn_limit(self, environment: BaseEnvironment) -> None:
        raw = os.environ.get("BOB_MAX_TURNS")
        if raw is None:
            return
        try:
            turns = int(raw)
        except ValueError:
            raise RuntimeError(f"BOB_MAX_TURNS must be a whole number, not {raw!r}") from None
        home = await self._agent_home(environment)
        settings = f"{home}/.bob/settings"
        body = json.dumps({"session": {"maxTurns": turns}})
        result = await self.exec_as_agent(
            environment,
            f"mkdir -p {shlex.quote(settings)} && chmod 700 {shlex.quote(home)}/.bob {shlex.quote(settings)} "
            f"&& printf %s {shlex.quote(body)} > {shlex.quote(settings)}/settings.json",
        )
        if result.return_code != 0:
            raise RuntimeError(f"could not write Bob's turn limit: {result.stderr}")

    async def _agent_home(self, environment: BaseEnvironment) -> str:
        result = await self.exec_as_agent(environment, 'printf %s "$HOME"')
        home = (result.stdout or "").strip()
        if result.return_code != 0 or not home:
            raise RuntimeError("could not resolve the agent user's home directory in the container")
        return home
