# GoPanel - Architecture Document

## 1. Purpose

GoPanel is a privileged control plane that lets a developer or operator perform common infrastructure operations without remembering CLI commands.

**GoPanel is a control interface, not an infrastructure state store. Managed systems own their state. GoPanel owns authentication, connection configuration, audit history, bounded process-local diagnostics, and presentation.**

It manages external systems. It does not own them.

```text
Internet
   |
Reverse Proxy (TLS)
   |
GoPanel
   |
   +-- Docker
   +-- Caddy Admin API
   +-- Vault
   +-- Kubernetes
```

The browser never talks directly to Docker, Caddy, Vault, or Kubernetes. Only GoPanel does.

Target user: a developer or operator. Not a non-technical user. The UI makes common infrastructure operations convenient, but it does not hide that containers, proxies, secrets, and clusters are technical concepts.

## 2. Design Principles

1. **Simple over clever:** Prefer boring, readable code. One file should have one obvious reason to change.
2. **Maintainability:** Code is for the next person. Small files, explicit errors, no magic. No numerical file limits.
3. **Single binary, single node for v1:** One GoPanel instance can manage many external systems. GoPanel itself does not need to be clustered for v1.
4. **External systems are authoritative:** Docker owns container state. Caddy owns route state. Kubernetes owns pod and deployment state. Vault owns secret values. GoPanel does not duplicate that state as durable truth.
5. **User friendliness:** Clean, aesthetically pleasing, and usable on mobile and desktop, but never at the cost of safety. Actions never fail silently; blocked, denied, invalid, and failed actions explain what happened and what the operator can safely do next.
6. **Server-rendered, progressively enhanced:** Real HTML works first. HTMX improves it. JavaScript failure does not break basic operation.
7. **Explicit privileged operations:** GoPanel exposes a small set of named operations, never generic passthrough access to infrastructure administration APIs.
8. **Secure production defaults, not enterprise sprawl:** v1 uses local authentication, two roles, bounded process-local controls, and simple operational contracts. Enterprise identity, clustering, and policy frameworks are deferred until demonstrated need.

## 3. Tech Stack - v1

- **Backend:** Go 1.27.0
- **Router:** chi
- **UI rendering:** Templ typed HTML components
- **Progressive enhancement:** HTMX, vendored and pinned
- **Styling:** Tailwind CSS, compiled ahead of production runtime
- **JavaScript:** Native browser features first (`<dialog>`, `<details>`, CSS). Very small plain JavaScript where needed. Alpine.js is an escape hatch, not a default dependency.
- **Database:** SQLite only for v1. No Postgres portability layer.
- **Authentication:** Local username/email + password with opaque server-side sessions
- **Infrastructure clients:** Docker SDK, Caddy Admin API client, Vault SDK, Kubernetes `client-go`

Why SQLite only: SQLite and Postgres are not interchangeable. SQL syntax, migrations, locking, transactions, timestamps, and JSON behavior diverge. Supporting both from day one creates permanent testing burden for no current benefit.

Add Postgres only when there is a demonstrated requirement SQLite cannot satisfy.

Production runtime is one GoPanel process, one SQLite file, and embedded or bundled static assets. Node.js is not required in production.

Go `1.27.0` is both the minimum supported and CI toolchain. The user selected the current supported Go release as the explicit baseline for this greenfield project. GoPanel does not claim compatibility with earlier Go releases.

## 4. High-Level Architecture

```text
Browser
  |
  v
TLS Reverse Proxy
  |
  v
GoPanel (one process, one SQLite file)
  |
  +-- Authentication + Sessions
  +-- Authorization Middleware
  +-- HTTP Handlers
  +-- Services
  +-- Store (SQLite)
  +-- In-memory CachedStatus
  +-- Bounded in-memory Error Buffer
  |
  +-- Docker SDK
  +-- Caddy Admin API
  +-- Vault SDK
  +-- Kubernetes client
```

Dependency rules:

```text
handler -> service -> external SDK / store
view -> presentation models only
service A -X-> service B
browser -X-> infrastructure APIs
```

Handlers may compose results from multiple services. Services do not become an orchestration graph by calling one another.

GoPanel never exposes a generic infrastructure request primitive such as:

```go
DockerRequest(method, path, body)
FetchURL(address)
```

It exposes explicit operations such as:

```go
ListContainers(ctx, serverID)
StartContainer(ctx, containerID)
StopContainer(ctx, containerID)
ListRoutes(ctx)
CreateRoute(ctx, input)
ListPods(ctx, namespace)
```

## 5. Project Layout

There is no three-file limit. A module has as many files as needed to stay readable.

