# ADR 0002: Pre-Phase-4 JavaScript-Disabled Browser Override

- Status: Accepted
- Date: 2026-08-24
- Decision maker: Project owner

## Context

The pre-Phase-4 repair requires literal browser verification with JavaScript disabled under GP-023 and GP-031. The available in-app browser does not expose a JavaScript-disable capability, the owner declined authorization for Codex Computer Use, and no alternate literal browser environment was used.

The implementation has separate supporting evidence from a JavaScript-independent real-HTML HTTP workflow. That workflow exercises the server-rendered HTML and ordinary form/navigation path, but it does not prove literal browser rendering or interaction with JavaScript disabled.

## Decision

Literal JavaScript-disabled browser verification remains:

`NOT RUN`

The owner accepts this unresolved evidence item for sequencing only. This decision does not reclassify the result, claim that literal browser behavior was observed, or satisfy the affected browser exit criterion.

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

The HTTP workflow proves the server-side HTML/form path. It does not prove literal browser rendering or interaction with JavaScript disabled.

## Residual Risk

A browser-specific dependency on JavaScript could still exist despite the passing direct HTTP form and navigation checks. Literal browser evidence has not ruled out defects involving:

- browser navigation behavior;
- cookie and redirect interaction;
- form submission behavior;
- browser rendering;
- visible placement of errors;
- browser-native interaction differences.

This decision does not claim that any of those defects currently exist.

## Controls Retained

- GP-023 remains unchanged.
- GP-031 remains unchanged.
- JavaScript-independent server behavior remains covered by automated and HTTP-level tests.
- Phase 9 retains representative JavaScript-disabled browser verification.
- Future availability of an appropriate browser environment should trigger the deferred literal check.
- Any failure discovered later is a real defect and is not dismissed because sequencing was previously overridden.
- Independent CI against the exact committed source remains mandatory before Phase 4 may begin.

## Scope

This override applies only to the unresolved literal JavaScript-disabled browser evidence for the current pre-Phase-4 repair. It does not create a standing exemption for future browser checks and does not weaken progressive enhancement or the real-HTML-first architecture.

## Sequencing Outcome

All other required local checks and the protected workflow against the exact repair commit passed without skipped required checks or source-correlation drift. The owner sequencing override therefore applies to the retained `NOT RUN` result.

`PRE-PHASE-4 REPAIR: CLOSED BY OWNER SEQUENCING OVERRIDE`

`PHASE 4: AUTHORIZED TO BEGIN`

Deferred evidence: literal JavaScript-disabled browser verification remains `NOT RUN`. It must be executed when a suitable browser environment becomes available and remains part of release verification. This closure permits sequencing; it does not mean the JavaScript-disabled exit criterion was satisfied.
