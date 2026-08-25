# ADR 0003: Phase 4 JavaScript-Disabled Browser Evidence Deferral

- Status: Accepted
- Date: 2026-08-25
- Decision maker: Project owner

## Context

Phase 4 requires literal browser verification of its real-HTML and progressive-enhancement behavior under GP-023 and source-correlated evidence under GP-031. The available in-app browser does not expose a JavaScript-disable capability, the owner declines authorization for Codex Computer Use, and no alternate literal-browser environment was available for the Phase 4 validation run.

[ADR 0002](./0002-pre-phase4-javascript-disabled-browser-override.md) applies only to the pre-Phase-4 repair. Its scope does not cover Phase 4 closure, so this separate decision is required.

## Decision

Literal JavaScript-disabled browser verification remains:

`NOT RUN`

The owner accepts this unresolved evidence item for Phase 4 sequencing and closure only if every other applicable local check passes, exact source identity is established, the exact committed source passes the protected workflow, and no required CI check is skipped or weakened.

This decision does not itself close Phase 4. Before exact-commit CI passes, Phase 4 remains locally validated and awaiting commit and CI.

## Evidence Already Available

### Literal browser evidence

| Check | Result |
| :--- | :--- |
| Authenticated desktop browser | `PASS` |
| Authenticated mobile browser | `PASS` |
| HTMX navigation and URL behavior | `PASS` |
| Literal JavaScript-disabled browser | `NOT RUN` |

### Supporting non-browser evidence

| Check | Result |
| :--- | :--- |
| JavaScript-independent real-HTML HTTP workflow | `PASS` |

The ordinary HTTP workflow proves the server-side link, form, authorization, CSRF, navigation, container-read, and bounded-log paths without HTMX headers. It does not prove literal browser rendering or interaction with JavaScript disabled.

## Residual Risk

Literal browser evidence has not independently ruled out browser-specific problems involving:

- navigation;
- redirect handling;
- cookie behavior;
- native form submission;
- browser rendering;
- visible error placement;
- browser-native control behavior;
- JavaScript-dependent presentation assumptions.

This decision does not claim that any of those defects currently exist.

## Controls Retained

- The result remains `NOT RUN`; it is not converted to `PASS`.
- GP-023 remains unchanged.
- GP-031 remains unchanged.
- Progressive enhancement and real-HTML-first behavior remain required.
- This decision does not claim browser-native no-JavaScript behavior was observed.
- This decision does not authorize Codex Computer Use or another interactive computer-control mechanism.
- The passing HTTP workflow remains supporting non-browser evidence only.
- Phase 9 retains representative literal JavaScript-disabled browser verification.
- Future availability of a suitable literal-browser environment should trigger the deferred check.
- A later failure is treated as a real defect, not dismissed because Phase 4 sequencing was deferred.

## Scope

This deferral applies only to the unresolved literal JavaScript-disabled browser evidence for Phase 4 Docker Read-Only. It does not create a general exemption for future phase or release browser verification.
