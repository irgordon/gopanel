# Develop GoPanel

Use this page to run and validate the current pre-Phase-4 application.

Phases 0–3 provide validated startup, file-backed SQLite, local authentication, process-local browser diagnostics, audited server registration, health and readiness probes, structured lifecycle logs, expired-session cleanup, and graceful shutdown. Managed-system integrations have not shipped; Phase 4 begins Docker read-only support.

## Prerequisites

Use the versions pinned by [the build workflow](../../.github/workflows/build.yaml):

- Go `1.27.0`
- Node.js `26`
- Templ CLI `v0.3.1020`
- Tailwind CSS and CLI `4.3.3`

Install the pinned development tools:

```bash
go install github.com/a-h/templ/cmd/templ@v0.3.1020
npm ci
```

Node.js and npm are build dependencies only. The compiled GoPanel binary embeds the generated browser assets and does not require Node.js at runtime.

## Run the Development Server

Generate the checked-in assets, then start GoPanel:

```bash
templ generate
npm run build:css
go run ./cmd/gopanel --dev
```

Development defaults are:

```text
listen address: 127.0.0.1:8080
database path:  ./gopanel.db
public URL:     http://127.0.0.1:8080
```

The database file is created with mode `0600`. Its parent directory must already exist. Development mode may override a default only with an explicit field:

```bash
go run ./cmd/gopanel \
  --dev \
  --listen-address 127.0.0.1:8081 \
  --database-path /tmp/gopanel-dev.db \
  --public-url http://127.0.0.1:8081
```

Production-mode configuration omits `--dev`, requires all three fields, and requires an HTTPS public URL. GoPanel rejects unknown flags, positional values, SQLite connection strings in place of filesystem paths, missing database parents, non-loopback development listeners, and invalid public origins.

Press `Control-C` to initiate bounded graceful shutdown. See [operations](./operations.md) for startup ordering, probe behavior, lifecycle events, and database failure handling.

## Validate a Change

Record the source before validation. For an uncommitted worktree, use a temporary Git index so the digest includes tracked and untracked files without changing the real index:

```bash
git rev-parse HEAD
git status --short --branch
validation_index_dir=$(mktemp -d /tmp/gopanel-validation-index.XXXXXX)
GIT_INDEX_FILE="$validation_index_dir/index" git read-tree HEAD
GIT_INDEX_FILE="$validation_index_dir/index" git add -A
GIT_INDEX_FILE="$validation_index_dir/index" git diff --cached --binary > /tmp/gopanel-validation.diff
shasum -a 256 /tmp/gopanel-validation.diff
```

Run the required local operations:

```bash
go mod tidy
npm ci
templ generate
git diff --exit-code -- '*.go'
npm run build:css
git diff --exit-code -- web/static/output.css
npm test
go clean -testcache
set -o pipefail
go test -count=1 -json ./... | tee /tmp/gopanel-go-test.json
go test -race -count=1 ./...
go vet ./...
go build ./...
cd web/static
shasum -a 256 -c htmx-1.9.12.min.js.sha256
```

Inspect the JSON test log for test-level `skip` actions. Inspect Node TAP output for `# SKIP` and `# TODO`. Package-level Go entries reporting `[no test files]` are not skipped test cases.

Generated Templ files and `web/static/output.css` are committed. Regeneration must leave them unchanged.

## Verify HTMX Provenance

GoPanel vendors the `dist/htmx.min.js` file distributed in the npm package `htmx.org@1.9.12` from `https://registry.npmjs.org`.

Registry metadata:

- Tarball: `https://registry.npmjs.org/htmx.org/-/htmx.org-1.9.12.tgz`
- Package SHA-1: `1c5bc7fb4d3eb4e8c0d72323dc774a6b9b66addc`
- Package integrity: `sha512-VZAohXyF7xPGS52IM8d1T1283y+X4D+Owf3qY1NZ9RuBypyu9l8cGsxUMAG5fEAb/DhT7rDoJ9Hpu5/HxFD3cw==`
- Distributed-file and vendored-file SHA-256: `449317ade7881e949510db614991e195c3a099c4c791c24dacec55f9f4a2a452`

Verify the local asset from its directory:

```bash
cd web/static
shasum -a 256 -c htmx-1.9.12.min.js.sha256
```

## Critical Negative Controls

When a critical test is introduced or materially changed:

1. Record the protected production file hash, `git status`, and complete diff digest.
2. Change only the production behavior to one known-wrong case. Do not change the test.
3. Run the targeted test uncached and record `EXPECTED FAILURE OBSERVED` only when it fails for the intended reason.
4. Apply the exact inverse change and confirm the original file hash and complete diff digest.
5. Rerun the unmodified test and record the successful result.

Current critical controls include migration failure, configuration rejection, SQLite-backed readiness, graceful HTTP draining, authentication and CSRF boundaries, diagnostic redaction, the 200-entry diagnostic bound, error-reference correlation, server-registration audit transitions, and lifecycle-owned session cleanup. Negative-control mutations are temporary and must never be committed.

## Evidence States

- `PASS`: the check executed, observed the requirement, and produced inspectable evidence.
- `FAIL`: the check executed and did not observe the requirement, or a required command failed.
- `NOT RUN`: the check did not execute.
- `INCONCLUSIVE`: the check executed but did not prove the requirement.

`LOCAL PASS` means required local checks and negative controls passed for the recorded baseline-plus-diff source. `CI PASS` requires the protected workflow to pass against the exact commit. A phase is complete only when all required local and exact-commit CI evidence exists.

## Current Owner Sequencing Override

[ADR 0002](../ADR/0002-pre-phase4-javascript-disabled-browser-override.md) records a narrow owner override for the current pre-Phase-4 repair. Authenticated desktop and mobile browser checks passed, as did HTMX navigation and URL behavior. Literal JavaScript-disabled browser verification remains `NOT RUN` because the available in-app browser cannot disable JavaScript, the owner declined Codex Computer Use authorization, and no alternate literal browser environment was used.

The JavaScript-independent real-HTML HTTP workflow is separate supporting evidence and remains `PASS`; it is not a browser test. The override permits sequencing only after every other required check and exact-commit CI passes. It does not weaken GP-023 or GP-031, satisfy the unresolved browser criterion, or create a standing exemption for later browser verification.

Those remaining checks and exact-commit CI passed, so the pre-Phase-4 repair is closed by owner sequencing override and Phase 4 is authorized to begin. The deferred literal browser result remains `NOT RUN` and remains mandatory later when a suitable environment is available.

## Current Phase Boundary

- SQLite contains migration metadata plus `users`, `sessions`, `servers`, and `audit_log`.
- Diagnostic records are process-local, capped at 200, and exposed only through administrator-authorized browser routes.
- Local authentication, two roles, session-bound CSRF, and audited server registration are present.
- Docker, Caddy, Vault, and Kubernetes clients and operations are not present.
- Readiness depends on GoPanel and SQLite only.

See [Architecture](../ARCHITECTURE.md) and [Invariants](../INVARIANTS.md) for the boundaries later phases must preserve.