```text
/cmd/gopanel/main.go             // wiring only
/internal/
  /app/
    app.go                       // lifecycle: Run, shutdown, poller ownership
  /config/
    config.go                    // application configuration + validation
  /auth/
    password.go
    session.go
    rate_limit.go
    middleware.go
    handler.go
  /diagnostic/
    record.go                    // safe diagnostic record + opaque error reference
    buffer.go                    // bounded process-local Error Panel entries
    handler.go                   // admin-only list and detail views
  /server/
    model.go                     // connection configuration, not live state
    store.go
    health.go                    // one finite CheckStatus operation
    handler.go
  /container/
    model.go
    list.go
    actions.go                   // explicit Start / Stop operations
    logs.go                      // bounded log retrieval in v1
    handler.go
  /proxy/
    model.go
    routes.go
    handler.go
  /secret/
    service.go                   // uses secret server-side, never renders value
    handler.go
  /cluster/
    pods.go
    deployments.go
    handler.go
  /store/
    sqlite.go
    migrations/
  /view/
    /layout/
      base.templ
    /components/
      button.templ
      badge.templ
      card.templ
      alert.templ
      form_field.templ
      permission_alert.templ
      error_reference.templ
    /pages/
      /auth/
        login.templ
      /server/
        server_list.templ
        server_detail.templ
        server_card.templ
      /container/
        container_list.templ
        container_row.templ
      /diagnostic/
        error_list.templ
        error_detail.templ
/web/static/
  htmx-1.9.12.min.js             // example pinned version; record exact hash
  application.js                 // small project-owned HTMX transport-error fallback
  output.css                     // compiled Tailwind output
config.yaml                      // application behavior only, no reusable secrets
```

`cmd/gopanel/main.go` wires dependencies only:

```go
func main() {
    app := buildApplication()
    app.Run(ctx)
}
```

Application lifecycle belongs in `internal/app/app.go`.

## 6. Ownership and Data Model

### 6.1 Do not persist live infrastructure state

Problem we avoid:

```text
GoPanel DB: container = running
Docker:     container = stopped
```

Docker is truth for containers. Caddy is truth for routes. Kubernetes is truth for pods and deployments. Server reachability and dependency health are observed state, not durable configuration.

### 6.2 v1 durable tables

```sql
users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,              -- admin, viewer
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)

sessions (
    token_hash TEXT PRIMARY KEY,     -- SHA-256(raw browser session token)
    user_id TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
)

servers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    address TEXT NOT NULL,           -- validated by owning module
    connection_type TEXT NOT NULL,   -- known type only
    credential_reference TEXT,       -- opaque module-owned reference, never raw secret
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)

audit_log (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    action TEXT NOT NULL,            -- create_server, stop, start, create_route, use_secret_ref
    target_type TEXT NOT NULL,       -- server, container, proxy_route, secret_ref
    target_id TEXT NOT NULL,
    result TEXT NOT NULL,            -- attempted, success, failed
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
)
```

Before Phase 4, server registration keeps `credential_reference` null. Only the integration that understands a reference may validate and populate it.

Enable SQLite foreign-key enforcement with:

```sql
PRAGMA foreign_keys = ON;
```

Versioned migrations are embedded in the GoPanel binary. Migrations run before the HTTP server starts. Migration or database startup failure is fatal.

The migration mechanism owns a `schema_migrations` metadata table. Phase 1 contains no application capability tables. Migration versions are contiguous, forward-only, and applied transactionally; a failed migration is neither skipped nor recorded as successful. A database with migration metadata newer than or incompatible with the running binary is rejected before listening.

WAL mode is an implementation choice, not an architectural dependency.

SQLite and application configuration are the only persistent GoPanel state requiring backup. GoPanel does not implement a backup subsystem in v1.

### 6.3 Temporary runtime state is memory-only

```go
type CachedStatus struct {
    ServerID             string
    CheckedAt            time.Time
    ServerReachable      bool
    DockerConnected      bool
    CaddyConnected       bool
    VaultConnected       bool
    KubernetesConnected  bool
}
```

`CachedStatus` is process memory only.

```text
GoPanel restart
    -> cached observations disappear
    -> poller checks dependencies again
    -> cache repopulates
```

A cached observation is never presented as durable truth. The UI shows when it was checked.

The Error Panel also uses process memory rather than a new durable table.

```go
type DiagnosticRecord struct {
    ID                  string
    CreatedAt           time.Time
    UserID              string        // empty for system-owned work
    Action              string
    Target              string
    Component           string
    HTTPStatus          int
    AuditCorrelationID  string
    PublicMessage       string
    TechnicalDetail     string        // safe rendered detail, never an unfiltered error
}
```

v1 keeps only the most recent 200 diagnostic records. Adding a new record after the limit is reached evicts the oldest record. The Error Panel states that entries cover only the current GoPanel process and disappear on restart.

Structured backend logs use the same diagnostic ID and remain the operator-controlled record across restarts. GoPanel does not add an `error_log` table in v1. Durable in-app error history requires a later design covering size, retention, deletion, redaction, access control, and backup consequences.

