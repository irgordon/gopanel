# Maintain GoPanel UI Patterns

GoPanel renders real pages and forms first. HTMX enhances the same resource URLs and swaps HTML; it does not create parallel API, fragment, or client-state routes.

Phase 4 uses these concrete patterns:

- `/servers/{id}` shows server identity plus Docker status, freshness, a real POST connection-test form, and a real container-list link.
- `/servers/{id}/containers` is refreshable and provides a back link to the server.
- `/servers/{id}/containers/{containerID}/logs` is an administrator-only full page.
- HTMX navigation uses the same URLs, pushes primary URL state, and includes explicit `Checking containers...` or `Testing Docker...` indicators.
- Empty container state means Docker returned no containers. Error state means the Docker read failed; the two are never collapsed.
- Desktop uses a resource-specific table. Mobile uses a concise card with name/image hierarchy, status badge, runtime text, and a full-width log action.
- Docker errors remain persistent and include one error reference plus **See Error Log** for the administrator-only surface.

Do not add container mutation controls until Phase 5. Do not replace resource-specific layouts with a generic table or JavaScript request framework.
