{{! Appended programmatically by stepkit.WriteStepResult on the steps that lead
    into an automatic git commit. Never include it from a step template. }}
## Automatic git commit

This project has `auto_commit` on, so advancing to `{{next_step}}` makes a **git
commit** in every registered repository that has changes. This is a commit to
git, not a document written to Spektacular — the two are different things.

Write the message yourself, from the work you just finished:

- The subject line must name `{{commit.spec_name}}`.{{#commit.milestone}} It must also name the milestone it completes, spelled `Milestone N`.{{/commit.milestone}}
- The body says what was specified, planned or implemented — the substance of
  the work, not a generic label.

{{#commit.milestone}}
Include a message **only if** the phase you just ticked was the last open phase
of its milestone. If it was not, advance with no `commit_message_from` at all
and no commit is made. If a milestone did just finish, the CLI refuses the
transition until you supply a message naming it.

{{/commit.milestone}}
Write the message to `{{commit.tmp_path}}` with your own file tool, then add it
to the advance command:

```
{{command}} {{commit.kind}} goto --data '{"step":"{{next_step}}","commit_message_from":"{{commit.tmp_path}}"}'
```

Do not ask the user to confirm the commit — it happens without asking.

If the CLI answers with `auto_commit_failed`, tell the user which repository
failed and the reason git gave, and do not advance. The workflow stays on this
step, so fixing the cause and re-running the same command retries the commit.
