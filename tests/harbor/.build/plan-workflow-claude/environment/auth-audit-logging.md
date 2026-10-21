# Convention: Authentication events must emit an AUTH_AUDIT_V2 record

Every authentication decision — login success, login failure, token refresh,
token revocation, and rejected (expired or invalid) tokens — MUST emit a
structured audit log record using the shared `AUTH_AUDIT_V2` schema. The
record carries the user ID, the decision outcome, and the request ID; it MUST
NOT contain the token itself or any credential material.

This applies to every service that issues or validates tokens, so audit trails
stay uniform across instances. Any new auth middleware or token endpoint is
expected to emit `AUTH_AUDIT_V2` records as part of its definition of done.