## 7. Naming Conventions

Use full words and human-readable names.

```go
// Good
func ListServers(ctx context.Context) ([]Server, error)
func GetServer(ctx context.Context, id string) (Server, error)
func StartContainer(ctx context.Context, id string) error

type Server struct {
    ID                 string
    Name               string
    Address            string
    ConnectionType     string
    CredentialReference string
    CreatedAt          time.Time
}

// Bad
func FetchSrvLst() ([]SrvEnt, error)
type SrvCfgMgr struct{}
```

Rules:

- Use full words. `server`, not `srv`.
- IDs are `ID`, not `Id` or `Identifier`.
- Booleans read as questions: `isActive`, `canEdit`, `hasError`.
- Constants are descriptive: `DefaultTimeout`, `MaxServerNameLength`.
- Templ components are nouns: `Button`, `ServerCard`, `ContainerRow`.
- Files use readable names: `server_list.templ`, not `srvLstV2.templ`.

## 8. Request Handling and Reliability

### 8.1 External calls have bounded lifetimes

Every infrastructure SDK call inherits a context and has a bounded timeout unless the operation is explicitly designed as a bounded stream.

```go
ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
defer cancel()

containers, err := dockerClient.List(ctx)
```

No infrastructure call may ignore cancellation.

Reads may be retried when the operation is safe and bounded. Writes must not be automatically retried unless the operation is explicitly known to be idempotent and safe to repeat.

Problem avoided:

```text
Create proxy
    -> timeout
    -> automatic retry
    -> duplicate or uncertain change
```

### 8.2 Health checks are finite operations

A health check performs one check and returns.

```go
func CheckStatus(ctx context.Context, server Server) (Status, error)
```

No service owns an endless timer or per-server goroutine.

The application lifecycle owns one deliberately simple poller:

```text
every 30 seconds:
    for each configured connection:
        check with timeout
        update in-memory CachedStatus{State, CheckedAt}
```

Meaningful health checks report the capability GoPanel actually depends on:

- Server reachable, where applicable
- Docker connected
- Caddy connected
- Vault connected
- Kubernetes connected

ICMP ping alone is insufficient. A host can answer ICMP while Docker is unavailable, or block ICMP while the managed service works correctly.

`/healthz` reports process liveness only. `/readyz` reports whether configuration, SQLite, and migrations are ready and becomes unavailable during shutdown. Readiness never depends on Docker, Caddy, Vault, Kubernetes, or another managed system.

### 8.3 Bounded log retrieval in v1

`ViewLogs` is not treated like an ordinary short SDK request if live streaming is eventually added.

For v1, prefer bounded retrieval such as:

```text
last 100 lines
```

If live streaming is added later, it must:

- use request context for lifetime;
- cancel when the client disconnects;
- use bounded buffering;
- never accumulate unbounded log data in memory.

### 8.4 Graceful shutdown

Background work and HTTP request contexts are separate.

Shutdown order:

```text
SIGTERM / SIGINT
    -> mark application shutting down
    -> stop status poller and other background work from scheduling new checks
    -> stop accepting new HTTP requests
    -> allow active HTTP requests a bounded time to finish
    -> cancel remaining background work when shutdown deadline expires
    -> close SQLite
    -> exit
```

Do not globally cancel healthy in-flight mutations the instant the process receives a shutdown signal.

Use Go's standard `http.Server.Shutdown(ctx)` for bounded HTTP draining. No lifecycle framework is required.

Lifecycle events use stable structured names for startup, database opening, migration completion or failure, listening, shutdown initiation, HTTP drain outcome, database closure, and shutdown completion. Fatal startup failures receive an opaque error reference in the structured terminal log; no browser or Error Panel surface is claimed before serving begins.

### 8.5 Full page and HTMX fragment use the same URL

One resource URL supports both normal navigation and progressive enhancement.

```go
if IsHTMXRequest(r) {
    return ServerDetail(...)
}
return ServerDetailPage(...)
```

Do not create parallel `/fragments/`, `/partials/`, or client-only endpoint trees for the same resource.

### 8.6 HTTP and HTMX error convention

Use consistent status semantics:

```text
422  validation error      -> field-specific correction in the returned form
403  authorization failure -> visible explanation of the required role or authorization
404  resource not found    -> stable not-found page or fragment
502/503 dependency failure -> persistent resource error + recovery action + error reference
500  unexpected server error -> persistent page alert + error reference
```

A user-initiated request always returns a visible outcome for both full-page and HTMX requests. A handler should not invent its own error transport convention.

Backend and managed-system failures receive one opaque error reference. Administrators see `See Error Log`, linking to the matching Error Panel entry. Non-administrators see the reference and a safe instruction to contact an administrator.

