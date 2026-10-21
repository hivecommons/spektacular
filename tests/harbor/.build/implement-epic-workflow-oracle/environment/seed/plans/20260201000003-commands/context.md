# Context: 20260201000003-commands

## Task: Add the add, list and done commands

`main(argv=None)` builds the parser, parses `argv`, and returns `args.func(args)` when a subcommand was given. Handlers return an int exit status; `__main__` already passes it to `SystemExit`.
