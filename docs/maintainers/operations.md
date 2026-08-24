# Operate GoPanel Before Phase 4

This page documents the lifecycle for the current Phase 0–3 application. Local authentication, the process-local Error Log, audit history, and server registration are active. Managed-system integrations remain deferred to Phase 4 and later.

## Startup Order

GoPanel starts in this order:

```text
parse and validate configuration
→ open and verify SQLite
→ apply embedded forward-only migrations
→ construct the HTTP application
→ listen for requests
```

Configuration, database, or migration failure stops startup before listening. Fatal startup failures emit an opaque `error_ref` through the structured terminal log. No browser or Error Panel surface exists at that point.

## SQLite Safety

The configured value is a filesystem path, not a caller-supplied SQLite connection string.

- A missing database is created only at that path and receives mode `0600`.
- An existing healthy database is reused.
- An unreadable, corrupt, or incompatible database stops startup.
- GoPanel does not delete, rename, truncate, overwrite, replace, or automatically repair an existing database.
- SQLite foreign-key enforcement is enabled for every connection opened through the configured data source and is verified during startup.

The migration mechanism owns `schema_migrations`. Phase-owned migrations also create `users`, `sessions`, `servers`, and `audit_log`.

## Migration Policy

Migration files are embedded in the binary and named `NNNN_description.sql`. Versions are positive, contiguous, unique, and applied in numeric order.

Each migration and its metadata record commit in one SQLite transaction. A failed migration rolls back, remains unapplied, and stops startup. GoPanel rejects missing, malformed, duplicate, and unsupported versions and rejects a database whose recorded migration history is newer than or incompatible with the running binary.

Corrections use a new forward migration. There is no automatic rollback, down-migration command, or migration CLI.

Migration `0004_pre_phase_4_integrity.sql` clears credential references written before an owning integration existed because those values were never semantically validated. It also rebuilds `audit_log` with restrictive user deletion so audit history cannot disappear through a cascade.

## Health and Readiness

```text
GET /healthz → 200 alive
GET /readyz  → 200 ready, or 503 not ready
```

`/healthz` reports process liveness. `/readyz` checks that SQLite remains available and fails while shutdown is in progress. Responses are stable plain text and do not expose raw database or filesystem errors.

Readiness deliberately excludes Docker, Caddy, Vault, Kubernetes, and future managed systems. Their reachability will be separate observed state and must never redefine process readiness.

## Structured Lifecycle Events

Current structured event names are:

```text
startup_initiated
configuration_rejected
database_opened
database_open_failed
migration_completed
migration_failed
listening
shutdown_initiated
http_drain_completed
http_drain_deadline_reached
database_closed
shutdown_completed
```

Failure events recorded through the diagnostic foundation include the same opaque `error_ref` as the corresponding process-local diagnostic record. Details are length-bounded and redact common password, token, secret, authorization, cookie, session, API-key, bearer, and URL-credential forms.

Callers pass only bounded, application-owned failure descriptions or error types. They do not pass raw request bodies, authorization headers, credentials, or untrusted workload output and then rely on regex redaction to discover every possible secret. Redaction is defense in depth, not a universal secret classifier.

When a render failure occurs before the response is committed, GoPanel returns safe HTML containing the same error reference and `See Error Log`; HTMX requests receive a swappable HTML fragment with the real `500` status. If response bytes were already committed or the client connection itself failed, GoPanel can record and log the reference but cannot retroactively deliver it to that client.

The buffer retains at most 200 entries, evicts the oldest first, is safe for concurrent access, and disappears when the process exits. Administrators can inspect it at `/errors`; viewer and anonymous requests are denied server-side.

## Graceful Shutdown

`SIGINT` and `SIGTERM` initiate this sequence:

```text
mark shutting down
→ make /readyz return 503
→ stop accepting new connections
→ drain active HTTP requests with http.Server.Shutdown
→ force-close remaining HTTP work at the bounded deadline
→ close SQLite
→ exit
```

The process-level signal context does not become the request context, so healthy in-flight requests are not canceled immediately. The same lifecycle owns periodic expired-session cleanup and stops that loop before SQLite closes. Phase 4 will add the first managed-system status poller.

## Startup Troubleshooting

Use the public message, event name, and `error_ref` together.

- `configuration_rejected`: correct the named field or invocation shape. Confirm the database parent exists and development values are loopback-only.
- `database_open_failed`: inspect file type, ownership, permissions, and integrity. Preserve the original file; do not delete it to make startup succeed.
- `migration_failed`: keep the database unchanged, inspect the matching embedded migration and correlated safe detail, then correct the migration with a deliberate forward change.
- `http_drain_deadline_reached`: identify the request that exceeded the shutdown bound. GoPanel force-closes remaining HTTP work before closing SQLite.

Do not paste credentials, authorization headers, request bodies, secret values, or untrusted workload output into configuration or diagnostic fields.
