# Changelog

## Unreleased

### Fixed

- Enforced bounded, origin-checked, body-only CSRF validation before browser mutations. Query parameters can no longer supply form tokens.
- Distinguished missing sessions and authorization denials from authentication-store failures. Logout and password-change failures now return one safe diagnostic reference without raw database detail or false redirects.
- Split global and per-account login limiting so malformed email attempts consume global capacity without filling the account map.
- Added truthful server-registration audit outcomes. Registration audits the real server ID, finalizes known failures, and renders a persistent do-not-retry partial-completion state when creation succeeds but audit finalization fails.
- Corrected HTMX URL state after sign-in and server creation with explicit redirect/push headers while preserving ordinary `303` form behavior without JavaScript.
- Removed credential-reference input and rendering until an owning integration exists. Server validation now returns normalized typed input that persistence stores unchanged.
- Added a forward migration that clears pre-Phase-4 credential references because no owning integration could have validated them, and prevents user deletion from cascading into audit-history deletion.
- Added structured JSON redaction, correct Error Log `404` behavior, and correlated rendering failures.
- Moved Phase 2/2A/3 dependency construction into `cmd/gopanel`; `internal/app` now owns lifecycle and route mounting only.
- Added lifecycle-owned expired-session cleanup and a strict, hidden-password `create-admin` command.
- Restored the npm lockfile, regenerated Tailwind output, enforced Go formatting in CI, and updated pre-Phase-4 user and maintainer documentation.
- Recorded the narrowly scoped pre-Phase-4 owner sequencing override without reclassifying the unresolved JavaScript-disabled browser check or weakening GP-023, GP-031, and later release verification.

### Security

- Production session cookies now use `__Host-gopanel_session`; development uses a separate loopback-only cookie name. Legacy session cookies are cleared during login, logout, and password changes.
- Added behavioral regression coverage for CSRF source, hostile origins, limiter consumption, raw-error disclosure, logout persistence failure, diagnostic redaction, audit partial completion, and session-cleanup ownership.

## v0.1.0 - 2026-08-24

### Added

- Phase 2 local authentication foundation (GP-007, GP-008, GP-009, GP-010): Argon2id password hashing with dummy hash for enumeration defense, email normalization, 256-bit opaque sessions (SHA-256 hash stored), `HttpOnly`/`Secure`/`SameSiteLax` cookies with `__Host-` prefix in production, 12h session lifecycle, per-account (5/min) + process-wide (20 burst/sec) token-bucket rate limiting without `X-Forwarded-*` trust, `http.CrossOriginProtection` origin check, process-local 32-byte HMAC-SHA256 CSRF (`gopanel-csrf-v1-authenticated` bound to raw session, `gopanel-login-context-v1` 15m bound to anonymous context), `RequireLogin` (401) and `RequireAdmin` (403) middleware, `gopanel user create-admin` CLI, and mobile-first login view (`internal/view/pages/auth/login.templ`).
- Phase 2A operator feedback and Error Panel (GP-021, GP-027, GP-030): Administrator-only `GET /errors` and `GET /errors/{id}` (RequireLogin+RequireAdmin), full-page and HTMX fragment from same `DisplayRecord` view model (`data-request-region`, `hx-get`/`hx-push-url`), process-local 200-entry `diagnostic.Recorder` with `UserID/Action/Target/HTTPStatus/AuditCorrelationID` extension, `sanitizeDetail` redaction, `application.js` swaps `4xx`/`5xx` `text/html` via `shouldSwap=true` and renders `sendError`/`timeout` with fixed transport text, `See Error Log` for admins vs `Contact an administrator` for non-admins, permission/error-reference components, banner “Only most recent 200 retained; oldest evicted; disappears on restart; newest first”, all required panel fields (timestamp, user/system, action/route, target, component, HTTP status, audit/correlation ID, public message, safe technical detail).
- Phase 3 server registration (GP-006, GP-013, GP-014): `servers` and `audit_log` tables (`0003_server_registration.sql`), `audit` primitive `RecordAttempt` (attempted) → `RecordResult` (success|failed) with `WHERE result='attempted'` guard, audit row ID as correlation ID reused in `diagnostic.AuditDiagnosticInput` and structured logs (`audit_id`/`server_id`), `server` package (`model.go`, `validate.go`, `store.go`, `handler.go`) with `ValidateInput` (name 3-64, address hostname/IP without scheme, connection_type ∈ docker/caddy/vault/kubernetes, credential_reference nullable ≤256 opaque until owning integration), `POST /servers` 422 validation returns `422 text/html` fragments that swap cleanly under HTMX, no remote contact on `POST /servers`, mobile-first `ServerListPage/Fragment` (Empty vs Loaded), `ServerDetailPage/Fragment`, `ServerFormPage/Fragment` (labels + help + FormField, `credential_reference` help “not validated until owning integration”), `ServerErrorFragment` with `error_ref` + `See Error Log`.
- Pre-Phase 4 hardening: `internal/diagnostic/audit.go` `AuditDiagnosticInput` helper enforcing correlation, `internal/diagnostic/audit_test.go` invariant `AuditCorrelationID != ""` for `create_server`, `internal/audit/audit_test.go` idempotency `RecordResult` second transition fails, `internal/server/validate_test.go` integration-safe messages (address errors never mention Docker reachability, credential errors never imply semantic meaning), `ErrorNotFoundFragment` no longer echoes raw `{ id }` (generic “matches that reference”), `RequireAdmin` fragment now explicitly states why denied + safe next steps (a) sign in as administrator (b) contact administrator (GP-027), diagnostic handler never logs raw bodies, HTMX detection never bypasses `RequireLogin`/`RequireAdmin`.

