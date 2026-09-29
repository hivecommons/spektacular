# Marketing Ideation: Brief

You are running the `{{workflow}}` workflow for `{{run_name}}`.

Capture the campaign seed before brainstorming:

1. Summarize the project, feature, release, or community moment being promoted.
2. State the goal in one sentence: awareness, contributors, adoption, feedback, sponsors, or another measurable outcome.
3. List the constraints that shape the campaign: timeline, launch channel, audience sensitivity, approvals, and assets already available.
4. Ask the user for anything missing, then record the agreed brief in `.spektacular/working-context.md`.

When the brief is settled, run:

```bash
{{command}} workflow goto {{workflow}} --data '{"step":"audience"}'
```