HTMX 1.9.12 does not swap `4xx` or `5xx` responses by default. The base layout loads the small external project-owned `application.js`, which handles `htmx:beforeSwap` and sets `event.detail.shouldSwap = true` only for GoPanel's `4xx` and `5xx` HTML responses. The response keeps its real HTTP status and declared target.

The same script handles `htmx:sendError` and `htmx:timeout` by placing a persistent generic transport error at the intended target. It does not synthesize an error reference because no server response exists. It builds fallback content with DOM APIs and fixed text, not response data. Inline event handlers are not used. Full-page and JavaScript-disabled requests retain normal browser behavior.

## 9. Security Model

GoPanel is a privileged host-management/control service. Compromise may permit container actions, proxy changes, Vault access, or Kubernetes operations according to the credentials available to the process.

A GoPanel instance with Docker daemon access must be treated as a privileged host-management service. The same principle applies to powerful Caddy, Kubernetes, and Vault credentials.

### 9.1 Local authentication for v1

OIDC is deferred. v1 uses local authentication.

Passwords:

- are never stored directly;
- are hashed with Argon2id;
- return the same login failure message for an unknown user and a wrong password;
- are protected by a bounded per-account limiter with short recovery;
- are also protected by a small process-wide token-bucket style limiter with bounded burst and automatic recovery;
- cause all active sessions for the user to be invalidated after password change.

GoPanel does not use client-IP login throttling in v1. It sits behind a reverse proxy and deliberately does not trust `X-Forwarded-*` headers, so the observed network peer is normally the proxy rather than the real client. Trusted client-IP forwarding may be added later only through explicit trusted-proxy configuration.

The v1 limiters may be process-local because GoPanel is single-node. They must remain bounded and must not create a long global login lockout.

### 9.2 Opaque sessions

The browser receives a cryptographically random session token of at least 256 bits.

```text
Browser cookie:
    session_id = random 256-bit token

SQLite:
    token_hash = SHA-256(session_id)
    user_id
    expires_at
    created_at

Request:
    cookie -> SHA-256 -> token_hash lookup
```

The raw session credential is never stored in SQLite.

Session cookies are:

- `HttpOnly`;
- `Secure`;
- `SameSite=Lax` or stricter;
- bounded by expiration;
- rotated on successful login;
- invalidated server-side on logout.

Expired sessions are periodically removed:

```sql
DELETE FROM sessions WHERE expires_at < ?;
```

Password changes invalidate all sessions for that user.

### 9.3 Authorization is enforced by the server

Hiding an action from a viewer is UX only. Every protected handler verifies authorization independently.

```text
RequireLogin
RequireAdmin
```

Example:

```text
POST /containers/{id}/stop
    -> RequireLogin
    -> RequireAdmin
```

v1 roles are only:

```text
admin
viewer
```

No general RBAC engine is required.

Authorization failure is visible, not a silent no-op:

- a missing or expired session prompts sign-in and may preserve only a validated same-origin relative return path;
- a viewer attempting an administrator action receives `403` with `Administrator access is required` and a safe next step;
- an action requiring confirmation names the action and target before submission; and
- the denial is recorded as a security event, not as an accepted-operation audit row.

### 9.4 CSRF protection covers every mutation

Use a deliberately simple HTTP contract:

```text
GET  = read only
POST = state change
```

Examples:

```text
POST /containers/{id}/start
POST /containers/{id}/stop
POST /routes/create
POST /servers/{id}/delete
```

Every state-changing POST validates a CSRF token. PUT/PATCH/DELETE are not required for the HTML interface.

Phase 2 uses one process-local 32-byte HMAC-SHA-256 signing key generated with `crypto/rand` during process composition. Randomness failure is fatal. The key is never persisted, logged, rendered, audited, or diagnosed; restart rotates it and invalidates outstanding forms without deleting otherwise-valid database sessions.

Tokens use strict versioned base64url fields and MAC length-framed values containing the protocol version, an exact domain, the bound raw browser credential, and a fresh 32-byte nonce. Authenticated forms use domain `gopanel-csrf-v1-authenticated` and bind to the raw session cookie credential—not user identity or the stored session hash. Tokens remain valid only while that session and process remain valid. Logout, rotation, expiry, revocation, password change, or restart invalidates them. No CSRF value is stored in SQLite.

Login uses a separately signed anonymous context cookie with domain `gopanel-login-context-v1`, 32 random context bytes, and a server-authenticated 15-minute absolute expiry. Its form token uses domain `gopanel-csrf-v1-login`. The context is host-only, `HttpOnly`, `Path=/`, `SameSite=Lax`, Secure in production, and non-Secure only for loopback `--dev`. Failed credentials preserve it for retry and multiple tabs. Successful login destroys it and creates a fresh independent session; it is never converted into a session.

