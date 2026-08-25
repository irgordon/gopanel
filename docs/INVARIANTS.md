# GoPanel — Invariants

## 1. Purpose

This document states what must remain true while GoPanel is built and changed.

An invariant is not a roadmap commitment. The roadmap may change sequence or scope, but it may not quietly weaken these constraints.

GoPanel currently contains the Phase 1 lifecycle and storage foundation, Phase 2 local authentication, Phase 2A operator diagnostics, Phase 3 audited server registration, and the local Phase 4 Docker read-only slice. These invariants are requirements for applicable current and future code, not claims that deferred capabilities already exist.

[`ARCHITECTURE.md`](./ARCHITECTURE.md) remains the technical source of truth. This file is its testable constraint register. If the two disagree, implementation stops until both documents are reconciled. [`ROADMAP.md`](./ROADMAP.md) does not override either document.

---

## 2. Terms

**Managed system:** Docker, Caddy, Vault, Kubernetes, or another external system operated through GoPanel.

**Observed state:** A time-bounded reading from a managed system, such as container status or dependency health.

**Privileged control operation:** An authenticated operator action that changes managed-system state, changes GoPanel's durable server or connection configuration, or resolves or uses a reusable secret.

Authentication and session housekeeping, health polling, migrations, pure reads, and the local first-admin bootstrap are outside this definition. Security-relevant events outside the definition still require security or operational logging.

**Accepted operation:** A privileged control operation that has passed authentication, authorization, CSRF checks where applicable, and input validation, and that GoPanel has decided to attempt.

**Fail closed:** When a required security or audit precondition cannot be established, GoPanel refuses the protected operation.

---

## 3. Core Architecture Invariants

These rules apply to every phase and integration unless the architecture is deliberately revised.

### 3.1 Authority and data ownership

| ID | Must remain true | Prevents | Minimum proof |
| --- | --- | --- | --- |
| **GP-001** | Docker owns container state; Caddy owns route state; Kubernetes owns pod and deployment state; Vault owns secret values. GoPanel never presents a durable shadow copy as authoritative. | Conflicting sources of truth after out-of-band changes or restarts. | Schema review and integration tests. |
| **GP-002** | GoPanel's durable state is limited to validated application configuration, users and password hashes, hashed session metadata, server and connection metadata, opaque module-owned credential references, and audit history. Reusable secret values are excluded. | GoPanel becoming an infrastructure state store or credential database. | Migration review and targeted secret-flow tests. |
| **GP-003** | Health and status observations are process-local, disappear on restart, and show when they were checked. Missing, stale, or failed observations are not presented as current truth. | Operators acting on stale status. | Restart, freshness, and stale-state tests. |
| **GP-004** | The browser communicates with GoPanel only. It never receives infrastructure credentials or directly calls privileged Docker, Caddy, Vault, or Kubernetes endpoints. | Client-side bypass of server policy and credential disclosure. | Browser/network inspection and tests. |
| **GP-005** | Every infrastructure capability is named and typed. GoPanel has no generic request, arbitrary URL fetch, generic resource action, generic connection test, or generic credential resolver. | SSRF and parameter-driven expansion of authority. | API review and negative capability tests. |
| **GP-006** | The module that owns a value validates it before durable storage or use. Server registration validates identity, address, and known connection type; each integration validates its own configuration and credential references. User input never becomes an arbitrary URL, socket, path, environment-variable name, or kubeconfig context. | Network, filesystem, environment, and credential-boundary escape. | Owner-module validation and reject-before-call tests. |

### 3.2 Identity, request, and secret boundaries

