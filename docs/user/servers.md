# Register Servers

Server registration records identity and connection type. Saving the form does not test connectivity. Docker connectivity is tested separately from a Docker server detail after registration.

## Add a Server

1. Sign in with an administrator account.
2. Open **Servers** and select **Add server**.
3. Enter a name between 3 and 64 characters.
4. Enter a hostname or IP address without a scheme or path.
5. Choose Docker, Caddy, Vault, or Kubernetes as the connection type.
6. Select **Add server**.

The target system may be offline. A successful registration means only that GoPanel stored validated configuration and finalized its audit record.

For Docker servers, the registered address remains identity metadata. GoPanel connects only through the local Unix socket selected by application configuration; it never turns the address into a Docker URL or socket. Open the created server to see Docker status, test the connection, and view containers.

If GoPanel says the server was created but its audit record is incomplete, do not submit the form again. Open the created server and give the displayed error reference to an administrator.