POST processing bounds the body, applies `http.CrossOriginProtection` as defense in depth, then validates the expected hidden form token even when origin metadata is missing or allowed. Missing, malformed, expired, forged, wrong-version, wrong-size, bad-MAC, other-context, cross-session, or pre-restart tokens fail before protected behavior. Tokens are never accepted from query data, cookies, referrers, or generic headers. Full-page and HTMX failures return visible `403` HTML: `This form has expired or is invalid. Reload the page and try again.` Origin approval and `SameSite` never replace token validation; trusted origins come only from validated `public_url`, never `Host` or `X-Forwarded-*`.

### 9.5 Trust boundary and SSRF prevention

The browser never talks directly to infrastructure APIs.

Users cannot make arbitrary HTTP requests through GoPanel. Every outbound connection belongs to a known connection type and is validated by the owning module.

Forbidden:

```go
FetchURL(address)
DockerRequest(method, path, body)
os.Open(databaseCredentialReference)
```

Allowed examples:

```text
Docker:
    connection_type = unix
    socket path comes from application configuration

Caddy:
    credential_reference = caddy_main
    module maps caddy_main to configured Caddy settings

Kubernetes:
    credential_reference = prod-context
    module accepts only configured allowed contexts

Vault:
    endpoint and token-file location come from application configuration
    database never supplies an arbitrary file path
```

A credential reference is interpreted only by its owning module. Users cannot supply arbitrary filesystem paths, environment-variable names, sockets, or URLs unless that module explicitly permits and validates the value.

Do not implement one universal network allowlist that blindly rejects private addresses. GoPanel legitimately manages private infrastructure. The protection is typed, explicit outbound operations rather than arbitrary fetch capability.

### 9.6 Secrets and credentials

GoPanel stores connection metadata and opaque references, never reusable credential values.

Secret values are never rendered into HTML.

```text
User chooses secret reference
    -> GoPanel resolves it server-side
    -> GoPanel uses it for the intended operation
    -> secret value is discarded
```

There is no `GetSecret(path)` result passed to Templ.

Do not place secret values in:

- browser HTML;
- browser memory intentionally;
- audit records;
- structured logs;
- error messages.

When secret use must be audited, record the reference and action, not the value.

### 9.7 Browser security headers

GoPanel emits a small, explicit browser security policy.

Example CSP:

```text
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'
```

Additional headers:

```text
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
```

`frame-ancestors` is a Content Security Policy directive, not a separate HTTP header.

Avoid inline JavaScript so the CSP can remain simple.

### 9.8 Reverse proxy trust

GoPanel may sit behind Caddy or Nginx, but it does not blindly trust inbound proxy headers from arbitrary clients.

Do not derive security-sensitive origins or callback URLs directly from:

```text
Host
X-Forwarded-For
X-Forwarded-Proto
X-Forwarded-Host
```

If GoPanel later needs an absolute public origin, use an explicit configured value such as:

```yaml
public_url: "https://panel.example.com"
```

OIDC is not part of v1. If added later, its callback uses configured `public_url`, not an untrusted request header.

### 9.9 Error handling

Every backend or managed-system failure receives one opaque diagnostic ID.

- Request-bound failures share the ID across the user-facing response, the Error Panel, and the structured backend log.
- Background failures share the ID across the affected status or resource when surfaced, the Error Panel, and the structured backend log.
- Fatal startup failures emit the ID in the structured backend log before exit. No browser or Error Panel route exists when startup fails before serving begins.

```text
Structured log:
    error_id=err_... user_id=... component=docker action=list_containers
    detail="docker request timed out"

Normal UI:
    Docker did not respond. Try again.
    Error reference: err_...
    [See Error Log]      # administrator only
```

The normal UI uses plain language and a safe recovery action. Validation errors appear beside the relevant field and state what acceptable input looks like. Blocked and denied actions explain the required authorization without exposing sensitive policy or infrastructure detail.

The Error Panel is server-authorized with `RequireAdmin`. It exposes:

- timestamp;
- associated user, or `system` for background work;
- action or route;
- target when known;
- component or integration;
- HTTP status;
- audit or correlation ID when present; and
- safe technical detail.

The Error Panel never displays an unfiltered Go error. The owning integration first maps its error into a safe diagnostic; the diagnostic recorder applies defense-in-depth redaction before writing the buffer or structured log.

Never record or display passwords, credential values, session tokens, authorization headers, secret values, raw request bodies, or untrusted container-log content. Filesystem paths and network endpoints are included only when they are safe and useful to the administrator.

Error Panel access is itself security-logged. The bounded buffer is not audit history and is not a substitute for the mutation audit log.

### 9.10 Audit semantics

A privileged control operation is an authenticated operator action that changes managed-system state, changes GoPanel's durable server or connection configuration, or resolves or uses a reusable secret.

Every accepted privileged control operation must first create a durable audit row with `result = attempted`. If that insert fails, the operation does not run.

