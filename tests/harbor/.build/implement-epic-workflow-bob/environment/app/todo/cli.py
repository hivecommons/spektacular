"""Entry point for `python -m todo`.

Only the argument parser exists so far: there are no commands yet and todos
are not stored anywhere.
"""

import argparse


def build_parser() -> argparse.ArgumentParser:
    return argparse.ArgumentParser(prog="todo")


def main(argv: list[str] | None = None) -> int:
    build_parser().parse_args(argv)
    return 0
