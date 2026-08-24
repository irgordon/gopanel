# Maintain Local Authentication

The `internal/auth` package owns local users, password hashing, sessions, login limiting, browser mutation protection, and password changes.

Passwords use Argon2id. Raw session credentials contain 256 random bits; SQLite stores only their SHA-256 hashes. Production cookies use host-prefixed names with `Secure`, `HttpOnly`, `Path=/`, and `SameSite=Lax`. Development uses separate non-Secure cookies and is restricted to loopback.

Every browser POST follows this order:

```text
bound body
→ configured-origin check
→ parse r.PostForm
→ authenticate and authorize where required
→ validate session-bound or login-context CSRF token
→ execute handler
```

Tokens are read only from `r.PostForm`. Query, cookie, referrer, and generic-header tokens are rejected. Expected authentication, role, origin, and CSRF denials create safe security events rather than audit rows. Authentication storage failures create one diagnostic reference and return a generic `500` response.

`Application.Run` owns periodic expired-session cleanup and joins it before closing SQLite. Password changes delete every session for the user.

Relevant invariants: GP-007–GP-011 and V1-002–V1-003.
