#!/bin/bash
# Reference solution for the plan-workflow harbor task.
#
# NOTE: This script documents the CLI sequence a correct run follows. It does
# NOT produce an agent transcript, so running it in isolation will not satisfy
# the verifier's transcript-driven assertions (skill retrieval, sub-agent
# spawning, next_step invariant, plan-file CLI usage). Its purpose is to prove
# the CLI surface is solvable end-to-end and to write the three artefact files;
# the real test is a `harbor run` with the claude-code agent.
set -e

cd /app

spektacular init claude

# The spec is already baked into the image at .spektacular/specs/20260101000000-user-auth.md

spektacular plan new --data '{"name":"20260101000000-user-auth"}'

# overview is reached automatically by plan new; advance through the rest.
spektacular plan goto --data '{"step":"discovery"}'
spektacular plan goto --data '{"step":"architecture"}'
spektacular plan goto --data '{"step":"components"}'
spektacular plan goto --data '{"step":"data_structures"}'
spektacular plan goto --data '{"step":"implementation_detail"}'
spektacular plan goto --data '{"step":"dependencies"}'
spektacular plan goto --data '{"step":"testing_approach"}'
spektacular plan goto --data '{"step":"milestones"}'
spektacular plan goto --data '{"step":"tasks"}'
spektacular plan goto --data '{"step":"open_questions"}'
spektacular plan goto --data '{"step":"out_of_scope"}'
spektacular plan goto --data '{"step":"assemble"}'
spektacular plan goto --data '{"step":"verification"}'

# The plan documents are owned by spektacular. Stage each filled document to a
# scratch file, then commit it into the plan store with `plan file write` —
# never write the plan documents with built-in file tools.

# Every task id is issued by spektacular, never invented, and every task names
# the one registered repo it is carried out in.
TASK_1_ID=$(spektacular plan task-id | jq -r .id)
TASK_2_ID=$(spektacular plan task-id | jq -r .id)
REPO=$(spektacular repo list | jq -r '.repos[0].name')

cat > /tmp/plan.md <<'PLAN_EOF'
# Plan: 20260101000000-user-auth

## Overview

Replace the current session-based authentication with stateless JWT access
and refresh tokens so the auth layer scales horizontally across service
instances without shared session state. Backend developers get a simpler
contract; end users get reliable login across services.

## Architecture & Design Decisions

A new `auth` package owns token issuance and validation. RS256 asymmetric
keys live in environment config so every service instance can verify
tokens independently without talking to a central session store. Middleware
validates the `Authorization` header on protected routes and attaches a
verified identity to the request context. Refresh tokens live in
PostgreSQL with hashed values; the deny list lives in Redis with TTL
matching token expiry.

Alternatives considered and rejected: symmetric HS256 signing (rejected
because every verifier would need the shared secret); server-side session
tokens (rejected because the whole point is to avoid shared session
state); storing refresh tokens plaintext (rejected because a DB dump
would leak live credentials).

## Component Breakdown

- **`auth/tokens` package** — issues and verifies JWT access/refresh tokens.
  Owns the RS256 signing keys and exposes `Issue()` and `Verify()`.
- **`auth/middleware` package** — the HTTP middleware that validates the
  `Authorization` header and rejects expired or revoked tokens.
- **`auth/store` package** — PostgreSQL access layer for hashed refresh
  tokens; used by the refresh and revoke endpoints.
- **`auth/denylist` package** — Redis-backed deny list with TTL; consulted
  by the middleware on every authenticated request.
- **`/auth/login`, `/auth/refresh`, `/auth/revoke` HTTP handlers** — the
  public surface the API exposes to clients.

## Data Structures & Interfaces

Access token payload: `{sub: user_id, role: string, exp, iat, iss}`. No PII.
Refresh token storage row: `{id, user_id, hashed_token, created_at,
expires_at, revoked_at}`. Deny list entry: Redis key `denylist:<token_id>`
with value `1` and TTL equal to remaining token lifetime.

## Implementation Detail

