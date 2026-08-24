# GoPanel — Documentation Guide

## 1. Purpose

GoPanel documentation has two audiences:

1. **Users** need to know what the app does, how to complete a task, what happened after an action, and what to do when something fails.
2. **Maintainers** need to know how GoPanel is organized, why important design choices exist, where a change belongs, and what must remain true when the code changes.

Documentation should reduce friction.

> Explain the concrete task first. Add technical detail only when it helps someone use, troubleshoot, or safely change GoPanel.

`docs/ARCHITECTURE.md` is the technical source of truth. `docs/INVARIANTS.md` is its testable constraint register. `docs/ROADMAP.md` describes the intended path to the current release. This file defines how documentation is written and organized.

---

## 2. Documentation Rules

1. **Plain language first.** Prefer common words and short sentences.
2. **Task before theory.** Start with what the reader is trying to do.
3. **Explain why it matters.** State the problem the feature solves.
4. **One page, one purpose.** Do not bury a simple task in a long reference.
5. **Use GoPanel's own words.** Documentation should match UI labels, statuses, and errors.
6. **Link instead of copying.** Do not duplicate large sections of Architecture or Roadmap.
7. **Keep current behavior documented.** Git records history; documentation explains the system as it works now.
8. **Be precise without being opaque.** Technical detail is useful only when the reader needs it.

---

## 3. User Documentation

### Audience

User documentation is written at approximately a 12th-grade reading level.

The user may know how to operate servers or containers, but they should not need to understand Go, HTMX, SQLite, request contexts, SDK internals, or GoPanel's package layout to use the application.

### Every user page should answer

- What does this do?
- Why would I use it?
- What do I need before I start?
- What steps do I take?
- What should happen when it works?
- What does a warning or error mean?
- What can I safely try next?

### Example

Good:

> **Stop a container**
> Use Stop when you want to shut down a running container. GoPanel asks you to confirm the container name before sending the request.

Avoid:

> The privileged mutation handler issues a bounded Docker SDK request after CSRF and authorization middleware complete.

The second explanation belongs in maintainer documentation.

### User writing rules

- Use the same terms shown in the UI.
- Define necessary technical terms in one sentence.
- Keep procedures short.
- Put warnings immediately before risky actions.
- Give every field and control a visible plain-language label or accessible name.
- When input is not obvious, state the expected format, unit, range, or allowed choice in one short line near the field.
- Use examples and placeholder text only as supplements; never use them instead of a label.
- Explain blocked, denied, invalid, and failed actions in plain language and state the safe next step.
- Explain what an error reference means and when an administrator should use `See Error Log`.
- Do not expose internal errors, credentials, tokens, or implementation detail.
- Do not claim application output is free of secrets. Container logs, for example, may contain sensitive application data.

---

## 4. Maintainer Documentation

Maintainer documentation explains enough of the design to make a safe change without rediscovering the project.

It should answer:

- What owns this behavior?
- Where does the code live?
- What system owns the underlying state?
- What security boundary protects this operation?
- Why was this approach chosen?
- What should not be generalized?
- What tests prove the change works?
- Does this change affect user documentation?

Technical language is allowed, but clarity still comes first.

Good:

> Docker owns container state. GoPanel reads that state when needed and does not store a second durable copy in SQLite. This prevents the UI database from becoming stale.

Avoid:

> The control plane implements a non-authoritative state projection across heterogeneous infrastructure domains.

### Maintainer writing rules

- Start with the concrete component or operation.
- State who owns the data or behavior.
- Explain non-obvious decisions in plain language.
- Link to Architecture for deeper rules.
- Use small code or route examples only when they clarify a contract.
- Do not document speculative abstractions as if they already exist.
- If implementation deviates from the roadmap and creates a lasting design choice, document the resulting behavior.
- Explain how error references connect the user-visible message, Error Panel entry, audit record where applicable, and structured backend log.
- Document redaction rules beside diagnostic fields so maintainers do not treat administrator-only access as permission to expose unfiltered errors.

---

## 5. Repository Documentation Layout

