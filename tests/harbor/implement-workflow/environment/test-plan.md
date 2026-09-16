---
created_date: "2026-01-01"
document_status: draft
---

# Test Plan: 20260101000000-jwt-auth

## Manual verification

- `POST /auth/login` with a valid user returns a JSON body containing
  `access_token` and `refresh_token` fields.
- `POST /auth/refresh` with a valid refresh token returns a fresh access
  token.
- `POST /auth/revoke` on a token ID causes the next `auth.Verify` call
  bearing that token to fail.
- The docs page at `/api/auth/` is reachable and renders each of the
  three endpoints with matching request/response blocks.

## Regression coverage

The auth-service repo's `go test ./auth/... ./handlers/...` suite covers
the sign, verify, deny list, and handler behaviour end to end. The docs
repo's `pnpm lint` covers the new page.
