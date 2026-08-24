# GoPanel — Coding Style

## 1. Purpose

GoPanel code should be easy to read, change, test, and remove.

This document constrains how maintainers and coding agents write code in this repository. It is not a replacement for standard Go formatting or linting rules.

The governing principle is:

> **Simple over clever.**

A correct solution that is easy to understand is preferred over a shorter, more abstract, or more reusable solution that hides behavior.

---

## 2. Core Rule

Use the smallest abstraction that protects the real failure mode.

- If the risk is **this operation might fail**, return an error.
- If the risk is **this operation may partially complete**, add recovery or explicit outcome handling.
- If the risk is **this operation crosses a trust boundary**, add authorization, validation, and audit where required.
- If none of those apply, call the function.

Do not build a framework for a problem that is already solved by a normal function call.

---

## 3. Single Level of Abstraction Principle

Follow **SLAP — Single Level of Abstraction Principle**.

A function should read at one conceptual level.

Good:

```go
func StopContainer(ctx context.Context, server Server, containerID string) error {
    client, err := openDockerClient(server)
    if err != nil {
        return err
    }
    defer client.Close()

    return client.Stop(ctx, containerID)
}
```

Avoid mixing high-level workflow with low-level parsing, validation, networking, logging, and formatting inside one large function.

Also avoid hiding a simple workflow behind many tiny functions that only rename the next call.

Good code should let a maintainer understand the main path without jumping through five files.

---

## 4. Keep Control Flow Shallow

Prefer early returns.

Good:

```go
func HandleStop(w http.ResponseWriter, r *http.Request) {
    user, err := CurrentUser(r)
    if err != nil {
        WriteUnauthorized(w)
        return
    }

    if !user.CanAdmin() {
        WriteForbidden(w)
        return
    }

    if err := stopContainer(r.Context(), r); err != nil {
        WriteActionError(w, err)
        return
    }

    WriteSuccess(w)
}
```

Avoid deeply nested logic:

```go
if user != nil {
    if user.CanAdmin() {
        if request.Valid() {
            if client != nil {
                // ...
            }
        }
    }
}
```

As a practical rule, if normal application logic is nesting more than two or three levels deep, restructure it with early returns or a smaller operation.

Do not replace nesting with a maze of callbacks.

---

## 5. Functions Should Have One Clear Job

A function should have one obvious reason to change.

Prefer:

```go
ValidateServerInput(...)
CreateServer(...)
ListContainers(...)
StopContainer(...)
WriteAuditResult(...)
```

Avoid broad functions such as:

```go
ProcessServer(...)
HandleEverything(...)
ExecuteOperation(...)
ManageResources(...)
DoAction(...)
```

If the name cannot describe the operation clearly, the function may be doing too much.

---

## 6. Explicit Operations Over Generic Engines

GoPanel is a privileged control interface. Explicit operations are easier to review and safer to maintain.

Prefer:

```go
ListContainers(...)
StartContainer(...)
StopContainer(...)
ListRoutes(...)
CreateRoute(...)
DeleteRoute(...)
```

Avoid:

```go
DockerRequest(method, path, body)
ExecuteResourceAction(kind, action, payload)
GenericMutation(...)
CallIntegration(name, operation, args)
```

Do not create generic privileged request paths for convenience.

A little duplication is acceptable when it keeps authority and behavior obvious.

---

## 7. Do Not Abstract Before Repetition Is Real

Do not create an abstraction because two pieces of code look similar once.

First ask:

1. Are they solving the same problem?
2. Do they change for the same reason?
3. Does the abstraction remove meaningful duplication?
4. Does it make the call site easier to understand?
5. Does it preserve important differences?

If the answer is unclear, keep the code separate.

Prefer duplication over the wrong abstraction.

Refactor when the repeated pattern is proven, not predicted.

---

## 8. No Helper or Utility Dumping Grounds

Do not create generic packages or files such as:

```text
utils/
helpers/
common/
misc/
shared/
```

unless the name describes a real, stable domain boundary.

A function belongs near the behavior that owns it.

Prefer:

```text
internal/auth/session.go
internal/container/logs.go
internal/server/validation.go
```

over:

```text
internal/utils/http.go
internal/helpers/validation.go
```

---

## 9. Keep Data Ownership Obvious

GoPanel does not duplicate external infrastructure state as durable truth.

Code should make ownership easy to see.

Examples:

- Docker owns container state.
- Caddy owns proxy route state.
- Kubernetes owns pod and deployment state.
- GoPanel owns authentication, sessions, connection configuration, audit history, bounded process-local diagnostics, and presentation.
- Temporary status observations and Error Panel records belong in process memory unless a later requirement explicitly changes that.

Do not add database fields merely because displaying the value would be convenient.

---

## 10. Keep Error Handling Explicit

Return errors where the caller can make a useful decision.

Prefer:

```go
container, err := service.GetContainer(ctx, id)
if err != nil {
    return err
}
```

Avoid swallowing errors:

```go
container, _ := service.GetContainer(ctx, id)
```

Avoid panic for normal runtime failures.