The `auth` package is a new module boundary; nothing else in the codebase
is refactored. The middleware is inserted at the router level so existing
handlers are unchanged. Migration from session-based auth runs in parallel
(both auth paths accepted) until all clients have migrated.

## Dependencies

- `github.com/golang-jwt/jwt/v5` — JWT signing and parsing.
- Existing PostgreSQL connection pool (no schema migrations beyond the new
  `refresh_tokens` table).
- Existing Redis client used elsewhere in the codebase.
- No new external services.

## Testing Approach

Unit tests for token issuance and verification covering valid, expired,
tampered, and revoked cases. Integration tests covering login, refresh,
and revoke flows against a real PostgreSQL and Redis. Cross-instance
validation test that issues a token from one instance and verifies it
from another. Load test to confirm the 5ms p99 latency target.

## Milestones & Tasks

### Milestone 1: Token issuance and verification

**What changes**: The `auth/tokens` package exists and can issue and verify
RS256-signed JWTs. Backend developers can call it directly in tests and see
correct accept/reject behaviour across valid, expired, and tampered tokens.

#### - [ ] Task: Implement RS256 issuance and verification
**Id:** __TASK_1_ID__
**Repo:** __REPO__
**Depends on:** none
**Execution:** agent

Implement the `auth/tokens` package with `Issue()` and `Verify()`. Load
RS256 keys from env config at startup. Cover valid, expired, and tampered
token cases with unit tests.