```text
accepted privileged control operation
    ↓
insert audit row: attempted
    ↓
if insert fails -> do not execute mutation
    ↓
perform control operation
    ↓
update audit row: success | failed
```

If the final audit update fails, the durable row remains `attempted`. This means GoPanel accepted the operation but could not durably establish its final outcome. A high-severity structured log records the failure.

The audit row ID is also used as the mutation correlation ID in structured logs so an `attempted` row can be matched to downstream operation and audit-update events without adding a tracing subsystem.

Examples:

```text
user:        admin@example.com
action:      stop
target_type: container
target_id:   nginx
result:      success
```

```text
user:        admin@example.com
action:      stop
target_type: container
target_id:   nginx
result:      failed
```

```text
user:        admin@example.com
action:      stop
target_type: container
target_id:   nginx
result:      attempted
```

Authentication and session housekeeping, health polling, migrations, pure reads, and the local first-admin bootstrap are not privileged control operations. Authorization rejections remain security-log events rather than accepted-operation audit rows.

## 10. UI/UX System

### 10.1 Mobile-first is part of every vertical slice

Design at approximately 375px first, then expand to desktop. Mobile UX is not a final cleanup phase.

Every feature is incomplete until its core interaction works on both mobile and desktop.

No critical action is available only on hover.

Use large tap targets for primary controls, approximately 44px minimum height where practical.

### 10.2 Progressive enhancement - real HTML first

HTMX enhances working HTML; it is not required for basic navigation or form submission.

Bad:

```templ
<button hx-get="/servers/123">View Details</button>
```

Good:

```templ
<a
    href="/servers/123"
    hx-get="/servers/123"
    hx-target="#main-content"
    hx-push-url="true"
>
    View Details
</a>
```

Actions are real forms:

```templ
<form
    method="post"
    action="/containers/123/start"
    hx-post="/containers/123/start"
    hx-target="#container-123"
>
    <input type="hidden" name="csrf_token" value={ csrfToken } />
    <button type="submit">Start</button>
</form>
```

HTML works first. HTMX improves it. JavaScript failure does not break the basic operation.

### 10.3 URL is UI state

Primary navigation uses real URLs and `hx-push-url="true"` when enhanced by HTMX.

A server detail page is:

```text
/servers/123
```

not `/servers` with hidden client-side state.

Refresh, Back, bookmarking, and sharing therefore behave like normal web navigation.

### 10.4 Components

Keep low-level visual primitives generic:

- `Button`
- `Badge`
- `Card`
- `Alert`
- `FormField`

Use resource-specific presentation components where behavior or mobile layout differs:

- `ServerTable`
- `ServerCard`
- `ContainerList`
- `ContainerRow`
- `ProxyRouteList`

Do not build a universal `Table(headers, rows)` abstraction. Resource-specific UI is more code but less awkward abstraction.

### 10.5 Deliberate mobile representation

Do not mechanically transform desktop tables into key/value cards.

Desktop example:

```text
NAME   IMAGE       STATUS   UPTIME   ACTION
nginx  nginx:1.29  Running  3 days   Stop
```

Avoid automatic mobile output like:

```text
Name: nginx
Image: nginx:1.29
Status: Running
Uptime: 3 days
Action: Stop
```

Prefer an intentionally designed representation:

```text
nginx                         [Running]
nginx:1.29
Up 3 days
[View]                          [Stop]
```

Each resource may have its own mobile card or row.

### 10.6 Every resource has four UI states

Every major resource view implements:

- **Loading:** `Checking containers...`
- **Empty:** `No containers are running on this server.`
- **Loaded:** resource content
- **Error:** persistent explanation + recovery action

Example:

```text
Docker is unreachable.
Last successful check: 11:42 AM
[Try Again]
```

Cached health displays freshness:

```text
Docker connected
Checked 18 seconds ago
```

### 10.7 Destructive actions use explicit confirmation

Distinguish:

```text
normal action              -> immediate
 destructive action        -> confirmation
 high-impact destructive   -> confirmation naming the target
```

Do not use generic `Are you sure?` prompts.

Examples:

```text
Restart             -> immediate
Stop                -> Stop container nginx?
Delete proxy route  -> Delete proxy route api.example.com?
```

### 10.8 Feedback

Use feedback according to persistence needs:

```text
Success          -> toast
Validation error -> inline beside field + expected input
Blocked/denied   -> dialog, resource alert, or page alert + reason + next step
Action error     -> persistent beside affected resource + Try Again
System error     -> persistent page alert + error reference
```

Important failures do not disappear in a toast.

Background dependency failures update the affected resource or status indicator. They do not create repeated pop-up noise.

### 10.9 Loading behavior

Every HTMX request that can produce visible delay has an intentional loading state or indicator. Disable repeated submission where duplicate action would be confusing or unsafe.

Do not use loading animations as a substitute for meaningful timeout and error behavior.

