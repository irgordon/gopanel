# ADR 0001 — Phase 0.75 Browser-Evidence Owner Override

- **Status:** Accepted
- **Date:** 2026-08-23
- **Decision maker:** Ian Gordon, project owner

## Context

Phase 0.75 required literal desktop, mobile, and JavaScript-disabled browser validation. The available browser tooling completed the desktop check but could not prove the requested mobile CSS viewport or disable JavaScript. Manual screenshots provide credible visual evidence, but they do not prove the missing measurements or JavaScript state.

This decision applies only to the following source identity:

- `HEAD`: `75fb44cc88bbf2bd23e89c607b1afd5150b80b6a`
- `origin/main`: `75fb44cc88bbf2bd23e89c607b1afd5150b80b6a`
- Complete uncommitted diff SHA-256: `1adb4820cafee58aef5ac25555020a0b75a1259df3a435316f5de3335b41c3c4`

## Decision

The project owner accepts the residual uncertainty in the incomplete browser checks and authorizes development to advance toward Phase 1. Phase 0.75 is:

`CLOSED BY OWNER OVERRIDE — RESIDUAL BROWSER EVIDENCE ACCEPTED`

The evidence states remain unchanged:

| Check | Result |
| --- | --- |
| Desktop visual rendering | `PASS` |
| Mobile visual rendering | `PASS` |
| Exact `375 × 812` CSS viewport measurement | `INCONCLUSIVE` |
| `document.documentElement.scrollWidth <= 375` measurement | `INCONCLUSIVE` |
| Literal JavaScript-disabled reload | `NOT RUN` |
| Confirmation that JavaScript was re-enabled | `NOT RUN` |
| Phase 0 operational workflows | `NOT APPLICABLE` |

## Evidence retained

| Evidence | Dimensions | SHA-256 |
| --- | --- | --- |
| `Screenshot 2026-08-23 at 13.51.35.png` (desktop) | 2264 × 1528 | `f2c411e6696a966566353ce94e2f7c3f53a58f35b068e17d8a8051b49f0a7731` |
| `Screenshot 2026-08-23 at 13.51.47.png` (mobile) | 970 × 2134 | `28f80fd90642fe68c1d590393b17f782631262b7c1bd2c67c52ade46af3ae910` |
| `Screenshot 2026-08-23 at 13.53.43.png` (performance) | 1946 × 1324 | `9fd76dfa96b5a313d4cb9db3b35ebd3159f6aca7815d855aadf2a2b2f14498e6` |

The performance screenshot records local measurements only; it is not a production benchmark.

## Scope and consequences

This override:

- does not reclassify `INCONCLUSIVE` or `NOT RUN` checks as `PASS`;
- applies only to the browser checks and source identity recorded above;
- does not weaken GP-031 or future test-and-evidence requirements;
- does not authorize fabricated, skipped, cached, bypassed, or weakened checks;
- does not waive exact-commit independent CI validation;
- does not automatically waive a Phase 1 acceptance gate; and
- does not itself authorize a commit, push, or Phase 1 implementation.

The owner accepts the limited residual browser-validation risk because it resulted from browser-tool capability limitations and the retained screenshots credibly show correct desktop and mobile visual rendering.
