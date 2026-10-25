#!/usr/bin/env python3
"""Sets log.level in a Spektacular project's config.yaml.

A suite's seeded project records the agent's run at the default level
(standard); its seed switches logging off while it seeds and back to the
default at the end. A verifier that runs `spektacular` itself switches the
project's logging off first, so its own calls stay out of the logs it
asserts against: nothing else keeps them out, since the level comes from the
project's configuration alone.

Only the `level:` line under the top-level `log:` block is rewritten; every
other byte of the file is kept. A config without that line is an error, so a
change of the settings format fails loudly rather than leaving logging on.

    python3 loglevel.py <config.yaml> <off|standard|verbose>
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

LEVELS = ("off", "standard", "verbose")


def set_log_level(config: Path, level: str) -> None:
    """Rewrite the log.level line of config to level."""
    if level not in LEVELS:
        raise ValueError(f"log level {level!r} is not one of {', '.join(LEVELS)}")
    lines = Path(config).read_text().split("\n")
    in_log = False
    for i, line in enumerate(lines):
        if re.match(r"\S", line):
            in_log = line.rstrip() == "log:"
        elif in_log and re.match(r"^ +level:", line):
            lines[i] = re.sub(r"level:.*", "level: " + level, line)
            break
    else:
        raise ValueError(f"{config} has no log.level line under a top-level log: block")
    Path(config).write_text("\n".join(lines))


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    set_log_level(Path(sys.argv[1]), sys.argv[2])
