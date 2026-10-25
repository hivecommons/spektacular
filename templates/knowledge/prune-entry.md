## Prune: check one knowledge entry

{{#dry_run}}
**This is a dry run (a preview).** Change nothing: do not write, correct or delete this entry or any other. Judge the entry exactly as you would in a real run and record the verdict you would give; the report then shows what a real prune would do.

{{/dry_run}}
Check this entry against the project and give it exactly one verdict. {{{remaining}}} entries are left to check, this one included.

- **Tier:** `{{{tier}}}`
- **Store:** `{{{name}}}`
- **Path:** `{{{path}}}`

The entry's full text, exactly as it is stored:

````markdown
{{{body}}}
````

Do not run `{{{command}}} knowledge read` or `{{{command}}} knowledge search` for this entry: its text is above, and prune's own reads must not count as usage. Check it against the project's code instead: the files, commands, flags, packages and behaviour it describes, in the repos the project registers (`{{{command}}} repo list` reports where each repo's code lives).

### The verdict rules

- **current**: the entry still describes how the project works, or states a standard it is meant to meet. An entry that states a standard the code has not yet met is **current**, not stale: in this project an entry states the target, and the code is what has yet to meet it, so a difference between the entry and the code is work to do, never evidence against the entry. An entry you cannot check from the code (a claim about intent, process or an external system) is also **current**. Leave a current entry exactly as it is.
- **deleted**: only an entry whose subject no longer exists is deleted. The file, command, flag, package or behaviour it is about is gone from the project.
- **corrected**: the subject still exists but the entry describes it wrongly. Correct the entry so it describes the subject as it is.

Never judge from age or tone: "looks old", "seems outdated" and "probably superseded" are not findings.

**Evidence is required for every change.** For a corrected or deleted entry, name the specific file, command, flag or behaviour you checked and what you found, so a reviewer can check the finding without opening the code. A change you cannot back with evidence is not a change: give the entry **current**.

{{^dry_run}}
### Making the change

- **To correct it**, stage the revised entry under `.spektacular/tmp/<slug>.md` with your own file tool, carrying its frontmatter block (its tags) through unchanged unless the correction is about the tags, then write it at its original address and remove the scratch file:

  ```
  {{{command}}} knowledge write --data '{"tier":"{{{tier}}}","name":"{{{name}}}","path":"{{{path}}}"}' --file .spektacular/tmp/<slug>.md
  rm .spektacular/tmp/<slug>.md
  ```

- **To delete it**, remove it through the tool, never with your own file tools:

  ```
  {{{command}}} knowledge delete --data '{"tier":"{{{tier}}}","name":"{{{name}}}","path":"{{{path}}}"}'
  ```

Prune acts without asking: make the change, then report it. Recovery from a wrong change is through the project's version history.

{{/dry_run}}
### Report the verdict

Then report your verdict, with your evidence, by running exactly this command with the verdict and evidence filled in. Keep single quotes out of the evidence, since the JSON sits inside them:

```
{{{command}}} knowledge prune verdict --data '{"tier":"{{{tier}}}","name":"{{{name}}}","path":"{{{path}}}","verdict":"<current|corrected|deleted>","evidence":"<what you checked and what you found>"}'
```

The command checks your verdict against the store before it accepts it: **deleted** means the entry is gone, **corrected** means its text has changed, and **current** means it is untouched{{#dry_run}}; in a dry run the entry must be untouched whatever the verdict{{/dry_run}}. It then returns the next entry's instruction, or the final report once every entry has a verdict.