| ID | Must remain true | Prevents | Minimum proof |
| --- | --- | --- | --- |
| **GP-007** | Passwords use Argon2id. Login failures do not reveal whether an account exists. Sessions use at least 256 bits of cryptographic randomness; SQLite stores only SHA-256 token hashes. Password changes invalidate all sessions for that user. | Password disclosure, account enumeration, and reusable stolen session records. | Authentication and invalidation tests. |
| **GP-008** | Session cookies are `HttpOnly`, `Secure` in production, `SameSite=Lax` or stricter, rotated on login, expired within a fixed lifetime, and invalidated server-side on logout. A non-`Secure` development cookie is allowed only on loopback. | Session theft and unsafe development settings reaching a network listener. | Cookie and configuration tests. |
| **GP-009** | Every protected route authenticates the caller and enforces the required role on the server. Hidden UI controls are never authorization. Rejections stop the operation and create security-log events, not accepted-operation audit rows. | Direct-request privilege bypass. | Anonymous, viewer, and admin route tests. |
| **GP-010** | `GET` is read-only. Every browser mutation uses `POST`, an HMAC-SHA-256 token signed by a process-local random key, exact domain separation, and binding to the raw authenticated session or signed 15-minute anonymous login context. Token validation remains mandatory when origin metadata is missing or allowed. Invalid, expired, cross-context, cross-session, rotated, revoked, or pre-restart forms fail before protected behavior and return visible full-page or HTMX `403` HTML. No CSRF key, context, or token is persisted or logged. | Cross-site requests, login CSRF, replay across browser contexts, prefetch side effects, and hidden writes. | Full-page and HTMX tests for valid, malformed, expired, forged, wrong-domain, cross-context/session, restart, origin, rotation/revocation, cookie, and JavaScript-disabled behavior; targeted negative controls. |
| **GP-011** | Security-sensitive origins, callback URLs, and client identities do not come directly from `Host` or `X-Forwarded-*`. Absolute public origin uses validated configuration. Forwarded client identity requires an explicit trusted-proxy configuration. | Spoofed origin, callback, rate-limit, or audit decisions. | Hostile-header and configuration tests. |
| **GP-012** | A reusable secret is resolved server-side, used only for its named purpose, and discarded. Its value never enters HTML, intentional browser memory, audit records, structured logs, or user-facing errors. Container logs are separately treated as potentially sensitive untrusted output. | Credential disclosure and false secrecy claims about workload logs. | Data-flow review, render/log tests, and container-log authorization tests. |

### 3.3 Privileged operations and audit truth

| ID | Must remain true | Prevents | Minimum proof |
| --- | --- | --- | --- |
| **GP-013** | Before an accepted privileged control operation runs, GoPanel durably inserts exactly one audit row with `result = attempted`. If insertion fails, the operation does not run. The audit ID is the structured-log correlation ID. | An accepted operation leaving no durable evidence. | Audit-insert failure injection and correlation tests. |
| **GP-014** | The same audit row may transition only from `attempted` to `success` or `failed`, and only when that outcome is known. If the final update fails, it remains `attempted` and a high-severity log records the audit ID. GoPanel never pretends SQLite can roll back a managed system. | False certainty after partial completion. | Success, known failure, timeout ambiguity, and final-write failure tests. |
| **GP-015** | Writes are not automatically retried unless the specific operation is proven safe to repeat. Each operation defines repeat, timeout, already-complete, and not-found behavior. A disabled button is not an idempotency guarantee. | Duplicate or uncertain infrastructure changes. | Operation-specific repeat-behavior tests. |

### 3.4 Reliability and process behavior

| ID | Must remain true | Prevents | Minimum proof |
| --- | --- | --- | --- |
| **GP-016** | Every infrastructure call inherits a context and has a bounded lifetime unless it is an explicitly bounded stream. Services do not escape cancellation with deep `context.Background()` calls. Request handlers do not start unmanaged goroutines. | Hung requests, leaked work, and stalled shutdown. | Timeout and cancellation tests per client operation. |
| **GP-017** | The application lifecycle owns recurring work. Health checks are finite calls. Concurrent work has an owner, cancellation path, and fixed concurrency and buffer bounds. | Per-server timer sprawl, unbounded fan-out, and resource exhaustion. | Concurrency-limit and shutdown-under-polling tests. |
| **GP-018** | Shutdown stops new background checks, stops accepting requests, drains active HTTP work for a bounded period, cancels remaining work at the deadline, closes SQLite, and exits. It does not immediately cancel healthy in-flight mutations. | Dropped operations or a process that cannot terminate. | Signal tests during idle, active requests, and drain timeout. |
| **GP-019** | Configuration validation and embedded migrations finish before HTTP serving. Database or migration failure is fatal. Foreign keys are enabled. A missing database may be created; an existing unreadable, corrupt, or incompatible database is never renamed, overwritten, deleted, or silently replaced. | Partial startup and silent durable-state loss. | First-run and failure-path startup tests. |
| **GP-020** | `/healthz` measures process liveness. `/readyz` measures SQLite availability and GoPanel's ability to serve. Managed-system reachability is separate observed state and does not determine readiness. | Deployment health being coupled to independent dependencies. | Readiness tests with every integration unavailable. |
| **GP-021** | Every backend or managed-system failure receives an error reference. Request-bound failures share it across the UI, Error Panel, and structured log. Background failures share it across the affected status when surfaced, Error Panel, and structured log. Fatal startup failures emit it in the structured log before exit because no browser surface exists yet. The normal UI uses `422` validation, `403` authorization, `404` missing resource, `502`/`503` dependency failure, and `500` unexpected failure. | Silent failures, internal-detail disclosure, and inconsistent recovery behavior. | Error-reference, mapping, redaction, lifecycle-log, and HTTP-boundary tests. |

