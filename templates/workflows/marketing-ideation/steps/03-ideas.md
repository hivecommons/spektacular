# Marketing Ideation: Ideas

Generate three campaign concepts for `{{run_name}}`.

For each concept, include:

- title;
- core message;
- channel mix;
- first asset to create;
- why it fits the audience;
- risk or reason to reject it.

Recommend one concept and explain the tradeoff. Ask the user to choose or revise. Update `.spektacular/working-context.md` with the chosen concept and rationale.

If the audience framing changed, go back:

```bash
{{command}} workflow goto {{workflow}} --data '{"step":"audience"}'
```

When the concept is chosen, run:

```bash
{{command}} workflow goto {{workflow}} --data '{"step":"publish"}'
```
