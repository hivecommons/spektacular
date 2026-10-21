## Step {{step}}: {{title}}

Have a separate reviewer check the current task's goal before anything is ticked. The reviewer starts with a fresh context, reads the work itself and changes nothing. It judges every acceptance criterion **met**, **not_met** or **impossible**, with evidence. `{{config.command}} implement goal record` then decides what happens next: you never decide it yourself.

### Step 1: The goal to judge

{{> partials/implement-current-task}}

This is the current item's goal, exactly as the CLI built it from the plan. Hand it to the reviewer **verbatim**, from the `GOAL` line to the `END GOAL` line:

```
GOAL {{{goal.item}}}: {{{goal.title}}}
Acceptance criteria (one verdict each):
{{#goal.criteria}}
{{n}}. {{{text}}}
{{/goal.criteria}}
Scope:
- Repo: {{{goal.repo}}}
{{#goal.has_files}}
- Files this item may change:
{{#goal.files}}
  - {{{repo}}}: {{{path}}}
{{/goal.files}}
{{/goal.has_files}}
- Rule: {{{goal.rule}}}
END GOAL
```

{{#awaiting_decision}}
### This check has already stopped

The last goal check of `{{{goal.item}}}` (round {{round}}, attempt {{attempt}}) stopped the run, and the user's answer has not been recorded yet. Do not check again. Go straight to Step 5 and ask the user about these criteria:

{{#stop_verdicts}}
- Criterion {{n}} ({{verdict}}): {{{text}}}. Evidence: {{{evidence}}}
{{/stop_verdicts}}

{{/awaiting_decision}}
{{^awaiting_decision}}
This is attempt {{attempt}} of round {{round}}. A round allows the first check and two retries.

{{/awaiting_decision}}
{{> partials/implement-code-locations}}

### Step 2: Stage the work for the reviewer

Just before launching the reviewer, record what the work looks like, for every code root above, with your own file tools. For each repo `<repo>` at code root `<root>`, write:

- the output of `git -C <root> diff HEAD` to `{{goal_tmp_dir}}/<repo>.diff`;
- the output of `git -C <root> status --porcelain --untracked-files=all` to `{{goal_tmp_dir}}/<repo>.status`.

Change nothing in the work after staging it. The CLI compares these files with the work when you record the verdicts, which shows whether the reviewer changed anything.

### Step 3: Launch the reviewer in a fresh context

Launch the reviewer as a separate helper agent, never as yourself:

- **If your agent has a helper named `spek-goal-reviewer`**, launch it. It is installed for Claude Code, Codex, Copilot and omp, on a cheaper model, with read-only tools where the agent supports them.
- **Under Bob**, launch Bob's built-in read-only `explore` helper, and give it the brief from `{{config.command}} skill review-task-goal` verbatim as the start of its prompt.
- **Otherwise**, launch any sub-agent that starts with a fresh context, and give it the brief from `{{config.command}} skill review-task-goal` verbatim as the start of its prompt.

Where you can choose the helper's tools, give it read and search tools only, never a tool that writes, edits or runs commands.

Hand the reviewer exactly these, and nothing else:

1. the goal block from Step 1, verbatim;
2. the verify step's pass/fail summary, one line per command;
3. pointers to the work: each code root, the staged `{{goal_tmp_dir}}/<repo>.diff` and `{{goal_tmp_dir}}/<repo>.status` paths, and the files the goal names.

Give it nothing from your own conversation: no summary of what you did, no account of your reasoning, and no claim that the work is complete or that any criterion is met. The reviewer judges the work from the files, not from you.

### Step 4: Record the verdicts

Write the reviewer's reply **unchanged** to `{{goal_tmp_dir}}/reply.json` with your own file tool. Never edit, reorder, complete or summarise it. Then record it:

```
{{config.command}} implement goal record --data '{"name":"{{plan_name}}","item":"{{{goal.item}}}"}' --from {{goal_tmp_dir}}/reply.json
```

If the CLI refuses the reply (`goal_reply_invalid`), it names what is wrong: ask the reviewer again with the same input, stage its new reply and record again. Once the record succeeds, remove the files you staged under `{{goal_tmp_dir}}/`.

### Step 5: Act on the decision

The record answers with a `decision`, the criteria in each verdict, and the exact `next_action` to run.

- **`accept`**: every criterion was judged met. Run the `next_action`, which advances to `update_plan`.
- **`retry`**: some criteria are not yet met and the round has retries left. Run the `next_action`, which goes back to `implement`; that step shows the reviewer's reasons. The task then runs through `test`, `verify` and this check again.
- **`exhausted`**: criteria are still not met after the round's last retry. STOP. Report each criterion that is not met, with the reviewer's evidence, and ask the user whether to:
  1. **Keep fixing**: another round, with two more retries (`keep_fixing`).
  2. **Accept the shortfall**: tick only the criteria judged met and move on (`accept`).
  3. **Skip the task**: tick nothing and move on (`skip`).
- **`impossible`**: a criterion cannot be met as written. STOP at once, without retrying. Report the criterion and the reviewer's evidence, and ask the user how to proceed:
  1. **Correct the criterion**: fix its text in the plan (read it with `{{config.command}} plan file read {{plan_name}} plan`, stage the corrected document under `.spektacular/tmp/{{plan_name}}/` and write it with `{{config.command}} plan file write {{plan_name}} plan --from <staged file>`), then check again (`recheck`).
  2. **Keep fixing** (`keep_fixing`), **accept the shortfall** (`accept`) or **skip the task** (`skip`), as above.

Record the user's answer, then run the `goto` it returns:

```
{{config.command}} implement goal decide --data '{"name":"{{plan_name}}","item":"{{{goal.item}}}","decision":"<keep_fixing|accept|skip|recheck>","reason":"<the user's reason, if given>"}'
```

{{> partials/implement-spec-conflict}}

### Advance

Run the goto that `implement goal record` (or `implement goal decide`) returns, never one of your own choosing. It is one of:

```
{{config.command}} implement goto --data '{"step":"{{next_step}}","name":"{{plan_name}}"}'
{{config.command}} implement goto --data '{"step":"implement","name":"{{plan_name}}"}'
```
