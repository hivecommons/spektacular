# Context: 20260301000000-todo-count

## Per-Task Technical Notes

### Task: Add the count command

- `todo/cli.py`: a `_count(args)` handler printing `len(load())`, registered as the `count` subparser beside `list`.
- `tests/test_count.py`: an empty list counts `0`; two added todos count `2`.
- `README.md`: a line under `## Usage` in the same shape as the others, such as `python3 -m todo count            # todo count: print how many todos there are`.
