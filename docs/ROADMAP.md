# GoPanel — Roadmap & Phase Plan (v4)

## Goal

Ship a maintainable, secure, production-usable **GoPanel v1.0.0** from the current pre-version state without over-engineering the path there.

This roadmap implements `ARCHITECTURE.md`. The architecture is the guardrail; the roadmap is the intended implementation sequence. Normal software development will expose better sequencing, smaller scopes, or practical deviations between phases. Those deviations are acceptable when they are documented, tested, and do not quietly weaken the architecture's core boundaries.

Prefer working vertical slices over speculative refinement. Build the smallest complete capability, verify it, then move forward.

Each phase is a *vertical slice*: it ships a real, usable capability end-to-end (store → service → handler → view, desktop + mobile, HTMX + full page) rather than a horizontal layer. This includes the database: a schema object lands with the capability that owns it, not all at once up front.

Sizing uses relative effort (S / M / L) instead of calendar time. Treat L as roughly 2–3x an S.

**Cross-cutting rule for every phase:** nothing ships without Loading/Empty/Loaded/Error states, server-side authorization, and visible outcomes for success, rejection, denial, timeout, and failure. Fields and controls use clear labels, constrained input explains what is expected, and backend failures receive a correlated error reference. Privileged control operations also require CSRF where applicable and two-phase audit logging. Every phase also requires exact-source evidence, uncached tests, applicable critical negative controls, honest result states, literal browser checks where required, and independent CI against the exact commit. These are part of the definition of done for each slice, not a separate cleanup phase.

---

## Phase 0 — Project Scaffolding (S)

**Objectives**
- Stand up the top-level shape so later phases have a place to land, without pre-building feature packages before they contain code.

**Deliverables**
- `/cmd/gopanel/main.go` wiring stub, `/internal/app/app.go` lifecycle stub
- `/internal/config` package (empty struct + loader stub — validation lands in Phase 1)
- `/internal/view` with base layout and generic components: `Button`, `Badge`, `Card`, `Alert`, `FormField`
- `/internal/store` package stub (SQLite driver wiring; schema lands per-capability starting in Phase 1)
- `/web/static` with Templ + Tailwind + HTMX toolchain wired: `templ generate --watch`, `tailwindcss --watch`, vendored/pinned HTMX with recorded checksum, and a small external project-owned `application.js` loaded by the base layout to swap returned `4xx`/`5xx` HTML fragments
- CI: build, vet, `templ generate` check

**Exit criteria**
- `go run ./cmd/gopanel --dev` boots an empty server rendering the base layout on desktop and mobile viewport.
- `auth/`, `diagnostic/`, `server/`, `container/`, `proxy/`, `secret/`, `cluster/` do not exist yet. Each is created in the phase that implements it, not here.

---

## Phase 0.75 — Baseline, Cleanup & Evidence Gate (S)

**Status:** `CLOSED BY OWNER OVERRIDE`

The owner accepted the residual browser-evidence uncertainty for the exact source recorded in [ADR 0001](./ADR/0001-phase-0-75-owner-override.md). The underlying results remain two `PASS`, two `INCONCLUSIVE`, two `NOT RUN`, and one `NOT APPLICABLE`; the override does not reclassify them or waive GP-031 and exact-commit independent CI validation.

**Objectives**
- Finalize the greenfield toolchain baseline, remove test-only scaffold behavior, and establish the evidence contract required by every later phase.

**Deliverables**
- User-approved Go `1.27.0` minimum and CI baseline; Node `26` remains development-only
- Retain Templ `v0.3.1020`, Chi `v5.3.2`, modernc SQLite `v1.57.0`, Tailwind `4.3.3`, and HTMX `1.9.12`
- Remove the production scaffold-check route, form, handlers, view model, templates, and feature-only tests
- Verify vendored HTMX bytes against the identified `htmx.org@1.9.12` npm package artifact
- Complete literal mobile, desktop, and JavaScript-disabled browser checks
- Add GP-031 and its source-correlated, uncached, negative-control, result-state, and exact-commit CI requirements
- Extend the existing workflow with source identity, uncached tests, skipped-test visibility, checksum verification, and final clean-worktree checks

**Exit criteria**
- No product behavior exists solely to demonstrate a test.
- All required local checks and critical negative controls have inspectable exact-source evidence.
- Literal browser requirements pass at mobile and desktop sizes with and without JavaScript as required.
- The exact committed source passes the protected CI workflow before Phase 0.75 is complete.

