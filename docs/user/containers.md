# View Docker Containers

GoPanel provides read-only visibility into the Docker daemon configured by the application administrator. Docker owns container state. GoPanel reads the current state when requested and does not store container rows or health results in SQLite.

## Check Docker Status

1. Sign in with an administrator account.
2. Open **Servers** and select a server whose connection type is Docker.
3. Read the Docker status and its **Checked** time.
4. Select **Test Docker connection** to ask Docker directly for a health response.

`Docker connected` means the configured daemon answered the most recent Docker API check. It does not mean every container is healthy. `Docker unavailable` means the latest check failed. The displayed time tells you how fresh the process-local observation is; observations disappear when GoPanel restarts.

## View Containers

Select **View containers** from the Docker server detail. The page lists the current Docker container name, image, state, and Docker status. An empty list means Docker responded successfully but returned no containers. It is different from a Docker error.

GoPanel does not provide start, stop, restart, remove, exec, image, volume, network, or Compose actions in Phase 4.

## View Bounded Logs

Administrators can select **View logs** or **View bounded logs** for a container. GoPanel requests only the last 100 lines and enforces an additional byte limit. It does not stream, persist, diagnose, or audit log content.

Container logs are untrusted, potentially sensitive application output. They may contain tokens, URLs, credentials, or other private values. For that reason, viewer accounts cannot retrieve them.

## When Docker Is Unavailable

Try the Docker connection test again. If the failure continues, use **See Error Log** and give the displayed error reference to an administrator. The reference connects the visible failure to safely mapped current-process diagnostics; raw Docker errors and log content are not placed there.