Use panic only for conditions that represent programmer error or an impossible initialization state where continuing is unsafe.

User-facing errors must remain stable and understandable. Raw SDK, database, or infrastructure errors are mapped into safe technical diagnostics before entering structured logs or browser surfaces.

Every backend or managed-system failure is recorded once at the HTTP, background-work, or lifecycle boundary. The record receives one opaque error reference shared by every applicable surface:

- the plain-language UI result for request-bound or surfaced background failures;
- the administrator-only Error Panel entry while the process remains available;
- the structured backend log; and
- the audit correlation when one exists.

Fatal startup failures emit the reference in the structured log before exit. They do not claim a browser or Error Panel surface exists before serving begins.

The owning integration maps its error into a safe technical diagnostic. The diagnostic recorder applies final redaction before writing the bounded buffer or structured log. Do not pass `err.Error()` directly to a view, persist unfiltered errors, or create duplicate user-facing diagnostic records at several layers. Mandatory security and audit-integrity events may be logged where they occur, but they use safe detail and the existing correlation ID.

Never record or display passwords, tokens, credentials, authorization headers, secret values, raw request bodies, or untrusted container-log output.

No user-initiated action may fail silently. Full-page, HTMX, and JavaScript-disabled paths all render a visible success, rejection, denial, timeout, or failure outcome.

---

## 11. Do Not Add Recovery Unless Partial Completion Exists

Recovery logic is justified when an operation can leave the system in a meaningful partial state.

Do not add rollback, compensation, retry orchestration, or transaction-like abstractions to ordinary calls that either succeed or fail cleanly.

When partial completion is possible, describe it explicitly.

Example:

```text
audit row created
→ Docker mutation attempted
→ audit result updated
```

If final audit persistence fails, preserve the known outcome and log the ambiguity. Do not invent a distributed transaction to make two systems behave like one.

---

## 12. Writes Must Be Deliberate

For state-changing operations:

- authorize on the server;
- validate input;
- require CSRF protection for browser POSTs;
- use bounded contexts;
- do not blindly retry;
- define repeat behavior;
- write audit evidence when required.

A privileged control operation includes a managed-system change, a durable server or connection configuration change, or reusable-secret resolution/use. These operations always use the two-phase audit contract. Authentication/session housekeeping, health polling, migrations, pure reads, and the local first-admin bootstrap use their own security or operational logs instead.

Do not rely on hidden UI controls as security.

Do not assume disabling a button guarantees idempotency.

---

## 13. Reads Should Stay Reads

GET requests should not change infrastructure or application state.

Use POST for actions that cause a change or deliberate external side effect.

Do not hide mutation behavior inside page loads, health checks, render functions, or getters.

### 13.1 Session-bound CSRF

Good: create one process-local random signing key, use distinct login and authenticated HMAC-SHA-256 domains, length-frame every MAC field, bind authenticated forms to the raw session cookie credential, bind login forms to a signed expiring anonymous context, validate with `hmac.Equal`, and return visible `403` HTML before mutation.

Bad: `sha256(session + nonce)`, binding to user ID or stored session hash, accepting a missing token because origin headers are absent, trusting cookie expiry, rotating login context after a wrong password, persisting CSRF material, or creating a generic token framework.

The anonymous context is not a session. Failed credentials preserve it; successful login deletes it and creates a fresh independent session. Raw session/context credentials and CSRF tokens never enter URLs, logs, diagnostics, audit, local storage, or session storage.

---

## 14. Keep Infrastructure Clients Typed

Each integration owns its own validation and client behavior.

Prefer:

```go
DockerClient
CaddyClient
VaultClient
KubernetesClient
```

Avoid generic network or credential resolvers that can be turned into arbitrary outbound access.

Do not introduce:

```go
FetchURL(...)
TestConnection(address, type)
ResolveCredential(reference)
```

when the owning module can perform the specific operation safely.

---

## 15. Contexts Must Be Bounded

External calls must receive a context.

Operations that may block must have a bounded lifetime.

Good:

```go
ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
defer cancel()

return client.List(ctx)
```

Do not create background contexts deep inside services to escape cancellation.

Do not start unmanaged goroutines from request handlers.

---

## 16. Keep Concurrency Boring

Concurrency is allowed when it solves a demonstrated need.

Prefer:

- one clear goroutine owner;
- bounded worker counts;
- channels or semaphores with obvious lifetimes;
- cancellation through context;
- predictable shutdown.

Avoid:

- goroutines started implicitly from constructors;
- unbounded fan-out;
- background work with no owner;
- custom worker frameworks for a handful of operations.

If sequential code is fast enough, keep it sequential.

---

## 17. Views Contain Presentation, Not Business Logic

Templ components should render prepared data.

They may decide how information looks. They should not decide whether an operation is authorized or perform infrastructure work.

Good:

```text
handler
→ service
→ view model
→ Templ component
```

Avoid SDK calls, database queries, authorization decisions, or mutation logic inside views.

Views render prepared error and form state:

- visible labels or accessible names for every field and control;
- concise help when the expected value is not obvious;
- field-specific validation explaining acceptable input;
- a persistent error reference for backend failures;
- `See Error Log` only when the prepared view model says the authenticated user is an administrator; and
- a clear denial or sign-in prompt when the handler has already determined the authorization outcome.

Placeholder text and icons do not replace labels. Views do not inspect raw errors or perform redaction.

---

## 18. Full Page and HTMX Must Share Data Preparation

Do not duplicate the business path for HTMX.

Prefer:

```go
page, err := buildServerDetailView(ctx, id)
if err != nil {
    return err
}

if IsHTMXRequest(r) {
    return ServerDetail(page)
}

return ServerDetailPage(page)
```

One data path. Two renderings.

---

## 19. Naming Must Explain Intent

Use full words.

Prefer:

```go
server
container
credentialReference
connectionType
auditResult
```

Avoid:

```go
srv
ctr
credRef
connTyp
res
mgr
```

Function names should be verbs plus concrete nouns:

```go
ListServers
CreateServer
StopContainer
ValidateContext
RecordAuditResult
```

Types and components should be nouns.

Avoid names that hide responsibility:

```go
Manager
Processor
Engine
Coordinator
HandlerFactory
```

unless that term describes a real architectural role.

---

## 20. Files Should Follow Responsibility

There is no arbitrary file-count or line-count limit.

Split a file when doing so makes ownership clearer.

Good:

```text
container/
    list.go
    actions.go
    logs.go
    handler.go
```

Do not keep unrelated operations in one large file just to reduce file count.

Do not split simple logic into many tiny files merely to satisfy a style preference.

The reader should be able to predict where a behavior lives.

---

## 21. Comments Explain Why

Do not narrate obvious code.

Bad:

```go
// Increment count by one.
count++
```

Useful:

```go
// Do not retry this write automatically.
// The remote API may have completed the operation before timing out.
```

Comments should explain constraints, risk, or non-obvious intent.

If a comment is required to explain confusing code, first consider simplifying the code.

---

## 22. Agent Change Discipline

Coding agents must make the smallest coherent change that satisfies the task.

Before adding code:

1. Inspect the existing package and nearby patterns.
2. Reuse an existing abstraction when it already fits.
3. Do not create a parallel implementation of the same responsibility.
4. Do not refactor unrelated code unless the requested change cannot be made safely without it.
5. Do not add dependencies for functionality already available in Go or the existing project.
6. Do not add speculative extension points, plugin hooks, interfaces, factories, or configuration switches.
7. Preserve architecture and security boundaries.
8. Add or update tests that prove the changed behavior.

A task is not permission to redesign the surrounding codebase.

---

## 23. Interfaces Are Earned

Do not create interfaces merely to wrap every struct.

Use an interface when there is a real boundary or multiple implementations that matter.

Good reasons:

- external client boundary that must be replaced in tests;
- two real implementations already exist;
- a package needs to depend on behavior rather than a concrete implementation.

Bad reason:

> We might need another implementation someday.

Prefer concrete types until abstraction provides a current benefit.

---

## 24. Dependencies Must Pay for Themselves

Before adding a dependency, ask:

- What problem does it solve?
- Can the standard library or current stack solve it clearly?
- Does it create another runtime or build requirement?
- Who will maintain it?
- Is the functionality large enough to justify the dependency?

Do not add a dependency to save a few lines of straightforward code.

---

## 25. Tests Should Protect Behavior

Tests should focus on contracts and failure modes.

Prefer tests for:

- authorization;
- validation;
- state transitions;
- failure handling;
- no-silent-error behavior across full-page, HTMX, and JavaScript-disabled paths;
- error-reference correlation;
- Error Panel authorization, bounded retention, and reset-on-restart disclosure;
- diagnostic redaction;
- clear labels, accessible names, expected-input help, and field-level correction;
- audit behavior;
- timeout/cancellation;
- full-page and fragment consistency;
- important mobile/user workflows where applicable.

Do not test implementation trivia solely to increase coverage.

A refactor that preserves behavior should not require rewriting every test.

### 25.1 Test and evidence integrity

Tests prove externally observable contracts. Use real routing, rendering, response writers, status codes, headers, bodies, embedded bytes, and temporary databases at the boundary under test. Mocks may isolate dependencies outside that boundary; they may not replace the behavior being proved.

Do not generate expected values with the implementation under test, duplicate implementation constants as the only assertion, assert only that execution completed, add production behavior solely for a test, weaken an assertion to obtain success, or use coverage percentage as proof.

Required Go tests run uncached with `go clean -testcache` and `go test -count=1`. When a critical test is introduced or materially changed, record the protected file hash and complete source digest, temporarily introduce one known-wrong behavior without changing the test, observe failure for the intended reason, restore the exact file and digest, then rerun the real check successfully.

Required failures, retries, skips, and limitations remain visible. Do not use `t.Skip`, `test.skip`, filtered execution, `|| true`, or `continue-on-error` and then report full success.

Use these result states exactly:

- `PASS`: the check executed, observed the requirement, and produced inspectable evidence.
- `FAIL`: the check executed and did not observe the requirement, or a required command failed.
- `NOT RUN`: the check did not execute.
- `INCONCLUSIVE`: the check executed but did not prove the requirement.

Local validation identifies the baseline commit and complete dirty-diff digest. Independent CI evidence identifies the exact committed source. Local workflow parity is not CI evidence, and neither state is phase completion by itself.

---

## 26. Refactoring Rule

Refactor when the current structure is actively making the requested change harder, riskier, or more repetitive.

Do not refactor because:

- a pattern could be more abstract;
- a file looks unfashionable;
- a framework would be more extensible;
- an agent prefers another architecture.

When refactoring is necessary:

1. keep the scope narrow;
2. preserve behavior;
3. separate unrelated cleanup where practical;
4. explain the reason in the commit or maintainer documentation if the decision is lasting.

---

## 27. Code Review Questions

Before accepting code, ask:

- Can a new maintainer understand the main path quickly?
- Is control flow shallow?
- Does each function operate at one clear level?
- Is the abstraction solving a real current problem?
- Did we preserve explicit privileged operations?
- Are failure and partial-completion cases visible?
- Does every user-initiated outcome appear in the affected form or resource?
- Can an administrator follow the error reference without exposing unfiltered diagnostics to other users?
- Do fields and controls explain their purpose and accepted input without excessive microcopy?
- Is data ownership still clear?
- Did we add complexity that the requirement did not ask for?
- Can any new generic function become a security escape hatch?
- Is there a simpler implementation that remains correct?

If the simpler version protects the same real failure modes, use the simpler version.

---

---

# Good vs. Bad Patterns for Agentic Go & HTMX

These examples are direct guidance for coding agents.

Agents commonly fail by making a correct task unnecessarily abstract. They add builders, generic repositories, utility packages, callback layers, reflection, custom JavaScript, or framework-like helpers before the codebase needs them.

The examples below show the preferred GoPanel shape.

When two implementations are both correct, prefer the one that exposes the operation, failure path, and authority most clearly.

## 28. Simple Over Clever

### Bad: hidden state and fluent execution

```go
// BAD: execution state is hidden inside a chain.
type QueryBuilder struct {
    db      *sql.DB
    filters []string
    err     error
}

func NewQueryBuilder(db *sql.DB) *QueryBuilder {
    return &QueryBuilder{db: db}
}

func (q *QueryBuilder) FilterUser(id string) *QueryBuilder {
    if q.err != nil {
        return q
    }

    q.filters = append(q.filters, "user_id="+id)
    return q
}

func (q *QueryBuilder) WithProfile() *QueryBuilder {
    // More hidden state mutation.
    return q
}

func (q *QueryBuilder) RenderHTMX(w http.ResponseWriter) {
    // Database work is now buried inside rendering.
}
```

This makes a simple request harder to follow:

- state changes across chained calls;
- errors may be stored and surfaced later;
- the query layer now knows about rendering;
- execution is no longer obvious from the call site.

### Good: explicit linear flow

```go
func HandleGetUserProfile(
    db *sql.DB,
    diagnostics *DiagnosticRecorder,
) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userID := r.PathValue("id")
        if userID == "" {
            http.Error(w, "missing user id", http.StatusBadRequest)
            return
        }

        user, err := findUserByID(r.Context(), db, userID)
        if errors.Is(err, ErrUserNotFound) {
            w.WriteHeader(http.StatusNotFound)
            renderTemplate(w, "user_not_found.html", nil)
            return
        }
        if err != nil {
            record := diagnostics.Record(
                r.Context(),
                DiagnosticInput{
                    UserID:          CurrentUserID(r),
                    Action:          "get_user_profile",
                    Target:          userID,
                    Component:       "sqlite",
                    HTTPStatus:      http.StatusInternalServerError,
                    PublicMessage:   "Profile could not be loaded.",
                    TechnicalDetail: SafeSQLiteDiagnostic(err),
                },
            )

            w.WriteHeader(http.StatusInternalServerError)
            renderProfileError(
                w,
                "Profile could not be loaded.",
                record.ID,
            )
            return
        }

        renderTemplate(w, "user_profile.html", user)
    }
}
```

The main path is visible from top to bottom.

Do not replace a readable sequence with a builder, pipeline, fluent API, or state machine unless the problem actually requires one.

---

## 29. Shallow Control Flow

### Bad: pyramid nesting

```go
func HandleStopContainer(w http.ResponseWriter, r *http.Request) {
    user := CurrentUser(r.Context())

    if user != nil {
        if user.Role == "admin" {
            id := r.PathValue("id")
            if id != "" {
                if err := StopContainer(r.Context(), id); err == nil {
                    renderSuccess(w)
                } else {
                    renderFailure(w)
                }
            } else {
                http.Error(w, "missing id", http.StatusBadRequest)
            }
        } else {
            http.Error(w, "forbidden", http.StatusForbidden)
        }
    } else {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
    }
}
```

### Good: guard clauses and middleware