The owner override closes this phase for sequencing despite the retained browser exceptions above. It does not represent satisfaction of the affected exit criteria.

**Explicitly out of scope**
- Any Phase 1 lifecycle, migration, readiness, or diagnostic behavior.

---

## Phase 1 — Lifecycle + Migration Machinery (M)

**Status:** `LOCAL PASS — AWAITING EXACT-COMMIT CI`

**Objectives**
- Establish the process lifecycle and the migration/storage *mechanism* everything else depends on — not the complete v1 schema. No table is created here that no capability yet owns.

**Deliverables**
- Embedded, versioned migration runner: ordered, fatal-on-failure, reversible only through deliberate future migrations; `PRAGMA foreign_keys = ON`
- SQLite first-run behavior is explicit:
  - configured database does not exist → first-run creation is allowed
  - configured database exists and opens normally → use it
  - configured database exists but is unreadable, corrupt, or incompatible → fatal startup error
  - GoPanel never renames, overwrites, deletes, or silently replaces an existing database
- `Config.Validate() error` — no schema framework, a plain validation function. At minimum: port is valid, database path is usable, `public_url` parses, required directories/files exist, unknown/unsupported configuration fails clearly. Later modules (Caddy, Vault, Kubernetes, Docker) add their own startup validation in their own phases.
- `/healthz` (process is alive) and `/readyz` (SQLite is available and GoPanel can serve requests). Readiness must never depend on Docker/Caddy/Vault/Kubernetes reachability.
- Structured startup/shutdown logging: started, migration failed, listening, shutting down, drain timeout reached, database closed
- `internal/diagnostic` recording foundation: opaque error reference, safe diagnostic record, application/SQLite error mapping, defense-in-depth redaction, bounded 200-entry process-local buffer, and structured-log correlation. The contract requires later integrations to add their own safe error mappings; no browser route exists before authentication.
- Graceful shutdown per Architecture §8.4 (stop poller → stop accepting requests → bounded drain → cancel background work → close SQLite → exit) using `http.Server.Shutdown`
- Process lifecycle and diagnostic-buffer tests (start with zero migrations, start with a deliberately broken migration, SIGTERM during idle, SIGTERM during in-flight request, invalid config, buffer eviction at 200 entries, reset on restart, redaction)

**Exit criteria**
- With zero application tables defined, the migration runner still runs cleanly against an empty database and fails loudly against a broken one.
- Invalid configuration fails before the HTTP server begins accepting requests and produces one actionable startup error.
- Every startup or request-bound backend failure recorded through the diagnostic foundation receives one error reference shared with its structured log entry.
- The diagnostic buffer never exceeds 200 entries and never retains an unfiltered error, credential, token, secret value, authorization header, raw request body, or untrusted workload output.
- Killing the process with SIGTERM during an active request drains it within the bounded deadline instead of dropping it.
- `/readyz` reports ready with Docker/Caddy/Vault/Kubernetes all unreachable, as long as SQLite is available.

**Explicitly out of scope**
- `users`, `sessions`, `servers`, and `audit_log` tables — `users`/`sessions` arrive in Phase 2; `servers` and `audit_log` arrive in Phase 3.
- Any durable `error_log` table. The v1 Error Panel is bounded and process-local.
- Backup subsystem (Architecture §15 — no backup subsystem in v1 by design; backup/restore is instead a documented *drill*, verified in Phase 9).

---

## Phase 2 — Local Authentication (M)

**Status:** `LOCAL PASS — OWNER BROWSER-EVIDENCE OVERRIDE — AWAITING EXACT-COMMIT CI`

**Objectives**
- Ship the only identity model v1 needs: local email/password with opaque sessions, plus a decision on how the first admin comes to exist.

