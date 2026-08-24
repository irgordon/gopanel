# GoPanel

GoPanel is a privileged web control panel for explicit Docker, Caddy, Vault, and Kubernetes operations. It gives developers and operators a clear interface for common infrastructure tasks without turning the browser into a generic administration client.

Managed systems remain authoritative for their own state. GoPanel owns authentication, validated connection configuration, audit history, bounded operator diagnostics, and presentation.

## Current Status

Phase 1 is implemented locally. The repository contains a runnable Go application with validated startup configuration, a file-backed SQLite database, embedded forward-only migrations, structured lifecycle logging, process-local diagnostic correlation, health and readiness probes, and bounded graceful shutdown.

Run it locally:

```bash
npm ci
templ generate
npm run build:css
go run ./cmd/gopanel --dev
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). See the [development guide](./docs/maintainers/development.md) for pinned prerequisites and validation commands.

Development mode creates `gopanel.db` in the current directory. Process liveness is available at `/healthz`; SQLite-backed readiness is available at `/readyz`.

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

Maintainer guidance includes [development](./docs/maintainers/development.md) and [operations](./docs/maintainers/operations.md). Additional guides will be added only when their related capabilities exist.