```go
func HandleStopContainer(
    service *ContainerService,
    diagnostics *DiagnosticRecorder,
) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        containerID := r.PathValue("id")
        if containerID == "" {
            http.Error(w, "missing container id", http.StatusBadRequest)
            return
        }

        err := service.StopContainer(r.Context(), containerID)
        if err != nil {
            record := recordStopFailure(
                r,
                diagnostics,
                service,
                containerID,
                err,
            )

            w.WriteHeader(http.StatusBadGateway)
            renderContainerError(
                w,
                buildContainerErrorView(
                    record,
                    CurrentUser(r).IsAdmin(),
                ),
            )
            return
        }

        renderContainerStopped(w, containerID)
    }
}
```

Privilege is visible at route registration:

```go
router.With(
    RequireLogin,
    RequireAdmin,
    RequireCSRF,
).Post(
    "/containers/{id}/stop",
    HandleStopContainer(containerService),
)
```

The handler does not need to recreate authentication and authorization logic that middleware already owns.

---

## 30. HTMX: Real HTML First

Do not build a JavaScript request framework around HTMX.

### Bad: custom client-side request engine

```html
<button
  onclick="sendContainerAction('/api/containers/123', 'stop')"
  data-container-id="123">
  Stop
</button>

<script>
async function sendContainerAction(url, action) {
  const response = await fetch(url, {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({action})
  });

  const data = await response.json();
  document.querySelector("#container-123").innerHTML = data.html;
}
</script>
```

This duplicates what normal HTML and HTMX already do.

### Good: form works without JavaScript, HTMX enhances it

```html
<form
  method="post"
  action="/containers/123/stop"
  hx-post="/containers/123/stop"
  hx-target="#container-123"
  hx-swap="outerHTML">

  <input
    type="hidden"
    name="csrf_token"
    value="{{ .CSRFToken }}">

  <button
    type="submit"
    hx-confirm="Stop container nginx?">
    Stop
  </button>
</form>
```

GoPanel uses GET for reads and POST for mutations. Do not introduce `hx-delete`, PUT, PATCH, or DELETE merely to make the HTTP shape look more REST-like.

The base layout loads the project-owned `application.js`. Its `htmx:beforeSwap` handler swaps GoPanel's `4xx` and `5xx` HTML error fragments into their declared targets. Its transport fallback handles `htmx:sendError` and `htmx:timeout` with fixed safe text. Do not rely on HTMX 1.9.12's default error behavior, which leaves error responses unswapped.

Use target-specific confirmation text. Avoid:

```text
Are you sure?
```

Prefer:

```text
Stop container nginx?
Delete proxy route api.example.com?
```

---

## 31. HTMX Responses Are HTML

If the browser asked for a fragment, return a fragment.

### Bad: JSON wrapper carrying HTML

```go
type APIResponse struct {
    Success bool   `json:"success"`
    HTML    string `json:"html"`
    Error   string `json:"error"`
}

func HandleStopContainer(w http.ResponseWriter, r *http.Request) {
    response := APIResponse{
        Success: true,
        HTML:    "<div>stopped</div>",
    }

    _ = json.NewEncoder(w).Encode(response)
}
```

This forces JavaScript to unpack HTML that the server could have returned directly.

### Good: render the target fragment

```go
func HandleStopContainer(
    service *ContainerService,
    diagnostics *DiagnosticRecorder,
) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        containerID := r.PathValue("id")

        err := service.StopContainer(
            r.Context(),
            containerID,
        )
        if err != nil {
            record := recordStopFailure(
                r,
                diagnostics,
                service,
                containerID,
                err,
            )

            w.WriteHeader(http.StatusBadGateway)
            renderContainerError(
                w,
                buildContainerErrorView(
                    record,
                    CurrentUser(r).IsAdmin(),
                ),
            )
            return
        }

        container, err := service.GetContainer(
            r.Context(),
            containerID,
        )
        if err != nil {
            record := recordRefreshFailure(
                r,
                diagnostics,
                service,
                containerID,
                err,
            )

            w.WriteHeader(http.StatusBadGateway)
            renderContainerError(
                w,
                buildContainerErrorView(
                    record,
                    CurrentUser(r).IsAdmin(),
                ),
            )
            return
        }

        renderContainerRow(w, container)
    }
}
```

JSON is acceptable when a real non-HTML consumer needs JSON.

Do not create JSON APIs solely because an agent considers them more conventional.

---

## 32. Full Page and HTMX Must Share Data Preparation

### Bad: duplicate business paths

```go
func HandleServerDetail(w http.ResponseWriter, r *http.Request) {
    if IsHTMXRequest(r) {
        server, _ := loadServer(r.Context(), r.PathValue("id"))
        containers, _ := loadContainers(r.Context(), server.ID)
        renderServerFragment(w, server, containers)
        return
    }

    server, _ := loadServer(r.Context(), r.PathValue("id"))
    containers, _ := loadContainers(r.Context(), server.ID)
    renderServerPage(w, server, containers)
}
```

Problems:

- errors are ignored;
- data loading is duplicated;
- full-page and fragment behavior can drift.

### Good: one view model, two renderings