**Deliverables**
- `internal/auth` package (created here, not in Phase 0)
- Migration adding `users` and `sessions` (schema per Architecture §6.2) — the first application tables added to the machinery built in Phase 1
- Argon2id password hashing — normative, not "Argon2id or bcrypt." Parameters defined once in code/config.
- Identical login-failure message for unknown user vs. wrong password
- Opaque 256-bit session tokens, SHA-256 hash stored server-side, never the raw token
- Session cookies: `HttpOnly`, `Secure`, `SameSite=Lax`+, rotated on login, invalidated on logout. Production must require `Secure`. `--dev` may permit a non-Secure session cookie, and only when serving on loopback — otherwise the documented `go run ./cmd/gopanel --dev` workflow and local HTTP-only development would conflict with the cookie policy.
- **Rate limiting respects the proxy trust model.** GoPanel sits behind Caddy/Nginx and deliberately does not trust `X-Forwarded-*` (Architecture §9.8), so a naive per-IP limiter would usually treat every user as the same client. v1 uses:
  - a **per-account** limiter with short recovery for targeted brute-force protection, plus
  - a small **process-wide token-bucket style limiter** with bounded burst and automatic recovery to limit aggregate login abuse without creating a long global lockout.

  No trusted-proxy header parsing is added just to recover client IPs. If real client-IP forwarding becomes useful later, it is implemented deliberately in v1.1 with an explicit trusted-proxy list — never by trusting `X-Forwarded-For` simply because it is present.
- Password change invalidates all sessions for that user
- Periodic expired-session cleanup
- Process-local 32-byte HMAC-SHA-256 CSRF key; authenticated tokens bind to the raw session credential and login tokens bind to a signed 15-minute anonymous context
- Login context survives credential failure and multiple tabs, is destroyed on success, and never becomes the new session
- `http.CrossOriginProtection` as defense in depth with mandatory visible full-page and HTMX token validation failures
- **Initial-admin bootstrap via CLI**: `gopanel user create-admin`, prompting for email/password locally. No default credentials, no public signup page, no environment-variable password.
- Mobile-first login page (full page + HTMX-enhanced)

**Exit criteria**
- `RequireLogin` / `RequireAdmin` middleware exist and are enforced server-side, not just hidden in the UI.
- Login flow works correctly with JavaScript disabled.
- A fresh install has no way to reach an authenticated session except through `gopanel user create-admin`.
- Repeated failed logins against one account are throttled even when every request appears to originate from the same proxy-forwarded address.
- Missing, forged, expired, cross-context/session, rotated, revoked, and pre-restart forms fail before authentication or mutation; JavaScript-disabled forms enforce the same contract.

**Explicitly out of scope**
- OIDC (Architecture §15 — deferred past v1).
- Any RBAC beyond the two roles (`admin`, `viewer`).
- Trusted-proxy / real-client-IP handling (deferred to v1.1 if needed).

---

## Phase 2A — Operator Feedback & Error Panel (S)

**Status:** `LOCAL PASS — OWNER BROWSER-EVIDENCE OVERRIDE — AWAITING EXACT-COMMIT CI`

*Depends on Phase 2 because diagnostic detail is administrator-only.*

**Objectives**
- Turn the diagnostic foundation into a safe operator workflow before server registration introduces normal control-panel forms and actions.

**Deliverables**
- Administrator-only `/errors` list and `/errors/{id}` detail routes using `RequireLogin` and `RequireAdmin`
- Full-page and HTMX renderings from the same prepared diagnostic view models
- Small external project-owned handling for `htmx:sendError` and `htmx:timeout`, rendering a persistent generic transport failure without inventing an error reference
- `See Error Log` links for administrators and an error reference plus `Contact an administrator` guidance for other users
- Visible sign-in prompt for missing/expired sessions and plain `Administrator access is required` denial for viewer-role requests
- Reusable presentation components for permission denial, error reference, persistent resource error, concise field help, and inline validation
- Every control has a visible label or accessible name; placeholder text and icons do not replace labels
- Error Panel fields: timestamp, associated user or `system`, action/route, target when known, component/integration, HTTP status, audit/correlation ID when present, public message, and safe technical detail
- Error Panel access is security-logged; panel entries clearly state that they cover only the current process and reset on restart

**Exit criteria**
- A user-initiated test failure is visible in full-page, HTMX, and JavaScript-disabled flows and carries the same error reference as its Error Panel and structured-log records. HTMX swaps the returned non-`2xx` fragment into the intended target.
- A viewer cannot list or retrieve Error Panel records server-side, regardless of UI state.
- A denied action explains why it cannot proceed and the safe next step; it is never a silent no-op.
- Redaction tests prove that credentials, session tokens, secret values, authorization headers, raw request bodies, unsafe paths, and untrusted workload output do not reach the Error Panel.

