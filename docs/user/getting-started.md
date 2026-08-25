# Get Started with GoPanel

GoPanel currently supports local sign-in, server registration, administrator diagnostics, and administrator-only Docker read visibility. Docker mutations and other managed-system integrations are not available yet.

## Before You Start

An administrator must create the first account locally on the GoPanel host:

```bash
gopanel user create-admin --database-path /path/to/gopanel.db
```

The command requires an interactive terminal and prompts for an email, name, and hidden password.

## Sign In

1. Open `/login`.
2. Enter the email and password created by an administrator.
3. Select **Sign in**.

If the form expired, reload the page and try again. Repeated failed attempts are temporarily limited.

## Available Areas

- **Servers** is currently administrator-only and shows registered server identity and connection type. Registration does not contact the remote server.
- Docker server details show process-local connection status, container navigation, a connection test, and bounded administrator-only logs.
- **Change password** updates your password and invalidates all existing sessions for your account.
- **Error Log** is administrator-only and contains safely mapped diagnostics from the current GoPanel process.

Diagnostic entries disappear when GoPanel restarts. Use the error reference to correlate a visible failure with the Error Log and structured backend logs.
