### If the spec or a design is wrong

The work may show that the spec itself, or a design it references, is wrong: a requirement, acceptance criterion, constraint or success metric of the spec (`{{config.command}} spec file read {{plan_name}}`) is wrong or contradicts another, or contradicts a rule in a design the spec references (list them with `{{config.command}} design ref list --data '{"spec":"{{plan_name}}"}'` and read each with `{{config.command}} design read --data '{"source":"<name>","path":"<path>"}'`). When it does, STOP. Do not work around it, and do not build to whichever text you prefer. Name:

- the document: the spec, or the design's source and path;
- the section of the spec, or the rule in the design;
- the conflict;
- the amendment you propose.

{{^orchestrated}}
Ask the user whether to amend the spec or the design. Apply an amendment only after the user's explicit approval:

- **A spec amendment.** Read the spec with `{{config.command}} spec file read {{plan_name}}`, change only the Requirements, Acceptance Criteria, Constraints or Success Metrics the user approved, stage the full result with the `Write` tool at `.spektacular/tmp/{{plan_name}}/spec_amend.md`, then record it and remove the scratch file:

  ```
  {{config.command}} spec amend --data '{"name":"{{plan_name}}","reason":"<what was wrong and what the user approved>","run":"interactive implement run, task <task title>"}' --from .spektacular/tmp/{{plan_name}}/spec_amend.md
  rm .spektacular/tmp/{{plan_name}}/spec_amend.md
  ```

- **A design revision.** Revise a design Spektacular authored with `{{config.command}} design author`, which keeps its capture date and references. A design the user supplied is replaced only with a new version the user gives you, stored with `{{config.command}} design write`. Then record the revision on the spec:

  ```
  {{config.command}} spec amend --data '{"name":"{{plan_name}}","reason":"<what was wrong and what the user approved>","run":"interactive implement run, task <task title>","design":{"source":"<name>","path":"<path>"}}'
  ```

If the user declines, carry on as they decide. An amendment that would invalidate the plan's tasks is beyond this path: stop, and the user re-plans.
{{/orchestrated}}
{{#orchestrated}}
Never write the spec's text or a design document yourself, and never record an amendment: the agent that talks to the user applies an approved amendment in the project. Put the document, section, conflict and proposed amendment in your hand-back. When the answer says the amendment was applied, continue as below; when it says it was declined, carry on as the answer decides.
{{/orchestrated}}

Once an amendment is applied, re-read the amended spec (`{{config.command}} spec file read {{plan_name}}`) or design (`{{config.command}} design read`) and re-run this step's check of the current task against the amended text before you advance. Record the amendment under **Amendments** in this task's changelog entry.