### 10.10 Controls, fields, and validation are self-explanatory

Every control has a visible plain-language label or accessible name. Buttons use a concrete verb and target when space permits, such as `Stop container` or `Delete route`.

When a field's purpose or format is not obvious, concise help states:

- what the value controls;
- whether it is required;
- the expected format;
- the unit, range, or allowed choice when applicable; and
- one short example when it materially reduces ambiguity.

Placeholder text and icons do not replace labels. Help text stays beside the relevant control and does not turn the page into a duplicate manual.

Validation errors identify the field, explain the problem, and state what acceptable input looks like. Safe values remain available for correction. Passwords, tokens, secret values, and unsafe raw input are never echoed.

## 11. Configuration

Application configuration describes allowed behavior and connection metadata. Reusable secrets do not live in `config.yaml` as database-style values or pseudo-secret expressions.

Example:

```yaml
port: 8080
database_path: "./data/gopanel.db"
public_url: "https://panel.example.com"

caddy:
  admin_url: "http://127.0.0.1:2019"

kubernetes:
  allowed_contexts:
    - "prod-context"
    - "staging-context"

vault:
  address: "https://vault.internal"
  token_file: "/run/secrets/gopanel-vault-token"

docker:
  socket: "/var/run/docker.sock"
```

The database may store a reference such as `caddy_main` or `prod-context`; the owning module maps that reference to allowed application configuration.

Do not store arbitrary user-provided file paths or environment variable names and later dereference them generically.

There is no `env:SESSION_SECRET` mini-language and no `SESSION_SECRET` requirement for opaque server-side sessions.

Phase 1 uses explicit command-line fields for `listen-address`, `database-path`, and `public-url`; `--dev` supplies loopback development defaults. This keeps configuration typed and dependency-free while configuration-file parsing remains unimplemented. Unknown flags and positional values are rejected.

## 12. Frontend Dependencies

Vendored JavaScript is still executable code. Pin its exact version and record its checksum.

```text
/web/static/htmx-1.9.12.min.js
    source: htmx.org 1.9.12
    sha256: <recorded checksum>
```

No package manager is required in production.

If Alpine.js is later justified, it follows the same rule: exact version, vendored asset, recorded checksum.

## 13. Development Workflow

```bash
templ generate --watch
go run ./cmd/gopanel --dev
tailwindcss -i ./web/static/input.css -o ./web/static/output.css --watch
```

### 13.1 Validation evidence

Critical behavior requires reproducible, inspectable evidence tied to the exact source state. An uncommitted result identifies both the baseline commit and a SHA-256 digest of the complete binary diff; committed and CI evidence identifies the exact commit.

A newly authored test is not self-proving. When a critical test is introduced or materially changed, a targeted negative control demonstrates that it fails for known-wrong behavior before the restored source passes uncached validation. Results are reported only as `PASS`, `FAIL`, `NOT RUN`, or `INCONCLUSIVE`.

Local validation and independent CI validation are distinct evidence. A phase is complete only when all required checks have inspectable evidence, literal browser requirements are satisfied, and the exact implementation commit passes the protected CI workflow without skipped or weakened required checks.

PR rules:

- Real link or form first, then add HTMX attributes.
- Primary HTMX navigation updates the URL.
- Every new resource view implements Loading, Empty, Loaded, and Error states.
- Every user-initiated action produces a visible success, rejection, denial, timeout, or failure result.
- Every backend failure has one error reference correlated across every applicable UI, Error Panel, and structured-log surface.
- The Error Panel is administrator-only and contains only safely rendered technical detail.
- Every field and control has a visible label or accessible name; constrained input explains the expected value.
- Every feature is reviewed on mobile and desktop as part of the same change.
- Use resource-specific components where generic components would hide important behavior.
- No new frontend dependency without explicit justification, version pin, and checksum.
- Authorization is enforced by server middleware/handler path, never only by hidden UI.
- Every accepted privileged control operation creates an `attempted` audit row before execution and resolves it to `success` or `failed` when the final outcome is durably known.
- A privileged control operation without audit behavior is incomplete.
- No secret values in HTML, logs, or audit records.
- Infrastructure SDK calls use contexts and bounded timeouts.
- Writes are not automatically retried unless explicitly safe to repeat.
- Credential references are resolved only by their owning module.

## 14. Vertical Build Sequence

Build features vertically so architecture is exercised by real behavior instead of speculative abstractions.

### 1. Lifecycle + SQLite

- application wiring;
- embedded migrations;
- SQLite startup contract;
- graceful shutdown;
- process lifecycle tests.

### 2. Local authentication

- password hashing;
- opaque hashed sessions;
- CSRF;
- bounded rate limiting;
- security headers;
- mobile login experience.

### 3. Operator feedback + Error Panel

