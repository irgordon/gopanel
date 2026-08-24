UPDATE servers
SET credential_reference = NULL
WHERE credential_reference IS NOT NULL;

CREATE TABLE audit_log_rebuilt (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    result TEXT NOT NULL CHECK (result IN ('attempted', 'success', 'failed')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO audit_log_rebuilt (
    id, user_id, action, target_type, target_id, result, created_at, updated_at
)
SELECT id, user_id, action, target_type, target_id, result, created_at, updated_at
FROM audit_log;

DROP TABLE audit_log;
ALTER TABLE audit_log_rebuilt RENAME TO audit_log;

CREATE INDEX audit_log_user_id_index ON audit_log(user_id);
CREATE INDEX audit_log_target_index ON audit_log(target_type, target_id);