The root stays small. `/docs/` acts as the repo wiki.

```text
/
├── README.md
├── AGENTS.md
└── docs/
    ├── ARCHITECTURE.md
    ├── INVARIANTS.md
    ├── ROADMAP.md
    ├── DOCUMENTATION.md
    ├── CODING_STYLE.md
    ├── user/
    │   ├── getting-started.md
    │   ├── servers.md
    │   ├── containers.md
    │   ├── proxies.md
    │   ├── kubernetes.md
    │   └── troubleshooting.md
    └── maintainers/
        ├── development.md
        ├── project-layout.md
        ├── authentication.md
        ├── integrations.md
        ├── ui-patterns.md
        ├── error-handling.md
        └── operations.md
```

Only create a page when the capability exists. Do not create empty placeholder documentation.

### Root files

**`README.md`** — What GoPanel is, the problem it solves, current status, and links into `/docs/`.

**`AGENTS.md`** — Short routing instructions that tell coding agents which governance documents to read and how precedence works.

### Governance files under `/docs/`

**`ARCHITECTURE.md`** — Authoritative design, ownership boundaries, security rules, and major technical decisions.

**`INVARIANTS.md`** — Testable conditions that every applicable implementation must preserve. It does not replace Architecture.

**`ROADMAP.md`** — Intended implementation path. Normal development may require documented deviations.

**`DOCUMENTATION.md`** — Documentation audiences, structure, style, and maintenance rules.

**`CODING_STYLE.md`** — Implementation and agent coding rules, including simple-over-clever and Good vs. Bad patterns.

The root `README.md` is the documentation entry point and links directly to the governance files. Do not create a second `docs/README.md` index.

---

## 6. User Wiki

User pages are organized around tasks, not internal modules.

**`getting-started.md`**
What GoPanel manages, sign-in, basic screen layout, server registration, and where status/errors appear.

**`servers.md`**
Add, edit, remove, and understand registered servers and connection status.

**`containers.md`**
View containers, understand status, view bounded logs, and use supported start/stop actions.

**`proxies.md`**
Create when Caddy support ships. Document only the route actions GoPanel actually exposes.

**`kubernetes.md`**
Create when Kubernetes support ships. Document only the resources GoPanel actually supports.

**`troubleshooting.md`**
Organize by visible problem, such as:

- Cannot sign in
- Server is unavailable
- Docker did not respond
- An action failed
- An action was blocked or denied
- A field will not accept the entered value
- An error reference says to contact an administrator
- Status looks old
- GoPanel will not start

Start with safe user actions. Put maintainer-only diagnostics elsewhere.

---

## 7. Maintainer Wiki

**`development.md`**
Required tools, build/test commands, development mode, generated assets, and local configuration.

**`project-layout.md`**
Package ownership and allowed dependency direction.

```text
handler -> service -> store or typed infrastructure client
view -> presentation models
browser -X-> infrastructure APIs
```

**`authentication.md`**
Password hashing, opaque sessions, authorization, rate limiting, logout, password changes, and CSRF.

The Phase 2 authentication page links its operational rules back to Architecture §9 and GP-007–GP-010 and identifies the behavioral and negative-control tests that prove password, session, limiter, cookie, and CSRF boundaries. Tests do not replace the successful specification.

**`integrations.md`**
Shared rules for Docker, Caddy, Vault if used, and Kubernetes:

- use typed clients;
- expose explicit operations only;
- each integration validates its own connection configuration;
- external systems own their operational state;
- calls are bounded and cancelable;
- writes are not blindly retried;
- Vault-resolved secret values are never rendered.

**`ui-patterns.md`**
Real HTML first, HTMX enhancement, full-page/fragment consistency, meaningful URLs, visible field labels, concise input guidance, Loading/Empty/Loaded/Error states, blocked-action explanations, persistent errors, success toasts, and mobile-first behavior.

**`error-handling.md`**
User-visible error categories, error-reference creation, administrator-only Error Panel behavior, diagnostic fields, correlation, process-local retention, redaction, and the boundary between safe technical detail and structured backend logs.