**Explicitly out of scope**
- Durable in-app error history, configurable retention, export, search platform, or external log aggregation.

---

## Phase 3 — Server Registration (M)

**Status:** `LOCAL PASS — OWNER BROWSER-EVIDENCE OVERRIDE — AWAITING EXACT-COMMIT CI`

**Objectives**
- Let an operator register server identity and connection type without requiring integration modules that do not exist yet and without coupling registration to external system availability.

**Deliverables**
- `internal/server` package (created here)
- Migrations adding `servers` and `audit_log`. Server registration is the first authenticated privileged control operation that changes durable GoPanel configuration, so the audit primitive must exist here.
- Small concrete `RecordAttempt` / `RecordResult` audit primitive with only `attempted -> success | failed` transitions. It is not a generic mutation framework.
- **Server identity stays separate from integration-specific credentials.** Phase 3 owns and validates: server ID, name, address, and connection type.
- `credential_reference` is nullable at this stage. It is only populated after the owning integration exists and can semantically validate the reference.
- `POST /servers` validates and persists registration data only. It does not contact the remote system. It records `attempted` before the accepted configuration change and resolves the audit row when the result is known.
- Server list + server detail views, full page + HTMX fragment on the same URL (Architecture §8.5)
- Loading / Empty / Loaded / Error states for the server list
- Visible labels and concise expected-input guidance for server name, address, and connection type; field errors state what acceptable input looks like and preserve safe entered values
- Mobile-first registration form and list/card views

**Exit criteria**
- A user can register a server while the target system is offline.
- Phase 3 does not introduce a generic network test, generic credential resolver, or integration-specific client.
- If `credential_reference` is present in durable state, it was set and validated by an integration phase that owns its meaning.
- An audit-insert failure prevents the server configuration change; a final audit-update failure leaves `attempted`, creates a high-severity diagnostic with the same correlation ID, and never produces a silent UI result.

---

## Pre-Phase-4 Repair Evidence

**Status:** `LOCAL PASS — OWNER BROWSER-EVIDENCE OVERRIDE — AWAITING EXACT-COMMIT CI`

The [owner browser-evidence override](./ADR/0002-pre-phase4-javascript-disabled-browser-override.md) permits sequencing only after every other pre-Phase-4 requirement and exact-commit CI passes. Literal JavaScript-disabled browser verification remains `NOT RUN`; the override does not satisfy that exit criterion or weaken GP-023 or GP-031.

Phase 4 is not authorized while exact-commit CI remains pending.

---

## Phase 4 — Docker Read-Only (M)

**Objectives**
- First real infrastructure integration, read-only, proving the client → service → handler → view path end to end, and establishing conventions the other read-only integrations (Vault, Kubernetes) will reuse. Also the phase that gives Docker's `credential_reference` semantic meaning, per Phase 3.

**Deliverables**
- `internal/container` package (created here)
- Typed Docker SDK client.
- Docker-owned semantic validation for its connection configuration. If Docker requires a durable `credential_reference`, Docker validates it before storing/updating it.
- Docker-owned safe diagnostic mapping from SDK failures into redacted technical detail; raw Docker errors never enter the Error Panel or structured log.
- `POST /servers/{id}/test-docker` using the typed Docker client — `RequireLogin`, `RequireAdmin`, CSRF, bounded timeout, stable error mapping, and structured/security logging for failed or suspicious attempts. No generic `TestConnection(address, type)` helper.
- `ListContainers(ctx, serverID)` with bounded timeout context (Architecture §8.1)
- `CheckStatus` health check integrated into the 30-second poller → in-memory `CachedStatus` (Architecture §8.2, §6.3)
- **Bounded concurrency on the poller.** At N servers, "every 30s, check every connection" can turn into a connection burst. Use a semaphore/channel capping simultaneous checks (e.g. max 5–10 in flight) — not a queue system or scheduler framework.
- Bounded log retrieval (`ViewLogs`, last 100 lines — Architecture §8.3)
- **`ViewLogs` is admin-only in v1, not viewer-accessible.** Read-only does not mean low-risk: GoPanel cannot guarantee arbitrary container output is free of tokens, database URLs, authorization headers, or other accidentally-logged secrets. Document explicitly: *container logs are untrusted, potentially sensitive application output.* Do not broaden the architecture's "secret values are never rendered" guarantee (Architecture §9.6, which is specifically about Vault-resolved secrets) to imply logs are scrubbed of secrets — GoPanel cannot make that guarantee for Docker log output. Viewer access to logs can be reconsidered later once there's a concrete need.
- Desktop table + intentionally-designed mobile card for containers (Architecture §10.5 — not a mechanical table→card transform)
- "Docker connected / Checked 18 seconds ago" freshness indicator

