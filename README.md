# GoPanel

GoPanel is a privileged web control panel for explicit Docker, Caddy, Vault, and Kubernetes operations. It gives developers and operators a clear interface for common infrastructure tasks without turning the browser into a generic administration client.

Managed systems remain authoritative for their own state. GoPanel owns authentication, validated connection configuration, audit history, bounded operator diagnostics, and presentation.

## Current Status

GoPanel is in a pre-Phase-4 state. Phases 0, 1, 2, 2A, and 3 provide the application scaffold, lifecycle and SQLite foundation, local authentication, the administrator-only Error Log, and audited server registration. Docker connectivity and container views begin in Phase 4 and are not implemented yet.

The pre-Phase-4 repair is closed by the narrowly scoped [owner sequencing override](./docs/ADR/0002-pre-phase4-javascript-disabled-browser-override.md), and Phase 4 is authorized to begin. Literal JavaScript-disabled browser verification remains `NOT RUN`; the override permits sequencing but does not satisfy or remove that requirement from later release verification.

Run it locally:

```bash
npm ci
templ generate
npm run build:css
go run ./cmd/gopanel user create-admin --database-path ./gopanel.db
go run ./cmd/gopanel --dev
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). See the [development guide](./docs/maintainers/development.md) for pinned prerequisites and validation commands.

The administrator command prompts locally for email, name, and a hidden password. Development mode creates `gopanel.db` in the current directory. Sign in at `/login`; registered servers are available at `/servers`; administrators can inspect current-process diagnostics at `/errors`. Process liveness is available at `/healthz`, and SQLite-backed readiness is available at `/readyz`.

The planned v1 baseline is:

- Go 1.27.0;
- one GoPanel process and one SQLite database;
- local email/password authentication with `admin` and `viewer` roles;
- server-rendered HTML enhanced by HTMX;
- explicit, typed infrastructure operations;
- server-side authorization, CSRF protection, and auditable privileged operations;
- visible, plain-language failure handling with administrator-only diagnostic details; and
- deliberate mobile and desktop behavior in every feature slice.

## Safety Boundaries

- The browser never talks directly to privileged infrastructure APIs.
- GoPanel does not persist live infrastructure state as durable truth.
- User input cannot become an arbitrary URL, socket, file path, environment-variable lookup, or kubeconfig context.
- Reusable secret values never enter HTML, audit records, logs, or the Error Panel.
- User actions never fail silently. Blocked, denied, invalid, and failed actions explain what happened and what can safely happen next.
- Unfiltered backend errors never appear in the browser. Administrators receive correlated, redacted technical diagnostics through the Error Panel.

## Project Governance

- [Architecture](./docs/ARCHITECTURE.md) — design, ownership, runtime, and security boundaries
- [Invariants](./docs/INVARIANTS.md) — conditions every applicable implementation must preserve
- [Roadmap](./docs/ROADMAP.md) — intended path to GoPanel v1.0.0
- [Coding Style](./docs/CODING_STYLE.md) — implementation rules and agent coding patterns
- [Documentation Guide](./docs/DOCUMENTATION.md) — user and maintainer documentation rules

Coding agents must also follow [AGENTS.md](./AGENTS.md).

## Documentation

User guidance includes [getting started](./docs/user/getting-started.md) and [server registration](./docs/user/servers.md).

Maintainer guidance includes [development](./docs/maintainers/development.md), [operations](./docs/maintainers/operations.md), [authentication](./docs/maintainers/authentication.md), [error handling](./docs/maintainers/error-handling.md), and [project layout](./docs/maintainers/project-layout.md).