### 3.5 Code and interface shape

| ID | Must remain true | Prevents | Minimum proof |
| --- | --- | --- | --- |
| **GP-022** | Dependency direction is `handler -> service -> store or typed client`; views receive prepared presentation models only. Handlers may compose services; services do not call one another into an orchestration graph. `cmd/gopanel/main.go` wires dependencies; `internal/app` owns lifecycle. | Hidden authority, cross-service coupling, and business logic in views. | Package review or dependency tests plus handler tests. |
| **GP-023** | Real links and forms work without JavaScript. HTMX enhances the same URL and prepared data path; fragments return HTML. A small external project-owned handler explicitly swaps `4xx`/`5xx` error fragments and makes HTMX transport failures visible. Primary navigation remains refreshable, bookmarkable, and compatible with Back. | Parallel business paths, silent HTMX failures, and JavaScript-only operation. | JavaScript-disabled, HTTP-error swap, transport-failure, and full-page/fragment consistency tests. |
| **GP-024** | Every major resource view implements Loading, Empty, Loaded, and Error states. Important failures remain visible. Mobile and desktop ship in the same slice; no critical action is hover-only; destructive confirmation names the target. | Unsafe or unusable behavior during delay, failure, or mobile use. | View-state and representative browser tests. |
| **GP-025** | New dependencies, interfaces, concurrency, shared abstractions, frameworks, generic engines, and extension points require a demonstrated current need and must keep authority and failure paths visible. | Speculative complexity and widened attack surface. | Narrow-scope review, dependency rationale, and behavioral tests. |

### 3.6 Operator clarity and error handling

| ID | Must remain true | Prevents | Minimum proof |
| --- | --- | --- | --- |
| **GP-026** | A user-initiated action never fails silently. Success, rejection, denial, timeout, and backend failure each produce a visible result associated with the affected form or resource. Important failures remain visible until dismissed, resolved, or replaced by a newer result. Background dependency failures update the affected status or resource state without creating repeated pop-up noise. | Operators believing an action succeeded when nothing happened. | Full-page, HTMX, JavaScript-disabled, timeout, and background-failure tests. |
| **GP-027** | An action that requires authentication, an administrator role, confirmation, or another explicit authorization step states that requirement before execution when practical. A blocked or denied attempt shows a clean dialog, inline alert, resource alert, or page alert explaining what was blocked, why it cannot proceed, and the safe next step. A missing session prompts sign-in; insufficient role states that administrator access is required. | Invisible permission gates, confusing no-ops, and repeated unauthorized attempts. | Anonymous, expired-session, viewer, admin, confirmation, and direct-request tests. |
| **GP-028** | Every user-facing field and control has a visible plain-language label or accessible name. When purpose or format is not obvious, concise help states what the value controls and gives the expected form, unit, range, or allowed choice. Placeholder text and icons do not replace labels. Help text stays local and brief; the UI does not become a duplicate manual. | Guesswork, inaccessible controls, and excessive microcopy. | Form/component review and accessibility tests. |
| **GP-029** | Validation errors appear beside the relevant field, identify the problem, and state what acceptable input looks like. Safe entered values remain available for correction. Validation does not echo passwords, tokens, secret values, or unsafe raw input. | Repeated invalid submissions and accidental sensitive-data disclosure. | Field-level validation, preserved-input, and secret-redaction tests. |
| **GP-030** | Request-bound backend and managed-system errors show a plain-language explanation and error reference. Administrator-facing failures include `See Error Log`, which opens an administrator-only Error Panel entry. Non-administrators receive the reference and a safe instruction to contact an administrator. The panel shows timestamp, associated user or `system`, action or route, target when known, component or integration, HTTP status, audit/correlation ID when present, and safely rendered technical detail. It never displays credentials, session tokens, secret values, authorization headers, raw request bodies, or untrusted container-log content. | Untraceable failures and unsafe exposure of raw diagnostics. | Role enforcement, field completeness, correlation, redaction, and error-panel rendering tests. |

