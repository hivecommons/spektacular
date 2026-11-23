# `spektacular meeting`: live meeting mode

`spektacular meeting` is the live mode a hive runs as a long-lived child
process for each meeting it attends. Hive streams the meeting into Spek's
stdin; Spek streams back *intents*: questions to ask, notes to keep,
research to start, and, when someone speaks to it by name, something to say.
Hive is the policy point: it decides what actually reaches the meeting, and
Spek never talks to the meeting directly.

```bash
spektacular meeting --from stdin --format jsonl --title "Weekly sync" --context ./meeting-ctx
```

| Flag | Value |
|---|---|
| `--from` | `stdin` (the only source) |
| `--format` | `jsonl` (the only format) |
| `--title` | the meeting's title, given to the model and used to head the summary |
| `--context` | a directory of files the model may read: agenda, prior knowledge, earlier notes. Every non-hidden text file under it is read, in path order, up to `SPEK_MEETING_CONTEXT_MAX_BYTES` |

The command runs anywhere: it needs no Spektacular project, is not held back
by the upgrade gate, and leaves nothing in a project's log.

## The contract

The wire format is **section 2 of the quorum contract**,
[hivecommons/quorum `docs/contract.md`](https://github.com/hivecommons/quorum/blob/main/docs/contract.md)
(`quorum.contract/v1`), shared by hive, quorum and Spektacular. That file is
authoritative; this page describes how Spek implements it. A change to the
lines either side reads or writes goes through that file first.

One JSON object per line in both directions. Unknown fields are ignored.

**stdin** (hive to Spek):

| Line | Spek's handling |
|---|---|
| `{"type":"segment", "meeting_id", "seq", "speaker":{id,name}, "text", "start", "end", "final"}` | Added to the transcript window. Only `final:true` segments count; a non-final one is logged and skipped. |
| `{"type":"chat", "author":{id,name}, "text", "at", ...}` | Added to the window as chat. A line whose author is one of Spek's names (its own post echoed back) is ignored. |
| `{"type":"answer", "qid", "text", "by"}` | Recorded against the question; the model sees it from then on and the summary lists it as answered. An unknown `qid` is logged. |
| `{"type":"research_update", "rid", "status", "summary?", "url?"}` | Status recorded. The first `done` with a summary adds a `notes.append` in section `notes`. An unknown `rid` is logged. |
| `{"type":"end"}` | The meeting is over: Spek writes the summary and exits 0. Lines after it are ignored. |

A line that is not a JSON object, has no `type`, or lacks a field its type
needs is logged to stderr and skipped; so is an unknown `type`.

**stdout** (Spek to hive):

| Intent | When |
|---|---|
| `{"type":"heartbeat"}` | At start, then every `SPEK_MEETING_HEARTBEAT_S`. |
| `{"type":"ask", "qid", "text"}` | A clarifying question, only at a natural pause, within the ask budget, never a repeat. |
| `{"type":"notes.append", "section", "text"}` | A line for the running notes in `decisions`, `actions` (with the owner when one was named), `questions` or `notes`; `summary` at the end. |
| `{"type":"research", "rid", "query", "reason"}` | An explicit research request, at once; or an offer someone agreed to. |
| `{"type":"chat.post", "text"}` | An offer to research something mentioned in passing; rarely, a short helpful message from the model. |
| `{"type":"say", "text"}` | A spoken reply, only when the latest segment addressed Spek by name. Hive decides whether voice is allowed. |

Stdout carries contract lines and nothing else: every intent is validated
before it is written (known type, required fields, a known section, ids of
the form `q-…`/`r-…`, text of at most 2000 characters) and an invalid one is
logged and dropped. Logs, skipped input and errors, including a refused flag,
go to stderr; a refused flag exits 1 with stdout empty.

## How Spek decides

Cheap rules run on every final line, without a model call:

- **Explicit research** ("hive, research X", "spek, can you look into X",
  "can you look up X", "go find out X") becomes `research` immediately, with a
  generated `rid`. Asking again for the same thing does not start it twice.
- **Implicit research** ("we should look into X", "someone should investigate
  X", "it would be good to know X") becomes a `chat.post` offer. A reply that
  opens with a yes ("yes", "sure", "go ahead", "please do") turns the latest
  open offer into `research`; a no ("no", "not now", "skip it") withdraws it.
  An offer nobody answers expires after `SPEK_MEETING_OFFER_TTL_S`.
- **Being addressed**: a line that starts with one of Spek's names (after
  fillers such as "hey" or "ok"), puts a comma or colon after it, or ends
  with ", hive?" calls for a decision straight away.

A **decision** is one model call, bounded by `SPEK_MEETING_LLM_TIMEOUT_MS`,
taken when Spek is addressed or at a **natural pause**: no new final segment
for `SPEK_MEETING_PAUSE_MS` after some speech. At most one decision is in
flight; stdin, heartbeats and the rules keep running while it is. The model
receives the title, the `--context` files, the recent window
(`SPEK_MEETING_WINDOW_SEGMENTS` lines), the running notes, the questions
asked so far with their answers, and the research in flight, all marked as
data rather than instructions. It replies with structured JSON proposing
asks, notes, chat, a say and research topics, and Spek then applies the rules:

- an ask only when the decision was taken at a pause, one per decision, at
  most `SPEK_MEETING_MAX_ASKS_PER_10M` in any ten minutes;
- never the same question twice: an ask the model marks `duplicate_of` an
  earlier qid is dropped, as is one that matches an earlier question once
  normalised (case, punctuation, filler words) or shares most of its content
  words;
- notes only in the four meeting sections, each line once;
- a say only when the decision was taken on a line that named Spek and the
  latest segment still does when the reply lands;
- research the model suggests only as an offer, one per decision.

A decision that fails or times out emits nothing and is logged.

On `end` or EOF (or SIGINT/SIGTERM) Spek lets a decision already under way
land, takes one closing decision (`SPEK_MEETING_SUMMARY_TIMEOUT_MS`) for an
overview and any missing notes, then writes a `notes.append` in section
`summary`: the overview, decisions, actions with owners, open questions
(unanswered asks and questions people raised), answered questions and
research. A longer summary is split over several lines of at most 2000
characters. Without a model, or when the closing call fails, the summary is
built from what was recorded during the meeting. Spek then exits 0.

## The model

`SPEK_MEETING_LLM` picks the backend; left unset, the first that is
configured wins:

| Backend | Chosen when | Needs |
|---|---|---|
| `anthropic` | `SPEK_MEETING_LLM=anthropic`, or `ANTHROPIC_API_KEY` / `ANTHROPIC_AUTH_TOKEN` is set | The Claude API through the official Go SDK, which reads its own variables (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`). Model `SPEK_MEETING_MODEL` (default `claude-haiku-5-5`, the fastest tier), low effort, structured output, no retries. |
| `command` | `SPEK_MEETING_LLM=command`, or `SPEK_MEETING_LLM_CMD` is set, or the project's `agent` is `claude` and `claude` is on `PATH` | A one-shot command that reads the prompt on stdin and prints the JSON reply on stdout, killed at the timeout. `SPEK_MEETING_LLM_CMD` is split on spaces. For a `claude` project the default is `claude -p --model haiku --output-format text`; other agents need `SPEK_MEETING_LLM_CMD`. |
| `none` | `SPEK_MEETING_LLM=none`, or nothing above is configured | No model: the rules above (research, offers, answers, heartbeats) and the summary from recorded notes only. |

The startup line on stderr names the backend in use. The live target is a
decision in under three seconds; a CLI backend's start-up time counts against
`SPEK_MEETING_LLM_TIMEOUT_MS`, so the `anthropic` backend is the one that
meets it.

## Settings

Every limit is an environment variable; a value that does not parse as a
positive integer is reported on stderr and the default kept.

| Variable | Default | Meaning |
|---|---|---|
| `SPEK_MEETING_PAUSE_MS` | 1500 | Silence after the last final segment that counts as a pause |
| `SPEK_MEETING_MAX_ASKS_PER_10M` | 3 | Asks allowed in any ten minutes |
| `SPEK_MEETING_HEARTBEAT_S` | 15 | Interval between heartbeats |
| `SPEK_MEETING_LLM_TIMEOUT_MS` | 2500 | Bound on one decision call |
| `SPEK_MEETING_SUMMARY_TIMEOUT_MS` | 10000 | Bound on the closing summary call |
| `SPEK_MEETING_WINDOW_SEGMENTS` | 40 | Recent lines the model sees |
| `SPEK_MEETING_OFFER_TTL_S` | 120 | How long a research offer waits for a yes |
| `SPEK_MEETING_CONTEXT_MAX_BYTES` | 65536 | Bound on the `--context` files sent to the model |
| `SPEK_MEETING_NAMES` | `hive,spek,spektacular` | Comma-separated names that address Spek |
| `SPEK_MEETING_LLM` | (auto) | `anthropic`, `command` or `none` |
| `SPEK_MEETING_MODEL` | `claude-haiku-5-5` | Model for the `anthropic` backend |
| `SPEK_MEETING_MAX_TOKENS` | 1024 | Reply bound for the `anthropic` backend |
| `SPEK_MEETING_LLM_CMD` | (none) | Command for the `command` backend |

## Tests

`internal/meeting` replays golden transcripts on a fake clock. Each fixture
under `internal/meeting/testdata/replay/` is a timed stream of stdin lines and
a scripted fake model; the harness steps the clock exactly as the live loop
does and compares the decisions taken and intents emitted with the fixture's
`.golden.jsonl`. The fixtures cover questions only at pauses, the ask budget,
question dedupe, explicit against implicit research, answer routing, say only
when addressed, the summary on end, malformed input and heartbeat timing.
Regenerate the goldens after an intended change with:

```bash
go test ./internal/meeting -run TestReplayGolden -update
```