**Exit criteria**
- Killing the GoPanel process and restarting it shows a fresh poller check, not stale cached state presented as truth.
- ICMP-only host reachability is never conflated with "Docker connected" (Architecture §8.2).
- With 20+ registered servers, the poller never opens more than the configured concurrency limit of connections at once.
- A viewer-role user cannot reach `ViewLogs`, server-side, regardless of UI state.
- Docker connection testing uses the Docker client itself and cannot be repurposed into arbitrary outbound requests.

**Explicitly out of scope**
- Any container mutation (start/stop) — that's Phase 5.
- Live/streaming logs (Architecture §15 — no unbounded live-log buffering in v1).

---

## Phase 5 — First Docker Mutation + Audit (L)

**Phase 5 is the canonical managed-system mutation implementation. Later managed-system mutation paths must copy its security and audit semantics, not invent new ones.**

**Objectives**
- Establish the shape every later mutation replicates: authorization, CSRF, confirmation, audit, and honestly-defined repeat behavior — with an audit guarantee that is actually true in every code path, including crash and write-failure cases.

**Deliverables**
- Reuse the Phase 3 `audit_log` schema and concrete `RecordAttempt` / `RecordResult` primitive. Do not introduce a second audit path or generic mutation framework.
- **Two-phase audit write across SQLite and Docker**, so every accepted managed-system mutation attempt is logged even when persistence itself fails partway through:
  1. Insert audit row with `result = attempted`
  2. If that insert fails, do not execute the infrastructure mutation — reject the request before anything happens externally
  3. Execute the infrastructure operation
  4. Update the row to `result = success` or `result = failed`
  5. If the update fails, the row remains `attempted` and a high-severity structured log is emitted

  Schema: `result` is `attempted | success | failed`. A row stuck at `attempted` means *GoPanel accepted this operation but its final outcome could not be durably established* — strictly better evidence than the alternative of no row existing at all. No distributed transaction is introduced.
- **Audit ID doubles as the mutation correlation ID.** Structured logs produced while executing the accepted mutation include the audit row ID. This lets an `attempted` audit row be correlated with infrastructure and audit-update logs without adding a tracing subsystem.
- One explicit, safe mutation path (e.g. `StopContainer`) — server-side `RequireAdmin`
- CSRF token on every mutating form (Architecture §9.4)
- Named destructive confirmation UX: "Stop container nginx?", not a generic "Are you sure?" (Architecture §10.7)
- No automatic retry on the write path (Architecture §8.1, §15)
- Authorization rejections logged separately as security events, not audit rows (unaffected by the two-phase write above, since no operation was accepted)
- **Explicitly understood repeat behavior**, not a UI-level idempotency guarantee. A disabled button while a request is in flight reduces *accidental* repeat submissions; it does not prevent network retries, two open tabs, deliberate repeated POSTs, or resubmission after JS failure. Define repeat behavior per operation instead of introducing generic idempotency keys:
  - Stop container → repeat is acceptable
  - Start container → repeat is acceptable
  - Create route (Phase 6) → repeat may NOT be acceptable
  - Delete route (Phase 6) → repeat should produce stable not-found/already-deleted behavior
- Persistent, resource-scoped error state with "Try Again" on failure (Architecture §10.8)
- One error reference shared by the resource error, Error Panel entry, audit correlation, and structured backend log
- Mobile action UX for the mutation (large tap target, immediate vs. destructive distinction)

**Exit criteria**
- A stop action that fails mid-flight produces exactly one audit row, ending at `result: failed`, and the UI shows a persistent error next to the affected container — not a toast.
- A stop action where the infrastructure call succeeds but the final audit update fails leaves a row at `result: attempted` plus a high-severity log — never a silently missing row and never a falsely reported failure.
- The mutation pattern, including the two-phase audit write, is documented with concrete handler, service, audit, and UI conventions that Phase 6 reuses directly.