```go
type ServerDetailView struct {
    Server     Server
    Containers []Container
}

func buildServerDetailView(
    ctx context.Context,
    serverService *ServerService,
    containerService *ContainerService,
    serverID string,
) (ServerDetailView, error) {
    server, err := serverService.GetServer(ctx, serverID)
    if err != nil {
        return ServerDetailView{}, err
    }

    containers, err := containerService.ListContainers(
        ctx,
        serverID,
    )
    if err != nil {
        return ServerDetailView{}, err
    }

    return ServerDetailView{
        Server:     server,
        Containers: containers,
    }, nil
}

func HandleServerDetail(
    serverService *ServerService,
    containerService *ContainerService,
    diagnostics *DiagnosticRecorder,
) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        view, err := buildServerDetailView(
            r.Context(),
            serverService,
            containerService,
            r.PathValue("id"),
        )
        if err != nil {
            record := recordServerDetailFailure(
                r,
                diagnostics,
                serverService,
                containerService,
                err,
            )

            w.WriteHeader(http.StatusBadGateway)
            renderServerError(
                w,
                buildServerErrorView(
                    record,
                    CurrentUser(r).IsAdmin(),
                ),
            )
            return
        }

        if IsHTMXRequest(r) {
            renderServerDetailFragment(w, view)
            return
        }

        renderServerDetailPage(w, view)
    }
}
```

The useful abstraction is the prepared page data, not a rendering framework.

The resource-specific failure recorders in these examples apply safe error mapping and create the single correlated diagnostic described in §35. They do not render raw errors or create a second business path.

The non-`2xx` fragments remain visible because the base layout uses the response-handling contract described in §30.

---

## 33. Explicit Privileged Operations

Generic engines erase authority.

### Bad: generic CRUD or reflection engine

```go
func GenericHandler[T any](
    db *sql.DB,
) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var entity T

        // Reflection.
        // Dynamic query generation.
        // Dynamic authorization.
        // Dynamic mutation selection.
        _ = entity
    }
}
```

### Bad: generic infrastructure passthrough

```go
func ExecuteInfrastructureAction(
    ctx context.Context,
    system string,
    method string,
    path string,
    body []byte,
) ([]byte, error) {
    // Generic privileged network access.
    return nil, nil
}
```

### Good: concrete operation

```go
func StopContainer(
    ctx context.Context,
    client *DockerClient,
    containerID string,
) error {
    return client.Stop(ctx, containerID)
}
```

```go
func CreateRoute(
    ctx context.Context,
    client *CaddyClient,
    input CreateRouteInput,
) error {
    return client.CreateRoute(ctx, input)
}
```

A reviewer should be able to tell what authority the function exercises from its name.

Prefer:

```go
ListContainers
StartContainer
StopContainer
ListRoutes
CreateRoute
DeleteRoute
```

Avoid:

```go
ExecuteAction
CallIntegration
GenericMutation
ResourceManager
```

---

## 34. Trust Boundaries Belong to the Owning Module

Do not build one generic credential or connection resolver.

### Bad: arbitrary reference resolution

```go
func ResolveCredential(
    reference string,
) ([]byte, error) {
    if strings.HasPrefix(reference, "env:") {
        name := strings.TrimPrefix(reference, "env:")
        return []byte(os.Getenv(name)), nil
    }

    if strings.HasPrefix(reference, "file:") {
        path := strings.TrimPrefix(reference, "file:")
        return os.ReadFile(path)
    }

    return nil, errors.New("unsupported reference")
}
```

This lets a string become filesystem or environment access.

### Good: integration owns its configuration

```go
type CaddyConfig struct {
    AdminURL string
}

func (c CaddyConfig) Validate() error {
    if c.AdminURL == "" {
        return errors.New("caddy admin URL is required")
    }

    return nil
}

func NewCaddyClient(
    config CaddyConfig,
) (*CaddyClient, error) {
    if err := config.Validate(); err != nil {
        return nil, err
    }

    return &CaddyClient{
        adminURL: config.AdminURL,
    }, nil
}
```

Docker validates Docker connection configuration.

Caddy validates Caddy configuration.

Kubernetes validates allowed Kubernetes contexts.

Vault validates Vault configuration if Vault ships.

Do not create:

```go
FetchURL(...)
TestConnection(address, type)
ResolveCredential(reference)
```

when the owning module can perform a specific typed operation.

---

## 35. Explicit Error Handling

Never ignore errors.

### Bad

```go
server, _ := store.GetServer(ctx, id)
containers, _ := docker.ListContainers(ctx, server.ID)

return renderServer(containers)
```

### Good

```go
server, err := store.GetServer(ctx, id)
if err != nil {
    return fmt.Errorf("get server %q: %w", id, err)
}

containers, err := docker.ListContainers(
    ctx,
    server.ID,
)
if err != nil {
    return fmt.Errorf(
        "list containers for server %q: %w",
        id,
        err,
    )
}

return renderServer(containers)
```

At the HTTP edge, keep the browser message stable:

```go
if err != nil {
    safeDetail := docker.SafeDiagnostic(err)

    record := diagnostics.Record(
        r.Context(),
        DiagnosticInput{
            UserID:          CurrentUserID(r),
            Action:          "list_containers",
            Target:          serverID,
            Component:       "docker",
            HTTPStatus:      http.StatusBadGateway,
            PublicMessage:   "Docker did not respond.",
            TechnicalDetail: safeDetail,
        },
    )

    renderContainerError(
        w,
        ContainerErrorView{
            Message:      "Docker did not respond. Try again.",
            Reference:    record.ID,
            ShowLogLink:  CurrentUser(r).IsAdmin(),
        },
    )
    return
}
```

The user gets an understandable message and a stable reference.

An administrator can open the matching safely rendered Error Panel record.

The diagnostic recorder emits the correlated structured log entry and owns the fixed 200-entry process-local buffer. The handler does not log the same raw error separately.

Bad:

```go
logger.Error("docker failed", "error", err)
renderContainerError(w, err.Error())
```

Administrator-only access is not permission to display unfiltered errors. Safe diagnostic mapping remains mandatory.

---

## 36. Partial Completion Needs Explicit Outcome Handling

Do not add recovery machinery to operations that either succeed or fail cleanly.

Add it when a real operation can partly complete.

### Bad: pretending Docker and SQLite share one transaction

```go
tx, _ := db.BeginTx(ctx, nil)

_ = docker.StopContainer(ctx, containerID)
_ = writeAudit(tx, "success")

_ = tx.Commit()
```

The SQLite transaction cannot roll back Docker.

### Good: preserve the real sequence

```go
auditID, err := auditStore.RecordAttempt(
    ctx,
    actorID,
    "stop_container",
    containerID,
)
if err != nil {
    return ErrAuditUnavailable
}

err = docker.StopContainer(ctx, containerID)
if err != nil {
    _ = auditStore.RecordResult(
        ctx,
        auditID,
        AuditFailed,
    )
    return err
}

err = auditStore.RecordResult(
    ctx,
    auditID,
    AuditSuccess,
)
if err != nil {
    logger.Error(
        "audit update failed after successful stop",
        "audit_id", auditID,
        "container_id", containerID,
        "detail", auditStore.SafeDiagnostic(err),
    )

    return ErrMutationSucceededAuditIncomplete
}

return nil
```

The code tells the truth about what happened.

Do not add a distributed transaction framework to hide a failure mode that cannot actually be made atomic.

---

## 37. Bounded Contexts

External calls must not wait forever.

### Bad

```go
func ListContainers(
    client *DockerClient,
) ([]Container, error) {
    return client.List(context.Background())
}
```

This ignores request cancellation and shutdown.

### Good

```go
func ListContainers(
    ctx context.Context,
    client *DockerClient,
) ([]Container, error) {
    ctx, cancel := context.WithTimeout(
        ctx,
        DefaultDockerTimeout,
    )
    defer cancel()

    return client.List(ctx)
}
```

Do not create `context.Background()` deep in request code merely to escape cancellation.

---

## 38. Boring Concurrency

Do not create unbounded goroutines.

### Bad: helper dumping ground plus unbounded fan-out

```go
package utils

func ProcessBatchAsync(items []string) {
    for _, item := range items {
        go func(value string) {
            DoSomething(value)
        }(item)
    }
}
```

Problems:

- no context;
- no error propagation;
- no concurrency bound;
- no obvious owner;
- generic `utils` package.

### Good: bounded standard-library pattern

```go
func CheckServers(
    ctx context.Context,
    servers []Server,
    maxConcurrent int,
    check func(context.Context, Server) error,
) error {
    if maxConcurrent < 1 {
        return errors.New(
            "maxConcurrent must be at least 1",
        )
    }

    semaphore := make(
        chan struct{},
        maxConcurrent,
    )

    errs := make(
        chan error,
        len(servers),
    )

    var wg sync.WaitGroup

    for _, server := range servers {
        server := server

        wg.Add(1)
        go func() {
            defer wg.Done()

            select {
            case semaphore <- struct{}{}:
                defer func() {
                    <-semaphore
                }()
            case <-ctx.Done():
                errs <- ctx.Err()
                return
            }

            if err := check(ctx, server); err != nil {
                errs <- err
            }
        }()
    }

    wg.Wait()
    close(errs)

    for err := range errs {
        if err != nil {
            return err
        }
    }

    return nil
}
```

If `golang.org/x/sync/errgroup` is already an accepted dependency or clearly makes the current code simpler, this is also acceptable:

```go
func CheckServers(
    ctx context.Context,
    servers []Server,
    maxConcurrent int,
    check func(context.Context, Server) error,
) error {
    group, ctx := errgroup.WithContext(ctx)
    group.SetLimit(maxConcurrent)

    for _, server := range servers {
        server := server

        group.Go(func() error {
            return check(ctx, server)
        })
    }

    return group.Wait()
}
```

Do not add `x/sync` merely because the agent prefers `errgroup`.

If sequential code is sufficient, keep it sequential.

---

## 39. Concrete Types First

