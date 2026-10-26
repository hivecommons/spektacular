---
created_date: "2026-10-25"
document_status: final
closed_date: "2026-10-25"
---

# Test plan: 000078_token-use-in-run-stats

Manual checks for the success metrics and manual reviews the plan's Testing Approach marks as manual. Everything else is covered by the Go suite (`go test -shuffle=on ./...`). Build the binary from the merged branch first, and never install it over the one driving a workflow:

```bash
go build -o /tmp/spek78 .        # in the spektacular repo
```

Every check below only reads: `stats` never writes a log, and the reader never writes a Claude Code session file.

## Metric 1: run total within 1% of Claude Code's own totals

**What to measure**: for a real implement run in a Claude Code session that drove only that run, the report's run total, which includes the tokens outside any step, matches Claude Code's own usage totals for the session within 1%, for each token type.

**How**:
1. In a scratch Spektacular project, start a fresh Claude Code session (`claude`). Run one implement workflow to `finished` in it, with nothing else in that session: no other spec, plan or implement run and no orchestrator. Then exit the session with `/exit`, because Claude Code writes its `cost-state` totals near the end of the session file.
2. From the project root, report the run:
   ```bash
   /tmp/spek78 stats run --name <spec> --kind implement --format json > /tmp/r.json
   python3 -I -c "import json; t=json.load(open('/tmp/r.json'))['tokens']; print(t['status'], t['session_overhead_reason'], t['total'], t['sessions'])"
   ```
3. Read Claude Code's own totals from the session file the report names (`tokens.sessions[0].file`):
   ```bash
   python3 -I -c "
   import json,sys
   last=None
   for l in open(sys.argv[1]):
       if '\"type\":\"cost-state\"' in l: last=json.loads(l)
   u=last['modelUsage']
   print({k: sum(m[k] for m in u.values()) for k in ('inputTokens','outputTokens','cacheReadInputTokens','cacheCreationInputTokens')})
   " <file>
   ```

**Expected result**:
- `status` is `known`, `session_overhead_reason` is `null`, and `session_overhead` is a figure.
- Each of `total.input`, `total.output`, `total.cache_read` and `total.cache_write` is within 1% of `inputTokens`, `outputTokens`, `cacheReadInputTokens` and `cacheCreationInputTokens` (by construction they should be equal).
- If the overhead is unknown with `totals_below_transcript`, or the totals differ by more than 1%, stop and raise it: the plan's Open Questions say a `cost-state` record that does not cover the whole session file needs a decision before overhead is computed differently.

**Who / when**: a maintainer, once before release, on a machine running Claude Code 2.1.309 or later.

## Metric 2: steps ranked by time per thousand output tokens

**What to measure**: the maintainer can rank a run's steps by working time per thousand output tokens and pick out the steps that are slow for the work they did.

**How**: on any finished Claude Code run in this project's logs, for example the epic's own implement runs:
```bash
/tmp/spek78 stats run --name 000077_html-log-viewer --kind implement
/tmp/spek78 stats run --name 000077_html-log-viewer --kind implement --format json \
  | python3 -I -c "import json,sys; d=json.load(sys.stdin); [print(s['step'], s['duration_ms'], s['tokens'] and s['tokens']['output'], s['tokens'] and s['tokens']['working_ms_per_1k_output']) for s in sorted(d['steps'], key=lambda s: -((s['tokens'] or {}).get('working_ms_per_1k_output') or 0))]"
```

**Expected result**: every step with output tokens has a `TIME/1K OUT` value in the Steps table and a `working_ms_per_1k_output` integer in the JSON, and steps with no output tokens show `n/a` (`null`). The sorted list puts short-output, long-duration steps (typically `verify` or `read_plan`) above heavy-output steps (`implement`, `check_goal`). The maintainer can name the slowest-for-its-work step from the table alone.

**Who / when**: a maintainer, once before release.

## Review 1: the figures are plausible and the tables readable on real data

**What to look at**: `stats run`, `stats compare` and `stats summary` against this machine's real logs (`.spektacular/logs`) and Claude Code sessions (`~/.claude/projects`), for a plan run with helpers and an implement run with tasks:
```bash
/tmp/spek78 stats list
/tmp/spek78 stats run --kind plan                      # a plan run with helpers
/tmp/spek78 stats run --kind implement                 # an implement run with tasks
/tmp/spek78 stats compare --kind implement
/tmp/spek78 stats summary --kind implement
time /tmp/spek78 stats summary --kind implement --format json > /dev/null
```
Add a `stats.prices` table (see the Run statistics page) to a scratch copy of the project's config, or run in a scratch project, to see costs; never edit this repository's own `.spektacular/config.yaml`.

**What to look for**:
- The `Tokens` block names the session and transcript it read; an orchestrated child names its own helper transcript.
- Step, task and helper output tokens look proportionate to the work (`implement` and `check_goal` heavy; `update_plan` light), helpers carry their own tokens and pace, and the outside-any-step row is small for an orchestrated child.
- Runs by other agents show `unknown (agent_unsupported)` and `?` cells; their timings match `stats run` from before this change.
- Costs with a price table match a hand calculation for one step (tokens times price per million, summed over the four types); an unpriced model gives `?` or a cost marked `*` with the footnote naming it.
- Columns line up, and the comparison and summary step tables for output tokens, cost and time per 1k are readable.
- A summary over every implement run finishes in about a second (0.26 s on the development machine with two large sessions).

**Passing**: no figure is implausible (for example, a helper's tokens counted in the step after its launch, a step total above the run total, or a zero where the data is missing), and the tables are readable at a normal terminal width.

**Who / when**: a maintainer, before release.

## Review 2: the documentation matches the shipped behaviour

**What to look at**: the built site, in the docs repo:
```bash
npm ci && npm run build && npx astro preview
```
Open `/run-stats/` (sections "Tokens and cost" to "When tokens are unknown", and the `tokens` example in "Reports for programs") and `/configuration/` (the `stats` key and the example `config.yaml`).

**What to look for**: every column, field, flag (`--sessions-dir`), key (`stats.prices.<model>.input` and the others) and reason code (`agent_unsupported`, `sessions_dir_missing`, `session_not_found`, `session_unreadable`, `format_unrecognised`) named on the pages exists with that name in `stats run --format json` and `stats run --schema`; the description of how a run finds its session, of helper attribution and of the incomplete-cost mark matches what Review 1 showed; the top-level key list says eighteen keys and includes `stats`.

**Passing**: no statement on either page contradicts the binary's behaviour or output.

**Who / when**: a maintainer or docs reviewer, before release.