---

## Phase 6 — Caddy (M)

*Depends on Phase 5 — this phase applies the mutation/audit/CSRF/confirmation pattern to a second resource and owns Caddy-specific connection validation.*

**Objectives**
- Reuse the established invariants from Phase 5, including the two-phase audit write; do not introduce a generic mutation framework.

**Deliverables**
- `internal/proxy` package (created here); semantic validation of Caddy connection configuration and any Caddy-owned `credential_reference` before persistence/use.
- Caddy-owned safe diagnostic mapping from client failures into redacted technical detail.
- `POST /servers/{id}/test-caddy` using the typed Caddy client — admin-only, CSRF-protected, bounded, stable error mapping, and no generic outbound request helper.
- Explicit named operations only, stated exactly rather than as a broad abstraction: `ListRoutes`, `CreateRoute`, `DeleteRoute` (an `UpdateRoute` can be added later if a concrete need appears)
- Destructive confirmation naming the target: "Delete proxy route api.example.com?"
- Reads via GET, mutations via POST — no PUT/PATCH/DELETE required (Architecture §9.4, §15)
- Per-operation repeat behavior defined per the Phase 5 rule (create likely not repeat-safe; delete should be idempotent in effect)

**Exit criteria**
- Route creation and deletion use the same two-phase audit write as Phase 5, ending at `success`/`failed`/`attempted` as appropriate.
- Caddy admin endpoint is read only from application configuration, never a user-supplied URL (Architecture §9.5).

---

## Phase 7 — Vault (Conditional, M)

*Depends on Phase 4 (read-only client conventions). Does not depend on Phase 5 or 6.*

**This phase requires a decision before implementation starts.** The architecture document lists Vault as an integration point, but the roadmap currently has no consuming operation for a resolved secret. Building "read a reference → resolve server-side → discard" with nothing that uses the value produces an abstract Vault browser that can't display anything, not useful functionality.

Choose one before starting:
- **Define a concrete v1 use case** — e.g., using a Vault reference as a credential when executing a specific GoPanel operation — and implement only that consumer, or
- **Defer Vault out of v1 entirely** until a concrete consuming operation exists elsewhere in the product. This is the preferred default: it reduces v1 surface area substantially and avoids building an integration solely because it appears in the architecture document.

**If undertaken, deliverables**
- `internal/secret` package (created here)
- Vault-owned semantic validation of any Vault connection/credential reference before persistence/use.
- Vault-owned safe diagnostic mapping that cannot include resolved secret values, tokens, or authorization headers.
- If a connection test is useful for the concrete Vault consumer, expose it as a Vault-specific POST using the typed Vault client with admin auth, CSRF, bounded timeout, and stable error mapping — never a generic network test.
- Secret reference selection UI → server-side resolution → use by the concrete consuming operation → discard
- No `GetSecret(path)` value ever passed to a Templ view (Architecture §9.6)
- Secret values excluded from HTML, browser memory, audit records, logs, and error messages
- Reference use audited (reference + action, never the value) where required
- Vault address and token-file location come from application configuration only — never a database-stored arbitrary path (Architecture §9.5)

**Exit criteria (if undertaken)**
- A named, real GoPanel operation actually consumes the resolved secret — this phase does not ship without one.
- Grep/log review confirms no code path renders or logs a raw secret value.

---

## Phase 8 — Kubernetes Read-Only (M)

*Depends on Phase 4 (read-only client conventions) only — not on Phase 5. Kubernetes owns validation of its allowed context configuration.*

**Objectives**
- Extend read-only infrastructure visibility to clusters with a deliberately narrow resource surface.

**Deliverables**
- `internal/cluster` package (created here), `client-go` restricted to `allowed_contexts` from `config.yaml`.
- Kubernetes-owned semantic validation for the selected context before persistence/use.
- Kubernetes-owned safe diagnostic mapping from `client-go` failures into redacted technical detail.
- `POST /servers/{id}/test-kubernetes` using `client-go` — admin-only, CSRF-protected, bounded, and restricted to configured `allowed_contexts`.
- `ListPods(ctx, namespace)`, deployment listing
- Desktop + mobile pod/deployment views with the standard four UI states

**Exit criteria**
- Attempting to use a kubeconfig context not in `allowed_contexts` is rejected before any API call is made.
- **Only Pods and Deployments are v1 resources.** No cluster explorer, generic resource browser, or YAML inspector.

