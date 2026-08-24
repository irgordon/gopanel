CREATE TABLE servers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    connection_type TEXT NOT NULL CHECK (connection_type IN ('docker', 'caddy', 'vault', 'kubernetes')),
    credential_reference TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE audit_log (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    result TEXT NOT NULL CHECK (result IN ('attempted', 'success', 'failed')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX audit_log_user_id_index ON audit_log(user_id);
CREATE INDEX audit_log_target_index ON audit_log(target_type, target_id);
