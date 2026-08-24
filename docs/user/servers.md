# Register Servers

Server registration records identity and connection type. It does not test connectivity and does not configure credentials before the owning integration exists.

## Add a Server

1. Sign in with an administrator account.
2. Open **Servers** and select **Add server**.
3. Enter a name between 3 and 64 characters.
4. Enter a hostname or IP address without a scheme or path.
5. Choose Docker, Caddy, Vault, or Kubernetes as the connection type.
6. Select **Add server**.

The target system may be offline. A successful registration means only that GoPanel stored validated configuration and finalized its audit record.

If GoPanel says the server was created but its audit record is incomplete, do not submit the form again. Open the created server and give the displayed error reference to an administrator.