**Explicitly out of scope**
- Any Kubernetes mutation (Architecture §15 — read-only in v1; no operator, no event framework).

---

## Phase 9 — Release Verification & Hardening (M)

*Depends on all completed capability phases (0–6, including 2A; 8; and 7 if undertaken).*

This phase should primarily answer "did we actually obey the architecture," not "can we now make this safe." Most security and reliability controls must already exist before this phase starts — it is a verification pass, not the place unfinished security work gets postponed.

**Deliverables**
- Security headers finalized: CSP (`default-src 'self'`, no inline JS), `X-Content-Type-Options`, `Referrer-Policy` (Architecture §9.7)
- Reverse-proxy trust review: confirm no security-sensitive logic derives from `Host` / `X-Forwarded-*` headers; `public_url` used explicitly where an absolute origin is needed (Architecture §9.8)
- Error-handling audit: confirm no raw Docker/Caddy/Vault/Kubernetes/SQLite error ever reaches the browser (Architecture §9.9)
- No-silent-error audit: every representative user action visibly reports success, rejection, denial, timeout, or failure in full-page, HTMX, and JavaScript-disabled flows
- Error Panel audit: confirm `RequireAdmin`, 200-entry bound, oldest-entry eviction, reset-on-restart disclosure, required diagnostic fields, security logging of access, and error-reference correlation
- Diagnostic redaction audit: confirm unfiltered credentials, session material, secret values, authorization headers, raw request bodies, unsafe paths, and untrusted workload output never reach the Error Panel or structured log
- Form clarity audit: every control has a visible label or accessible name; constrained fields explain expected format, unit, range, or allowed choice; invalid input produces field-specific correction without echoing unsafe values
- Full mobile + desktop pass across the Error Panel and every resource view built in Phases 3–8
- HTTP/HTMX error-status convention audit (422/403/404/502-503/500 per Architecture §8.6)
- Frontend dependency audit: every vendored asset has an exact pinned version + recorded checksum (Architecture §12)
- **Backup/restore drill**: stop GoPanel, copy SQLite + config, restore them, start GoPanel, confirm users/servers/audit history survive. Documented, not automated — no backup subsystem is built (Architecture §15).
- **File permissions, made concrete**: for files GoPanel owns, `0600` on the SQLite database and on any credential/token files (especially Vault token files, if Phase 7 was undertaken). `config.yaml` contains no secrets, so it can follow normal operator ownership, provided it isn't writable by untrusted users.
- **Database integrity behavior confirmed**: first-run creation works when the configured database does not exist; an existing unreadable/corrupt/incompatible database causes fatal startup and is never renamed, overwritten, deleted, or silently replaced.
- **Structured logging confirmed**: startup/shutdown events (from Phase 1) are present and actionable in practice, not just in code.
- **Browser test baseline** — a representative end-to-end matrix, not every condition against every workflow. Cover login, server registration, container list, and container stop, applying only the failure modes relevant to each: e.g. CSRF failure belongs on registration and stop, not on container listing; session-expired-mid-request belongs on any authenticated action; JS-disabled and mobile/desktop viewport apply broadly. The goal is representative coverage, not a full Cartesian product of workflows × conditions.
- Walk the full Architecture §16 invariants list as a release checklist

**Exit criteria**
- Every item in Architecture §16 (v1 Architecture Invariants) can be pointed to a concrete implementation or test.
- Every item in Architecture §15 (What We Will Not Do in v1) is confirmed absent from the codebase, not just unplanned.
- The backup/restore drill has been executed at least once, successfully.

---

## Dependency Picture

```text
Phase 0 — Scaffolding
   ↓
Phase 0.75 — Baseline, Cleanup & Evidence Gate
   ↓
Phase 1 — Lifecycle + Migration Machinery
   ↓
Phase 2 — Local Authentication (users, sessions)
   ↓
Phase 2A — Operator Feedback & Error Panel (process-local diagnostics)
   ↓
Phase 3 — Server Registration (servers, audit_log; identity + connection type)
   ↓
Phase 4 — Docker Read-Only (Docker-owned config validation + test)
   ├───────────────┐
   ↓               ↓
Phase 5            Phase 8 — Kubernetes Read-Only (K8s-owned validation + test)
Docker Mutation
(reuse audit_log)
   ↓
Phase 6 — Caddy (Caddy-owned validation + test)

Phase 7 — Vault (conditional on a concrete consuming workflow;
           depends only on Phase 4, not on Phase 5 or 6;
           Vault owns its own validation)

All completed capabilities
   ↓
Phase 9 — Release Verification & Hardening
```