*Technical detail:* [context.md#task-implement-rs256-issuance-and-verification](./context.md#task-implement-rs256-issuance-and-verification)

**Acceptance criteria**:

- [ ] `Issue()` produces a JWT with the documented payload shape
- [ ] `Verify()` returns the claims for a valid token and an error for expired or tampered tokens
- [ ] Unit tests cover valid, expired, and tampered cases

### Milestone 2: HTTP auth surface

**What changes**: Clients can log in, refresh, and revoke tokens via
HTTP endpoints. The middleware rejects expired and revoked tokens on
protected routes. End users see a working login flow.

#### - [ ] Task: Login, refresh, revoke endpoints
**Id:** __TASK_2_ID__
**Repo:** __REPO__
**Depends on:**
- __TASK_1_ID__ — Implement RS256 issuance and verification
**Execution:** agent

Add the three HTTP handlers, backed by the `auth/tokens` package and the
new `refresh_tokens` table. Wire the middleware into the router.

*Technical detail:* [context.md#task-login-refresh-revoke-endpoints](./context.md#task-login-refresh-revoke-endpoints)

**Acceptance criteria**:

- [ ] POST /auth/login returns access and refresh tokens for valid credentials
- [ ] POST /auth/refresh exchanges a valid refresh token for a new access token
- [ ] POST /auth/revoke marks a token as revoked in the deny list

## Open Questions

None — every implementation uncertainty was resolved during planning.

## Out of Scope

- OAuth2 / OpenID Connect provider support
- Multi-factor authentication
- Social login
- Fine-grained permission scoping beyond role
PLAN_EOF
sed -i -e "s/__TASK_1_ID__/${TASK_1_ID}/g" -e "s/__TASK_2_ID__/${TASK_2_ID}/g" -e "s/__REPO__/${REPO}/g" /tmp/plan.md
spektacular plan file write 20260101000000-user-auth/plan.md --from /tmp/plan.md

cat > /tmp/context.md <<'CONTEXT_EOF'
# Context: 20260101000000-user-auth

## Current State Analysis

The existing auth layer is session-based: login writes a row to a sessions
table and returns a cookie. Every authenticated request hits the sessions
table via a shared DB connection. This does not scale horizontally because
session state is centralised. The JWT rewrite replaces that with stateless
token verification that every instance can perform independently.

## Per-Task Technical Notes

### Task: Implement RS256 issuance and verification

- `auth/tokens/tokens.go` — new file implementing `Issue()` and `Verify()`
- `auth/tokens/tokens_test.go` — new unit test file covering the three cases
- Key loading happens once at process start via `env.Get("JWT_PRIVATE_KEY")`

**Complexity**: Low
**Token estimate**: ~8k
**Agent strategy**: Single agent, sequential

### Task: Login, refresh, revoke endpoints

- `auth/handlers/login.go` — new handler, calls `auth/tokens.Issue()`
- `auth/handlers/refresh.go` — new handler, validates refresh token, issues new access
- `auth/handlers/revoke.go` — new handler, adds token ID to deny list
- `auth/middleware/auth.go` — new middleware, checks deny list + verifies signature
- `migrations/0042_refresh_tokens.sql` — new migration creating the refresh_tokens table
- `router/router.go:42` — wire the middleware and the three endpoints

**Complexity**: Medium
**Token estimate**: ~20k
**Agent strategy**: 2-3 parallel agents for independent handler files, sequential integration

## Testing Strategy

Unit tests are colocated with each package. Integration tests live under
`tests/integration/auth/` and use a docker-compose harness with real
PostgreSQL and Redis.

## Project References

- `thoughts/notes/commands.md` — project commands
- `thoughts/notes/testing.md` — testing patterns

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

Both auth paths run in parallel during migration. Feature flag controls
which path new clients see. Rollout proceeds per-service-instance.

## Performance Considerations

Token verification must add less than 5ms p99 latency. RS256 verification
is CPU-bound; benchmarks show ~0.3ms per verification on the production
hardware profile.
CONTEXT_EOF
spektacular plan file write 20260101000000-user-auth/context.md --from /tmp/context.md

cat > /tmp/research.md <<'RESEARCH_EOF'
# Research: 20260101000000-user-auth

## Alternatives considered and rejected

### Option A: Symmetric HS256 signing

Use a shared secret that every service instance holds.

**Rejected**: every verifier would need the shared secret, which spreads
the signing capability across every service and widens the blast radius
of a single leaked instance.

### Option B: Server-side session tokens with Redis

Keep sessions but move them from PostgreSQL to Redis for faster reads.

**Rejected**: still requires shared session state, which is the exact
problem this spec is trying to eliminate. Redis becomes a hard dependency
for every authenticated request.

### Option C: Plaintext refresh tokens in the database

Store refresh tokens without hashing so the refresh endpoint can look them
up by value.

**Rejected**: a database dump would leak live credentials. Hashing adds
negligible latency to the refresh flow.

## Chosen approach — evidence

- RS256 is supported by `github.com/golang-jwt/jwt/v5` with minimal
  configuration
- PostgreSQL `refresh_tokens` table fits the existing connection pool
  and migration tooling
- Redis is already a dependency of the codebase for rate limiting
- 5ms p99 latency is achievable with RS256 verification per benchmarks

## Files examined

- `auth/session/session.go` — the existing session-based auth
- `router/router.go:42` — where the middleware is inserted
- `env/env.go` — env var loading for the new JWT keys

## External references

- RFC 7519 (JSON Web Tokens) — payload and signing format
- `github.com/golang-jwt/jwt/v5` README — API shape

## Prior plans / specs consulted

- `.spektacular/specs/20260101000000-user-auth.md` — source spec

## Open assumptions

- The existing PostgreSQL connection pool has headroom for the new table
- Redis has capacity for the deny list (bounded by token TTL)
- Clients can be migrated incrementally without downtime

## Rehydration cues

- Start with `.spektacular/specs/20260101000000-user-auth.md`
- `auth/session/session.go` shows the current auth path
- `router/router.go:42` is where the new middleware slots in
RESEARCH_EOF
spektacular plan file write 20260101000000-user-auth/research.md --from /tmp/research.md

# Advance through the write steps — each verifies its document was committed —
# then through the mandatory walkthrough review (sign-off) to the finished step.
spektacular plan goto --data '{"step":"write_plan"}'
spektacular plan goto --data '{"step":"write_context"}'
spektacular plan goto --data '{"step":"write_research"}'
spektacular plan goto --data '{"step":"walkthrough"}'
spektacular plan goto --data '{"step":"finished"}'

cp -r /app/.spektacular /logs/artifacts/spektacular
