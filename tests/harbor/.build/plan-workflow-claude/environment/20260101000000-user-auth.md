# Feature: user-auth

## Overview

Stateless user authentication using JWT access and refresh tokens. Replaces
the current session-based auth to enable horizontal scaling across multiple
backend services. Benefits backend developers who consume the auth API and
end users who need reliable login across services.

## Requirements

- **Issue signed JWT access tokens on successful login**
  The system MUST return an RS256-signed access token when a user provides
  valid credentials.

- **Issue refresh tokens alongside access tokens**
  Every login response MUST include a refresh token that can be exchanged
  for a new access token without re-submitting credentials.

- **Enforce short access token expiry**
  Access tokens MUST expire after 15 minutes.

- **Enforce longer refresh token expiry**
  Refresh tokens MUST expire after 7 days.

- **Validate token signatures on every authenticated request**
  The system MUST reject any request whose token signature does not verify.

- **Reject expired tokens with HTTP 401**
  The system MUST return 401 Unauthorized for expired tokens, without
  leaking whether the token was malformed or simply expired.

- **Support token revocation via deny list**
  The system MUST support revoking a token such that subsequent requests
  bearing it are rejected.

- **Hash refresh tokens at rest**
  Refresh tokens MUST be hashed before storage.

## Constraints

- Must use RS256 signing algorithm for JWT tokens
- Must not store access tokens server-side
- Refresh token storage must use the existing PostgreSQL database
- Must not break existing API endpoints during migration
- Token payload must not contain sensitive PII beyond user ID and role

## Acceptance Criteria

- **Valid login returns both tokens**
  A user with valid credentials receives a JWT access token and a refresh
  token in a single login response.

- **Expired access tokens are rejected**
  A request with an expired access token returns HTTP 401.

- **Refresh flow issues a new access token**
  A valid refresh token can be exchanged for a new access token without
  re-submitting credentials.

- **Revoked tokens are rejected**
  A revoked token is rejected on the next authenticated request.

- **Cross-instance token validation works**
  Tokens issued by one service instance are accepted by another instance
  without shared session state.

## Technical Approach

Implement a new `auth` package that owns token issuance and validation.
Use asymmetric RS256 keys stored in environment config so every service
instance can validate tokens independently. Introduce middleware that
validates the `Authorization` header on protected routes and attaches a
verified user identity to the request context. Store refresh tokens in a
`refresh_tokens` PostgreSQL table with hashed values. Add `/auth/login`,
`/auth/refresh`, and `/auth/revoke` endpoints. Use Redis for the token
deny list with TTL matching token expiry so entries age out automatically.

## Success Metrics

- Token validation adds less than 5ms p99 latency to authenticated requests
- Auth service handles 1000 login requests per second
- Zero downtime during migration from session-based auth
- 100% of existing integration tests pass after migration

## Non-Goals

- OAuth2 / OpenID Connect provider support (future work)
- Multi-factor authentication (tracked separately)
- Social login (Google, GitHub, etc.)
- Fine-grained permission scoping beyond role-based access