## Sequencing Notes

- **The roadmap is directional, not ceremonial.** If implementation exposes a smaller or safer path, adjust the phase, document the deviation, preserve the architecture boundary, and keep moving. Do not reopen settled design questions without evidence from the code or tests.
- **Phase 0.75 gates every implementation phase.** Later phases are incomplete without GP-031 evidence tied to the exact source and independent CI run.
- **Phases 1, 2, 2A, 3, and 4 are strictly linear** — each is a hard dependency of the next.
- **A schema object lands with the capability that owns it.** Phase 1 builds migration machinery only; `users`/`sessions` arrive in Phase 2; `servers` and `audit_log` arrive in Phase 3 because server registration is the first authenticated privileged control operation that changes durable configuration. No table is created before something exists to populate or read it correctly.
- **The Error Panel is not a durable state store.** Phase 1 creates a fixed 200-entry process-local buffer; Phase 2A exposes it only after administrator authorization exists. Cross-restart diagnostics belong to structured backend logs in v1.
- **Integration-specific credential references arrive with the integration that owns them.** Phase 3 registers server identity and connection type. Docker, Caddy, Vault, and Kubernetes each validate their own connection/credential configuration before it becomes durable or usable. No generic credential resolver or generic connection tester is introduced.
- **Phase 5 gates Phase 6 only.** Caddy reuses the mutation/audit/CSRF/confirmation pattern Phase 5 establishes, so it has a real technical dependency on Phase 5. Kubernetes read-only (Phase 8) and Vault (Phase 7) do not — both depend only on Phase 4's read-only client conventions.
- **Phases 7 and 8 are independent of each other and of Phase 5/6**, and can be reordered or parallelized across contributors — none of them depends on the others (services do not become an orchestration graph, per Architecture §4).
- **Phase 9 is a verification pass, not a first pass.** Security and reliability controls are built incrementally in the phases that introduce them; Phase 9 confirms they were actually done, and adds the handful of release-only concerns (backup drill, file permissions, browser baseline) that don't belong to any single feature phase.

## Risk Watch List

- **Phase 5 scope creep.** It's tempting to build "the mutation system" abstractly here. Keep it to one concrete operation (stop container) — the goal is a proven, copyable pattern, not a framework (consistent with "No generic table abstraction" / "No plugin architecture" in Architecture §15).
- **Silent HTMX failures.** A fragment request that returns a status without replacing the target with a visible error is incomplete. Full-page, HTMX, and JavaScript-disabled paths must show the same outcome and error reference.
- **Error Panel becomes a secret browser.** Administrator-only access does not make unfiltered SDK errors safe. Integrations map errors into safe diagnostics; the recorder performs defense-in-depth redaction; container logs and raw request bodies never enter the panel.
- **Error Panel becomes a logging platform.** Keep v1 to the current process, 200 entries, newest-first list, and exact-reference detail. Do not add durable retention, exports, aggregation, or search infrastructure without a demonstrated need and an architecture change.
- **Vault shipped without a consumer.** Do not build Phase 7 merely because Vault appears in the architecture document. If no concrete consuming operation exists when Phase 7 comes up, defer it.
- **Mobile as an afterthought.** Every phase above lists mobile explicitly because Architecture §10.1 requires it in the same slice, not a cleanup pass.
- **Credential reference validation drift.** Each integration owns semantic validation of its connection/credential configuration. Do not centralize this into a generic credential resolver or generic network test (Architecture §9.5, §9.6).
- **Phase 9 as a dumping ground.** If a security or reliability item from an earlier phase isn't actually done, it doesn't get silently deferred into "we'll harden it in Phase 9" — Phase 9 exit criteria assume the work already happened and only checks it.
- **Overbroad secrecy claims.** Do not let "secret values are never rendered" (which is specifically about Vault-resolved secrets, Architecture §9.6) get restated as a general claim that GoPanel output is free of secrets — Docker log output in particular cannot carry that guarantee, which is why `ViewLogs` is admin-only.
