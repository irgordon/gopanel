# Maintain Infrastructure Integrations

Each integration owns a small set of typed operations, validates its own connection configuration, and maps its own technical failures into bounded safe detail. Do not introduce generic requests, connection tests, credential resolvers, pollers, or resource browsers.

## Docker Read-Only

`internal/container` owns the Phase 4 Docker boundary:

```text
handler → container.Service → container.Client → official Moby client
```

The only external capabilities exposed by the narrow client boundary are Docker ping, container listing, and bounded log retrieval. The SDK client may implement many other Docker operations, but GoPanel does not expose them through its Phase 4 wrapper.

The pinned direct modules are `github.com/moby/moby/client` v0.5.1 and `github.com/moby/moby/api` v1.55.0. They are the official typed Docker Engine client/API used by current Docker documentation. No secondary Docker wrapper or frontend dependency is present.

Docker configuration comes from `--docker-socket`. `ValidateConfig` accepts only `/var/run/docker.sock` and `/run/docker.sock`. It rejects URLs, TCP endpoints, environment-derived hosts, arbitrary filesystem paths, and database values before SDK construction. Registered server address is identity metadata and is never supplied to `client.WithHost`.

Docker remains authoritative for container state. GoPanel has no container or Docker-status table. `StatusCache` is synchronized process memory, starts empty, and shows `CheckedAt` when an observation exists.

`internal/app` owns the 30-second poller. It starts a fixed six-worker pool, schedules finite `CheckStatus` calls, stops scheduling on lifecycle cancellation, reuses a failure reference while the same unavailable state continues, and stops before the Docker client and SQLite close. `/readyz` never reads Docker status.

Every Docker SDK call inherits the caller context and receives the single five-second Docker timeout. Log retrieval requests exactly the last 100 lines, disables follow mode, closes the response stream, and caps decoded output at 1 MiB.

Container logs are administrator-only untrusted workload output. They may be displayed as escaped text to an authorized administrator, but they are never placed in diagnostics, structured logs, audit rows, or SQLite.

Docker reads do not create audit rows. Connection-test authorization and CSRF rejections use the existing security logging contract; backend failures receive one HTTP-boundary or background-boundary diagnostic reference.