- opaque error references;
- safe integration-owned error mapping and central redaction;
- bounded process-local diagnostic buffer;
- administrator-only Error Panel list and detail views;
- visible sign-in, role, confirmation, validation, timeout, and backend-failure states;
- clear field labels and concise expected-input guidance;
- full-page and HTMX error consistency;
- mobile Error Panel and denial UX.

### 4. Server registration

- validated connection metadata;
- constrained credential references;
- `audit_log` migration and two-phase audit primitive;
- mobile form and list;
- full page + HTMX rendering;
- Loading / Empty / Loaded / Error.

### 5. Docker read-only

- typed Docker connection;
- list containers;
- meaningful health;
- bounded log retrieval;
- desktop and mobile container representations.

### 6. First Docker mutation + audit

- one explicit safe mutation path;
- server-side admin authorization;
- CSRF;
- named confirmation UX;
- no automatic write retry;
- reuse the established audit primitive across SQLite and Docker;
- audit `attempted`, then `success` or `failed` when known;
- persistent action errors;
- mobile action UX.

### 7. Caddy

- explicit route operations only;
- configured admin endpoint;
- no generic API passthrough;
- destructive route confirmation.

### 8. Vault

- secret-reference selection;
- server-side resolution and use;
- secret values never rendered;
- reference use audited when required.

### 9. Kubernetes read-only

- allowed kubeconfig contexts only;
- list pods and deployments;
- no mutation in v1.

Every vertical slice includes, where applicable:

```text
desktop + mobile
full page + HTMX fragment
Loading + Empty + Loaded + Error
authorization
visible authorization and validation outcomes
error reference + safe administrator diagnostic
bounded external calls
```

## 15. What We Will Not Do in v1

- No Postgres support.
- No multi-node GoPanel deployment.
- No persisted live infrastructure state as database truth.
- No container, route, pod, or deployment shadow state owned by GoPanel.
- No numerical file limits.
- No Alpine.js by default.
- No generic table abstraction.
- No arbitrary HTTP fetch endpoint.
- No generic Docker/Caddy/Vault/Kubernetes API passthrough.
- No generic credential database.
- No user-controlled arbitrary credential file paths or environment-variable dereferencing.
- No OIDC in v1.
- No enterprise RBAC or policy engine.
- No message bus.
- No Redis.
- No background worker platform.
- No Kubernetes operator.
- No event framework.
- No plugin architecture.
- No requirement for REST PUT/PATCH/DELETE; HTML uses GET + POST.
- No secret values rendered to UI.
- No automatic retries for mutation operations unless explicitly safe and idempotent.
- No unbounded live-log buffering.
- No backup subsystem.
- No durable `error_log` table in v1.
- No unfiltered backend, SDK, database, filesystem, or infrastructure errors in browser surfaces, including the administrator Error Panel.

## 16. v1 Architecture Invariants

The following statements define the v1 architecture and should not change casually during implementation:

1. GoPanel is a control interface, not an infrastructure state store.
2. Managed systems remain authoritative for their own operational state.
3. GoPanel owns authentication, hashed server-side sessions, connection configuration, audit history, bounded process-local diagnostics, and presentation.
4. Temporary status and Error Panel records disappear on restart and are never presented as durable history.
5. The browser never talks directly to privileged infrastructure administration APIs.
6. Every infrastructure operation is explicit and typed; there is no generic passthrough request function.
7. Credential references are resolved only by their owning module against allowed configuration.
8. Secret values are used server-side and never rendered into HTML.
9. Every accepted privileged control operation is authorized server-side, protected against CSRF where applicable, bounded by context, and durably recorded as `attempted` before execution, then resolved to `success` or `failed` when known.
10. Real HTML works first; HTMX is progressive enhancement.
11. Mobile and desktop behavior are designed in the same vertical slice.
12. SQLite is the only v1 database and GoPanel remains a single-node application.
13. `cmd/gopanel/main.go` wires; `internal/app` owns process lifecycle.
14. Shutdown drains active requests for a bounded period before remaining work is canceled.
15. New complexity must be justified by a demonstrated requirement, not anticipated scale.
16. User-initiated actions never fail silently; every outcome is visible and associated with the affected form or resource.
17. Blocked and denied actions explain the required authorization and safe next step.
18. Fields and controls use clear labels, concise help when needed, and actionable validation errors.
19. Backend failures share one error reference across every applicable UI, Error Panel, and structured-log surface.
20. The Error Panel is administrator-only, process-local, bounded, and limited to safely rendered technical detail.

---

**GoPanel identity:** One Go process, one SQLite file, server-rendered HTML with real links and forms enhanced by HTMX, a small number of explicit privileged infrastructure operations, external systems authoritative for their own state, opaque hashed sessions, constrained credential references, auditable control operations, bounded external calls, visible user outcomes, redacted administrator diagnostics, and deliberate mobile-first UX.
