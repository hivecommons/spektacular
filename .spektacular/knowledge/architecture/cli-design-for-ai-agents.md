---
tags: [cli, agent, json, schema, validation]
---

# CLI Design for AI Agents

Source: https://justin.poehnelt.com/posts/rewrite-your-cli-for-ai-agents/

## Core Thesis

CLIs designed for human operators require fundamental redesign when AI agents become primary users. Human-focused design prioritizes discoverability; agent-focused design prioritizes **predictability** and **defense against hallucinations**.

## Key Design Principles

### 1. Raw JSON Over Bespoke Flags

Prefer `--json` flags that accept complete API payloads rather than custom flag combinations. This maps directly to underlying APIs without translation loss, making it easier for LLMs to generate correct commands.

### 2. Runtime Schema Introspection

Expose API specifications as queryable JSON at runtime (e.g. `gws schema drive.files.list`). This eliminates the need to embed static documentation in system prompts and prevents agents from working with outdated information.

### 3. Context Window Efficiency

APIs returning massive responses waste agent tokens. Use:
- **Field masks** — let agents request only necessary fields
- **NDJSON pagination** — allow incremental processing of large result sets

### 4. Input Validation Against Hallucinations

Agents generate adversarial inputs differently than humans do: path traversals, embedded parameters, double-encoding, control characters. CLIs must validate exhaustively.

> "Assume inputs can be adversarial."

### 5. Agent Skills Documentation

Ship structured `SKILL.md` files that encode invariants agents cannot intuit, such as:
- Always use `--dry-run` for mutations
- Always add field masks to list operations

### 6. Multi-Surface Architecture

A single CLI binary can serve multiple consumers:
- **Humans** — interactive terminal with human-readable output
- **MCP** — JSON-RPC tool surface
- **Gemini extensions** — native capability surface
- **Headless/CI** — environment variables for auth, JSON output

### 7. Safety Rails

- `--dry-run` flag validates before mutating
- Response sanitization defends against **prompt injection** embedded in API data returned to the agent

## Implementation Roadmap (from article)

1. Add `--output json` flag
2. Add input validation
3. Expose schema introspection endpoint
4. Support field masks on responses
5. Implement `--dry-run` for mutating operations
6. Ship context documentation (SKILL.md or equivalent)
7. Optionally expose MCP surface
