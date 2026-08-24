# AGENTS.md

Read before every change:

1. `docs/ARCHITECTURE.md`
2. `docs/INVARIANTS.md`
3. `docs/CODING_STYLE.md`
4. `docs/ROADMAP.md`
5. `docs/DOCUMENTATION.md`

`ARCHITECTURE.md` is authoritative. `INVARIANTS.md` is its testable constraint register. If they conflict, stop and reconcile them before implementation. The roadmap may change sequence or scope; it may not waive an invariant.

Rules:

- Make the smallest coherent change.
- Preserve explicit, typed infrastructure operations and module-owned trust boundaries.
- Do not add generic request, credential, mutation, or plugin frameworks.
- Enforce authentication, authorization, CSRF, and audit behavior on the server.
- Never allow a user action to fail silently.
- Keep field labels, expected input, blocked-action reasons, and recovery steps clear and concise.
- Correlate backend failures across every applicable surface: the UI, administrator-only Error Panel, structured log, and audit.
- Never expose unfiltered errors, credentials, tokens, secret values, authorization headers, raw request bodies, or untrusted workload output.
- Add or update behavioral tests for every affected invariant.
- Run required Go tests uncached and correlate results to the exact commit or complete dirty-worktree diff.
- Run required critical-path negative controls without changing the test under evaluation.
- Report each check as `PASS`, `FAIL`, `NOT RUN`, or `INCONCLUSIVE`.
- Distinguish local evidence from independent CI evidence.
- Never call a phase complete without inspectable evidence from the exact committed source.
- Update user or maintainer documentation when visible behavior, ownership, security, configuration, or operations change.
