# Context: 20260201000003-commands

## Task: Add the add command

`main(argv=None)` builds the parser, parses `argv`, and returns `args.func(args)` when a subcommand was given, otherwise prints the usage and returns 0.

## Task: Add the list command

The handler prints `format_todos(storage.load())` one line at a time.

## Task: Add the done command

`done` takes `id` as `type=int`, finds the todo, sets `done` to true and saves.

## Task: Reject an unknown id

The handler returns 1 after `print(f"No todo {id}", file=sys.stderr)` when no todo matches.

## Task: Describe each command in help

`sub.add_parser("add", help="add a todo")` and likewise for `list` and `done`.

## Task: Document usage in the README

One fenced example per command, using `python3 -m todo`.
