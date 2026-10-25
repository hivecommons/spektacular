---
tags: [claude, tokens, logs, subagent, metrics]
---

# Claude Code's on-disk session transcripts undercount tokens, and other traps

`spektacular stats` (internal/agentusage) reads Claude Code's own session files,
`~/.claude/projects/<project>/<session>.jsonl` (or `$CLAUDE_CONFIG_DIR/projects`),
with helpers in `<session>/subagents/agent-<id>.jsonl` and
`agent-<id>.meta.json` (`toolUseId`, `parentAgentId`, `description`). This is a
different format from the headless `stream-json` Harbor reads
(`gotchas/harbor-agent-log-fields.md`). What bites:

- **The transcript misses most uncached input.** Side requests Claude Code makes
  outside the conversation (permission classifier, titles, summaries) are
  counted in the `cost-state` record's `modelUsage` but never written as
  `assistant` records. On real sessions the de-duplicated requests fall short
  of `cost-state` by 97-99% of uncached input and up to 64% of output, and
  side models (haiku) can be missing entirely. A run total that must match
  Claude Code has to add `cost-state` minus the transcripts as overhead, and
  that is only valid when the session drove that one run.
- **`cost-state` is written near the end of the session file.** A session still
  running may have none yet; read the last one.
- **One response is many records.** Each content block is its own `assistant`
  record repeating `message.id` and usage; a few later records carry larger
  usage. De-duplicate by `message.id`, keep the field-wise maximum, and time the
  request at its first record.
- **A user record's `message.content` is sometimes a plain string**, not a block
  list; decode it as raw JSON and walk only lists.
- **Filtering lines by the unescaped substring `"type":"assistant"` is safe**:
  the same text inside a string value (a tool result quoting a transcript) is
  escaped. It lets the reader skip large attachment and request-body lines
  without decoding them.
- **`<synthetic>` model records** carry no tokens and are not model requests.
- **An orchestrated child's transcript is a helper** of the orchestrator's
  session, so the child's run is found in `subagents/`, and session overhead
  is unknown for it (the session holds other runs).