### 3.7 Test and evidence integrity

| ID | Must remain true | Prevents | Minimum proof |
| --- | --- | --- | --- |
| **GP-031** | Evidence for a critical invariant identifies the exact commit or baseline-plus-complete-diff digest, executes required tests uncached, demonstrates new or materially changed critical tests against known-wrong behavior, and reports checks only as `PASS`, `FAIL`, `NOT RUN`, or `INCONCLUSIVE`. Local evidence is distinct from independent CI evidence. No phase is complete until every required check has inspectable evidence and the exact commit passes CI without skipped, bypassed, or weakened required checks. | Cached, self-fulfilling, stale, skipped, or source-ambiguous checks being presented as proof. | Source manifest, uncached logs, restored negative-control records, literal browser evidence where required, and exact-commit CI logs. |

---

## 4. v1 Boundary Invariants

These rules define the v1 surface. A later version may change them only through the process in §6.

| ID | v1 boundary |
| --- | --- |
| **V1-001** | GoPanel is one process using one SQLite database. There is no Postgres layer, Redis, message bus, distributed worker system, or clustering protocol. |
| **V1-002** | Identity is local email/password with only `admin` and `viewer`. There is no public signup, default credential, OIDC, or general RBAC engine. The first admin is created locally with `gopanel user create-admin`; its password is prompted for, not supplied through a public page or environment variable. |
| **V1-003** | Login protection uses a short-recovery per-account limiter and a small process-wide limiter with bounded burst and automatic recovery. It does not rely on untrusted forwarded client IPs or create an indefinite global lockout. |
| **V1-004** | Container log retrieval is administrator-only and bounded to the planned last 100 lines. v1 has no live or unbounded log stream. |
| **V1-005** | Kubernetes accepts only configured allowed contexts and lists only Pods and Deployments. It has no mutation, generic resource browser, YAML inspector, operator, or event framework. |
| **V1-006** | Vault is optional. If it ships, a named GoPanel operation must consume the resolved secret. There is no generic secret browser or unused secret-resolution abstraction. |
| **V1-007** | Vendored browser code has an exact version and checksum. Production requires no Node.js runtime. GoPanel emits a self-only Content Security Policy with framing disabled, `X-Content-Type-Options: nosniff`, and `Referrer-Policy: no-referrer`. Inline JavaScript is avoided. |
| **V1-008** | The SQLite database and GoPanel-owned credential or token files use mode `0600`. `config.yaml` contains no reusable secrets and is not writable by untrusted users. |
| **V1-009** | v1 has no generic infrastructure passthrough, arbitrary HTTP fetch, plugin architecture, backup subsystem, Kubernetes mutation, or automatic write-retry framework. |
| **V1-010** | The Error Panel uses a bounded process-local diagnostic buffer for the current GoPanel process. It clearly states that entries reset on restart. Structured backend logs remain the operational record across restarts. Durable in-app error retention requires a later explicit storage, retention, and access-control design. |

---

## 5. Enforcement

Every change identifies the invariants it touches and adds or updates behavioral proof for the protected condition. A phase is not complete while an applicable invariant lacks evidence. Phase 9 verifies earlier controls; it does not make an unsafe earlier slice complete after the fact.

Before a v1 release, each invariant must point to:

- an automated test;
- an inspection or operational drill; or
- `not applicable`, with a reason tied to an unshipped capability.

Documentation alone is not proof.

---

## 6. Changing an Invariant

