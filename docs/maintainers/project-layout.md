# Understand the Project Layout

GoPanel uses three implementation levels:

```text
HTTP handler → domain service → typed store or client
```

- `cmd/gopanel` constructs concrete dependencies and starts the application.
- `internal/app` owns HTTP serving, recurring work, and bounded shutdown.
- `internal/auth` owns identity and browser request security.
- `internal/server` owns server-registration validation and audit sequencing.
- `internal/audit` owns attempted-to-result SQLite transitions.
- `internal/diagnostic` owns the bounded process-local diagnostic buffer and Error Log.
- `internal/store` owns SQLite opening and migrations.
- `internal/view` receives prepared presentation models only.

Handlers do not access `database/sql`. Services contain domain decisions and call narrow typed stores. Only low-level store implementations execute SQL. Avoid generic request, credential, mutation, repository, or plugin frameworks.