### Changed

- Extended `diagnostic.Record` to carry audit correlation fields for Phase 3; `recorder.logRecord` now logs `user_id` and `http_status` alongside `error_ref`.
- `internal/app` now mounts Error Panel and server routes under `RequireLogin+RequireAdmin` groups, sharing the same process-local `diagnostic.Recorder` and auth `CSRF` instance.
- `internal/auth/middleware.go` `RequireAdmin` now returns `text/html` 403 fragment (not `text/plain`) so HTMX correctly swaps denied state; `RequireLogin` guidance now includes “contact an administrator”.
- `internal/store` migrations now 3 (`0001` baseline `SELECT 1`, `0002` users/sessions, `0003` servers/audit_log); `TestEmbeddedMigrationsCreateNoCapabilityTables` now expects `[audit_log schema_migrations servers sessions users]`.
- `internal/diagnostic/error_list.templ` banner and `ErrorDetail` footnote now explicitly state eviction semantics and non-durable availability (“may be evicted after 200 or lost on restart; ordering not stable”).

### Security

- CSRF remains process-local HMAC-SHA256 with domain separation; audit correlation prevents silent `attempted` rows; `POST /servers` is `RequireLogin`+`RequireAdmin`+CSRF protected and never performs generic outbound fetch (typed `POST /servers` only).
- Diagnostic redaction defense-in-depth preserved across new `UserID/Target/AuditCorrelationID` fields; `Error Panel` access security-logged (`event=error_panel_access`), viewer 403 HTML does not echo unsafe input.

### Fixed

- Import cycles `server → serverpages → server` and `diagnostic → diagnosticpages → diagnostic` broken via `DisplayServer`/`DisplayInput` and `DisplayRecord` indirection.
- `audit` FK tests now create a real `users` row before `RecordAttempt` to satisfy `FOREIGN KEY users(id)`.

## v0.0.1 - 2026-08-23

### Added

- Phase 0 scaffolding and Phase 0.75 baseline: `cmd/gopanel` wiring, `internal/app` lifecycle stub, `internal/config` stub, `internal/view` Layout + Button/Badge/Card/Alert/FormField, `web/static` vendored HTMX 1.9.12 (sha256 `449317ade7881e949510db614991e195c3a099c4c791c24dacec55f9f4a2a452`), Tailwind 4.3.3, Node 26.

### Changed

- Removed test-only scaffold-check product behavior before initial release.
- Recorded scoped Phase 0.75 owner override without reclassifying browser evidence or waiving exact-commit CI validation (GP-031).

## v0.0.0 - 2026-08-22

### Added

- Initial GoPanel application scaffold and Phase 1 lifecycle foundation: Go 1.27 + Node 26, pinned Templ 0.3.1020/Chi 5.3.2/modernc/sqlite 1.57.0, embedded forward-only migration machinery (`schema_migrations`), process health `/healthz` + SQLite-backed `/readyz`, validated startup configuration, structured lifecycle diagnostics, bounded graceful shutdown (5s drain), and behavioral lifecycle/migration/config/database-safety/diagnostic concurrency tests.

### Security

- Same-origin vendored browser assets with recorded checksums, server-rendered error responses without raw backend details, external CSP-compatible JavaScript with no inline script.
