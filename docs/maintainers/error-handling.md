# Maintain Error Handling

Backend failures are classified by the component that understands them. HTTP, lifecycle, or background boundaries record one diagnostic and reuse its reference across every applicable surface.

Do not pass `err.Error()` to HTML or generic structured logs. Safe mappings provide bounded technical categories; the recorder adds defense-in-depth text and structured-JSON redaction.

The administrator-only Error Log shows records from the current process. It is capped at 200 entries and is not audit history. Missing references return `404`. Rendering failures return `500` with a newly correlated reference.

Expected validation and authorization outcomes do not create diagnostics:

- `401` missing or expired session;
- `403` role, origin, or CSRF rejection;
- `404` missing resource;
- `422` field validation.

Unexpected backend failures return `500` with a plain-language recovery step and reference. A server created with incomplete audit finalization returns a persistent `500` partial-completion state that says not to retry.

Docker uses typed safe categories such as unavailable, timeout, permission denied, missing container, oversized bounded-log response, and protocol failure. Request-bound dependency failures normally return `503` with one shared UI/Error Log/structured-log reference. Known missing servers or containers return `404`; a non-Docker server on a Docker route returns `422` without manufacturing a backend diagnostic.

Raw SDK errors, socket paths, authorization material, response bodies, and container-log output never enter Docker diagnostics. Background polling reuses the current reference while a server remains unavailable so a 30-second check does not create repeated Error Log noise.

Relevant invariants: GP-021, GP-026–GP-030, and V1-010.
