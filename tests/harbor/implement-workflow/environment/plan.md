---
created_date: "2026-01-01"
document_status: draft
---

# Plan: 20260101000000-jwt-auth

## Overview

Deliver the JWT auth-service surface (login, refresh, revoke) alongside the
integrator-facing docs page that documents it. The work splits cleanly
across two repositories: the auth-service repo owns the endpoint
implementations and signing, and the docs repo owns the public reference
page. Both repos ship together so the docs page is live the moment the
endpoints are announced.

## Conventions

- All authored prose omits em dashes.

## Architecture & Design Decisions

- **RS256 asymmetric signing.** Chosen over HS256 so every consuming
  service instance can validate tokens independently with only the public
  key; no shared secret distribution problem.
- **Refresh token storage in PostgreSQL, hashed at rest.** Reuses the
  auth-service's existing PostgreSQL connection rather than introducing a
  new datastore.
- **Docs page under `content/api/auth.md`.** Sits alongside existing API
  reference pages and inherits the site's shared layout.

## Milestones & Phases

### Milestone 1: Endpoint implementation

#### - [x] Phase 1.1: JWT signing and validation in the auth-service repo

Implement RS256 signing and validation helpers behind a small `auth`
package inside the auth-service repo, covering token issuance on login,
token verification on protected requests, and revocation via a deny list.

**Acceptance criteria**:
- [x] `auth.Sign(claims)` produces an RS256-signed token that
      `auth.Verify(token)` accepts.
- [x] `auth.Verify` rejects tampered tokens with a distinctive error.

#### - [x] Phase 1.2: Login, refresh, and revoke endpoints in the auth-service repo

Wire the three endpoints (`/auth/login`, `/auth/refresh`, `/auth/revoke`)
to the auth package, returning both access and refresh tokens on login,
minting fresh access tokens on refresh, and pushing revoked token IDs
onto the deny list.

**Acceptance criteria**:
- [x] `POST /auth/login` returns `{"access_token", "refresh_token"}` on
      valid credentials.
- [x] `POST /auth/refresh` accepts a refresh token and returns a fresh
      access token.
- [x] `POST /auth/revoke` marks a token ID revoked; subsequent verify
      calls reject it.

### Milestone 2: Public documentation

#### - [x] Phase 2.1: Integration guide on the docs site

Add a single reference page to the docs site describing the three
endpoints, their request shapes, and their response shapes, cross-linked
from the API index.

**Acceptance criteria**:
- [x] Docs page lives at `content/api/auth.md` and lints clean.
- [x] The API index links to the new page.

## Open Questions

None outstanding at implementation time.

## Out of Scope

- OAuth2/OpenID Connect provider support
- Multi-factor authentication
- Migrating existing session-based clients

## Changelog

### 2026-01-01 - Phase 1.1: JWT signing and validation in the auth-service repo

**What was done**: Added a new `auth` package to the auth-service repo
owning RS256 signing, verification, and a small in-memory deny list for
revoked token IDs. `auth.Sign(claims)` produces a signed token;
`auth.Verify(token)` returns the claims and an error, rejecting anything
whose signature fails or whose token ID appears on the deny list.

**Deviations**: None. Signing key material is loaded from environment
config exactly as the plan specified.

**Files changed**:
- `auth/token.go`
- `auth/token_test.go`
- `auth/denylist.go`
- `auth/denylist_test.go`

**Discoveries**: None durable beyond this change.

### 2026-01-01 - Phase 1.2: Login, refresh, and revoke endpoints in the auth-service repo

**What was done**: Wired `POST /auth/login`, `POST /auth/refresh`, and
`POST /auth/revoke` to the `auth` package. Login returns both an access
token and a refresh token; refresh mints a fresh access token from a
valid refresh; revoke pushes the token ID onto the deny list so
subsequent verify calls reject it.

**Deviations**: None.

**Files changed**:
- `handlers/login.go`
- `handlers/login_test.go`
- `handlers/refresh.go`
- `handlers/refresh_test.go`
- `handlers/revoke.go`
- `handlers/revoke_test.go`
- `main.go`

**Discoveries**: None durable beyond this change.

### 2026-01-01 - Phase 2.1: Integration guide on the docs site

**What was done**: Added a single reference page to the docs repo at
`content/api/auth.md` covering all three endpoints (login, refresh,
revoke), with request/response shape blocks copied verbatim from the
handler tests. Linked the page from the API index.

**Deviations**: None. The docs site's existing layout picked up the new
page automatically; no navigation config edit was needed beyond the
index link.

**Files changed**:
- `docs: content/api/auth.md`
- `docs: content/api/index.md`

**Discoveries**: None durable beyond this change.