Do not create interfaces by reflex.

### Bad

```go
type ServerManager interface {
    Get(context.Context, string) (Server, error)
    Create(context.Context, Server) error
    Update(context.Context, Server) error
    Delete(context.Context, string) error
}

type ServerManagerImpl struct {
    repository ServerRepository
}
```

There is one implementation and several abstraction layers.

### Good

```go
type ServerService struct {
    store *ServerStore
}

func (s *ServerService) GetServer(
    ctx context.Context,
    id string,
) (Server, error) {
    return s.store.GetServer(ctx, id)
}
```

An interface may be useful at a real external boundary:

```go
type ContainerClient interface {
    List(
        ctx context.Context,
    ) ([]Container, error)

    Stop(
        ctx context.Context,
        id string,
    ) error
}
```

Use interfaces because a current consumer benefits from substitution, not because another implementation might exist someday.

---

## 40. Behavioral Tests Over Mock Frameworks

Tests should protect observable behavior and important failure modes.

### Bad: testing SQL formatting through mocks

```go
func TestUserHandler_Mock(t *testing.T) {
    mockDB := new(MockDB)

    mockDB.
        On(
            "QueryRowContext",
            mock.Anything,
            "SELECT id, email, name FROM users WHERE id = ?",
            "missing-user",
        ).
        Return(sql.ErrNoRows)

    // Breaks when harmless query formatting changes.
}
```

### Good: test the HTTP behavior

```go
func TestHandleGetUserProfile_NotFound(
    t *testing.T,
) {
    db := setupTestSQLite(t)
    handler := HandleGetUserProfile(db)

    req := httptest.NewRequest(
        http.MethodGet,
        "/users/non-existent-id",
        nil,
    )
    req.SetPathValue(
        "id",
        "non-existent-id",
    )

    rec := httptest.NewRecorder()

    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusNotFound {
        t.Fatalf(
            "expected status %d, got %d",
            http.StatusNotFound,
            rec.Code,
        )
    }

    if !strings.Contains(
        rec.Body.String(),
        "User Not Found",
    ) {
        t.Fatalf(
            "expected user-not-found content",
        )
    }
}
```

Use a lightweight real SQLite database or fixture when practical.

Fake or mock external boundaries when necessary.

Do not mock every internal call simply because a mocking library can.

---

## 41. Narrow Refactoring

A feature request is not permission to redesign the surrounding codebase.

### Bad agent response

Task:

```text
Add container stop confirmation.
```

Agent changes:

```text
- adds GenericAction[T]
- rewrites route registration
- creates shared/helpers
- creates a JavaScript modal framework
- renames unrelated handlers
- changes container listing behavior
```

### Good agent response

Task:

```text
Add container stop confirmation.
```

Agent changes:

```text
- updates the stop form/button
- adds target-specific confirmation text
- updates the related handler/view test
- updates user documentation if visible behavior changed
```

Make the smallest coherent change.

If a broader refactor is genuinely required to make the requested change safely, explain why.

---

## 42. Agent Decision Ladder

When unsure how much structure to add, use this order:

1. **Can a direct function call solve it?**
   Call the function.

2. **Can the function fail?**
   Return and handle an error.

3. **Can it block?**
   Pass a context and bound the call.

4. **Can it partially complete?**
   Record the intermediate state and define outcome handling.

5. **Does it cross a trust boundary?**
   Add authorization, validation, CSRF where applicable, and audit where required.

6. **Is the same real pattern repeated in several places?**
   Consider a small abstraction.

7. **Does the abstraction make authority or behavior less obvious?**
   Do not add it.

This ladder overrides the agent's instinct to generalize early.

### Summary Rules

**Concrete types first.**
Interfaces must earn their place.

**Local behavior first.**
Keep code near the feature that owns it.

**Explicit errors.**
Never discard returned errors.

**Visible outcomes.**
No user action fails silently. Render the result where the action occurred.

**Correlated diagnostics.**
One error reference connects the UI, Error Panel, structured log, and audit when applicable.

**Clear input.**
Labels are visible. Constrained fields state what acceptable input looks like.

**No generic utility packages.**
Prefer `auth/`, `server/`, `container/`, `proxy/`, and `cluster/` over `utils/`, `helpers/`, or `common/`.

**Boring concurrency.**
Bound it, cancel it, own it.

**Explicit privileged operations.**
Prefer `StopContainer()` over `ExecuteAction()`.

**HTML for HTMX.**
Return fragments, not generic JSON wrappers.

**Narrow changes.**
Do not widen the task without a demonstrated need.

**Behavior tests.**
Test what users, operators, and trust boundaries observe.

---

## 43. Final Rule

GoPanel should be boring to maintain.

Readable code, explicit behavior, small operations, shallow control flow, and clear ownership are features.

Do not optimize for cleverness, theoretical reuse, abstraction density, or hypothetical future flexibility.

When choosing between two correct implementations, prefer the one a maintainer can understand by reading the least amount of code.

> **If a normal function call is enough, call the function.**

> **The best GoPanel code should look unsurprising.**