An invariant changes only when implementation evidence or a demonstrated product requirement shows that it is wrong or incomplete.

The same change must:

1. explain the concrete requirement;
2. identify what the old rule protected;
3. describe the new failure modes and controls;
4. update `ARCHITECTURE.md` and this file together;
5. update affected tests and operational or user documentation; and
6. record any compatibility, migration, or rollback consequence.

Changing `ROADMAP.md` alone does not change an invariant.

---

## 7. Governance Decisions

### D-001 — Audit begins with server registration

Server and connection configuration changes are privileged control operations. The `audit_log` migration and small `RecordAttempt` / `RecordResult` primitive therefore arrive in Roadmap Phase 3 with server registration.

Phase 5 remains the first managed-system mutation and proves partial-completion behavior across SQLite and Docker. It reuses the Phase 3 audit primitive and does not introduce a generic mutation framework.

### D-002 — Root documentation entry point

The repository root contains `README.md` and `AGENTS.md`. Governance files live under `/docs`. There is no separate `docs/README.md`; the root README links directly to each governance document.

### D-003 — Error Panel disclosure boundary

The Error Panel is a browser surface even though it is administrator-only. It preserves diagnostic usefulness without displaying unfiltered backend values.

The owning integration maps errors into safe technical diagnostics. The diagnostic recorder applies defense-in-depth redaction of credentials, session material, secret values, authorization headers, raw request bodies, unsafe paths where necessary, and untrusted workload output before writing the Error Panel buffer or structured log.

v1 uses the bounded process-local buffer in V1-010. Cross-restart in-app history requires a later architecture change with explicit retention, deletion, size, redaction, access-control, and backup rules.

### D-004 — Go 1.27 baseline

The user selected Go `1.27.0` as the minimum supported and CI toolchain for this greenfield project. GoPanel retains Templ `v0.3.1020`, Chi `v5.3.2`, and modernc SQLite `v1.57.0` and does not claim Go 1.22 or Go 1.25 compatibility.

### D-005 — Phase 2 CSRF baseline

The owner selected the process-local HMAC-SHA-256 contract in GP-010. Authenticated tokens bind to the raw session credential. Login tokens bind to a separately signed 15-minute anonymous context that survives ordinary credential failure and multiple tabs but is destroyed on successful login. Restart rotates the signing key and invalidates forms, not valid database sessions. `http.CrossOriginProtection`, `SameSite`, and browser expiry are defense in depth and never replace server token and signed-expiry validation.

---

## 8. Source Map

| Invariant group | Primary source |
| --- | --- |
| GP-001–GP-006 | `ARCHITECTURE.md` §§1, 4, 6, 9.5–9.6; `CODING_STYLE.md` §§6, 9, 14, 34 |
| GP-007–GP-012 | `ARCHITECTURE.md` §9; `ROADMAP.md` Phases 2, 4, and 7; `CODING_STYLE.md` §§12–14 and 34 |
| GP-013–GP-015 | `ARCHITECTURE.md` §§8.1, 9.10, 13, and 16; `ROADMAP.md` Phases 3 and 5–6; `CODING_STYLE.md` §§11–12 and 36 |
| GP-016–GP-021 | `ARCHITECTURE.md` §§6.3, 8, 9.9, and 11; `ROADMAP.md` Phases 1, 2A, and 4; `CODING_STYLE.md` §§10, 15–16, and 35–38 |
| GP-022–GP-025 | `ARCHITECTURE.md` §§2, 4, 8.5, 10, and 16; `CODING_STYLE.md` §§17–18, 22–26, 30–32, and 42–43 |
| GP-026–GP-030 | `ARCHITECTURE.md` §§8.6, 9.3, 9.9, and 10.6–10.10; `ROADMAP.md` Phase 2A; `DOCUMENTATION.md` §§2–4 |
| GP-031 | `ARCHITECTURE.md` §13.1; `CODING_STYLE.md` §25.1; `ROADMAP.md` Phase 0.75; `DOCUMENTATION.md` §12; `.github/workflows/build.yaml`; `docs/maintainers/development.md` |
| V1-001–V1-010 | `ARCHITECTURE.md` §§3, 6.3, 9.7, 12, and 15–16; `ROADMAP.md` Phases 0–9 |
