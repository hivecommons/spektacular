#!/bin/bash
set -e

cd /app

# Initialize project
spektacular init claude

# Start the spec workflow and capture the resolved spec name.
SPEC_JSON=$(spektacular spec new --data '{"name":"user-auth"}')
SPEC_NAME=$(echo "$SPEC_JSON" | jq -r '.spec_name')

# Run the interview step before any section is drafted.
spektacular spec goto --data '{"step":"interview"}'

# Walk every section step. Section content is gathered in conversation; the
# completed spec is committed to the store in a single write at the end.
#
# This scripted walkthrough does not reproduce the cross-section-amendment
# scenario (TestCrossSectionAmendment in test_spec_workflow.py) — that proof
# depends on a live rejection-and-follow-up conversation the agent conducts,
# which a non-interactive CLI script has no equivalent for. The token
# revocation requirement below is included directly in the hardcoded content
# rather than surfaced via a scripted rejection; the cross-section-amendment
# behavior itself is verified only against the live agent run.
for STEP in overview requirements acceptance_criteria constraints technical_approach success_metrics non_goals verification; do
  spektacular spec goto --data "{\"step\":\"$STEP\"}"
done

# Assemble the completed spec. The heredoc expands $SPEC_NAME only.
cat > /tmp/spec_final.md << CONTENT
# Feature: $SPEC_NAME

## Overview

Stateless user authentication system using JWT access and refresh tokens.
Replaces the current session-based auth to enable horizontal scaling across
multiple backend services. Benefits backend developers who consume the auth
API and end users who need reliable login across services.

## Requirements

- **Issue access token** — The system issues a signed JWT access token on successful login.
- **Issue refresh token** — The system issues a refresh token alongside the access token.
- **Access token expiry** — Access tokens expire after 15 minutes.
- **Refresh token expiry** — Refresh tokens expire after 7 days.
- **Validate signatures** — The system validates token signatures on every authenticated request.
- **Reject expired tokens** — The system rejects expired tokens with a 401 response.
- **Support revocation** — The system supports token revocation via a deny list.
- **Hash stored tokens** — The system hashes refresh tokens before storing them.

## Constraints

- Must not store access tokens server-side.
- Refresh token storage must use the existing PostgreSQL database.
- Must not break existing API endpoints during migration.
- Token payload must not contain sensitive PII beyond user ID and role.

## Acceptance Criteria

- [ ] A user can log in with valid credentials and receive a JWT access token and refresh token.
- [ ] A request carrying an expired access token returns HTTP 401.
- [ ] A valid refresh token can be exchanged for a new access token.
- [ ] A revoked token is rejected on the next authenticated request.
- [ ] Tokens issued by one service instance are accepted by another instance.

## Technical Approach

- Add a new auth package implementing token issuance and validation.
- Use asymmetric RS256 keys stored in environment configuration.
- Implement middleware that validates the Authorization header on protected routes.
- Store refresh tokens in a refresh_tokens PostgreSQL table with hashed values.
- Add /auth/login, /auth/refresh, and /auth/revoke endpoints.
- Use Redis for the token deny list with a TTL matching token expiry.

## Success Metrics

- Token validation adds less than 5ms p99 latency to authenticated requests.
- The auth service sustains 1000 login requests per second.
- Zero downtime during migration from session-based auth.
- All existing integration tests pass after migration.

## Non-Goals

- OAuth2 / OpenID Connect provider support (future work).
- Multi-factor authentication (separate initiative).
- Social login (Google, GitHub, etc.).
- Fine-grained permission scoping beyond role-based access.
CONTENT

# Commit the completed spec into the store through the CLI. The spec file is
# owned by spektacular — never edit it directly with built-in file tools. The
# body is staged under .spektacular/tmp/ and passed with --from.
mkdir -p .spektacular/tmp
cp /tmp/spec_final.md .spektacular/tmp/spec_final.md
spektacular spec file write "$SPEC_NAME" --from .spektacular/tmp/spec_final.md
rm .spektacular/tmp/spec_final.md

# Finish the workflow.
spektacular spec goto --data '{"step":"finished"}'