**`operations.md`**
Startup/shutdown, `/healthz`, `/readyz`, SQLite location and permissions, backup/restore, configuration validation, Error Panel reset-on-restart behavior, and structured logs.

Do not repeat the Architecture document. Explain how its rules appear in the current code.

---

## 8. Keep Technical Detail Useful

Use technical detail when it protects a boundary or helps someone make a safe change.

Good:

> `StopContainer` is an explicit operation. Do not replace it with a generic Docker request function because that would expand what GoPanel is allowed to ask Docker to do.

Unhelpful:

> A long explanation of Docker internals or control-plane theory that does not change how GoPanel is used or maintained.

Before adding detail, ask:

> Does the reader need this to use the feature, troubleshoot it, or change it safely?

If not, remove it.

---

## 9. When Documentation Changes

Update **user documentation** when:

- a visible workflow changes;
- a field, button, status, warning, or error changes;
- a new user-facing capability ships;
- prerequisites or troubleshooting steps change.

Update **maintainer documentation** when:

- code ownership moves;
- a security boundary changes;
- an integration or mutation pattern changes;
- configuration or operational behavior changes;
- a roadmap deviation becomes a lasting implementation decision.

A documentation update is usually unnecessary when code is refactored without changing behavior, ownership, or supported contracts.

---

## 10. Commit Messages

Keep commits small and easy to scan.

Recommended form:

```text
<area>: <short action>
```

Examples:

```text
auth: add session cleanup
server: add registration form
docker: add bounded log view
audit: record mutation attempts
ui: add mobile container card
docs: clarify connection errors
```

Rules:

- One clear purpose per commit.
- Keep the subject short.
- Use an action, not a paragraph.
- Separate unrelated changes.
- Update small documentation changes in the same commit as the behavior they describe when practical.
- Use a separate `docs:` commit when documentation is the main change.

Git shows **what changed**. Documentation explains **how the current system works and why important boundaries exist**.

---

## 11. Documentation Definition of Done

Before merging a user-facing feature, check:

### User side

- Can the user tell what the feature does?
- Can they complete the normal task without knowing the implementation?
- Does every field and control have a clear label or accessible name?
- When input is constrained, does the UI state the expected format, unit, range, or allowed choice?
- Are risky actions explained before they happen?
- Are blocked, denied, invalid, and failed actions visible and understandable?
- Does every user-visible backend failure include a usable error reference and safe next step?
- Does the documentation match the actual UI?

### Maintainer side

- Is ownership clear?
- Are important security/design boundaries documented?
- Is the reason for a non-obvious decision recorded?
- Are Error Panel access, retention, correlation, and redaction rules documented?
- Are build, test, or operational changes documented?
- Would a new maintainer know where to continue the work?

If the answer is yes, stop writing. More documentation is not automatically better documentation.

---

## 12. Validation Reports

Every implementation-phase validation report includes:

- baseline commit and, for an uncommitted worktree, a SHA-256 digest of the complete binary diff;
- exact commands, exit outcomes, retries, approvals, and material output;
- generated-file and dependency state;
- critical negative-control mutations, expected failures, restoration hashes, and final checks;
- literal browser evidence where required;
- deviations, errors, skipped checks, and inconclusive checks;
- separate local and independent CI status; and
- final phase status.

Use these result states exactly:

- `PASS`: the check executed, observed the required behavior, and produced inspectable evidence.
- `FAIL`: the check executed and did not observe the requirement, or a required command failed.
- `NOT RUN`: the check did not execute, including missing tools, denied access, and skipped work.
- `INCONCLUSIVE`: the check executed but did not prove the requirement.

Do not convert `NOT RUN` or `INCONCLUSIVE` into `PASS` through explanation. Local workflow parity is reported separately from independent CI. A phase report does not claim completion until the exact implementation commit has inspectable CI evidence and no required check remains unresolved.

## 13. Final Rule

GoPanel should be understandable before it is impressive.

For users, documentation should make the application easier to operate.

For maintainers, documentation should make the next safe change easier to make.

If documentation makes either audience work harder to understand the product, simplify it.
