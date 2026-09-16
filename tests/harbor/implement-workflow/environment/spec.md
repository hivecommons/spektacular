---
created_date: "2026-01-01"
document_status: draft
---

# Feature: jwt-auth

## Overview

Stateless user authentication using JWT access and refresh tokens, delivered
in tandem with public API documentation on the docs site. Replaces the
current session-based auth to enable horizontal scaling across multiple
backend services, and gives integrators a single reference for the new
endpoints on day one.

## Requirements

- **Issue signed JWT access tokens on successful login**
  The auth-service MUST return an RS256-signed access token when a user
  provides valid credentials.

- **Issue refresh tokens alongside access tokens**
  Every login response MUST include a refresh token that can be exchanged
  for a new access token without re-submitting credentials.

- **Validate token signatures on every authenticated request**
  The auth-service MUST reject any request whose token signature does not
  verify.

- **Document the new endpoints on the docs site**
  The docs site MUST carry an integration guide for `/auth/login`,
  `/auth/refresh`, and `/auth/revoke` before the feature is announced.

## Constraints

- Must use RS256 signing algorithm for JWT tokens
- Documentation changes must land in the docs repo, not the auth-service repo
- Token payload must not contain sensitive PII beyond user ID and role

## Acceptance Criteria

- **Valid login returns both tokens**
  A user with valid credentials receives a JWT access token and a refresh
  token in a single login response.

- **Docs page covers all three endpoints**
  The published docs page names each endpoint, its request shape, and its
  response shape.

## Technical Approach

Implement a new `auth` package in the auth-service repo that owns token
issuance and validation, using asymmetric RS256 keys. Add a matching
integration guide to the docs repo under `content/api/auth.md`.

## Success Metrics

- Token validation adds less than 5ms p99 latency to authenticated requests
- Docs page is reachable and lints clean

## Non-Goals

- OAuth2 / OpenID Connect provider support
- Multi-factor authentication
