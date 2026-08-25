# Understand the Project Layout

GoPanel uses three implementation levels:

```text
HTTP handler → domain service → typed store or client
```

- `cmd/gopanel` constructs concrete dependencies and starts the application.
- `internal/app` owns HTTP serving, recurring work, and bounded shutdown.
- `internal/auth` owns identity and browser request security.
- `internal/server` owns server-registration validation and audit sequencing.
- `internal/container` owns Docker socket validation, the typed SDK boundary, explicit read operations, process-local status storage, safe Docker diagnostics, and Docker HTTP presentation.
- `internal/audit` owns attempted-to-result SQLite transitions.
- `internal/diagnostic` owns the bounded process-local diagnostic buffer and Error Log.
- `internal/store` owns SQLite opening and migrations.
- `internal/view` receives prepared presentation models only.

Handlers do not access `database/sql`. Services contain domain decisions and call narrow typed stores or clients. `cmd/gopanel` supplies small Docker-specific adapters from the existing server store into the container service without making either service call the other. Only low-level store implementations execute SQL. Avoid generic request, credential, mutation, repository, integration, or plugin frameworks.
