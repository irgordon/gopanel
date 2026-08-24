package audit

import (
	"context"
	"database/sql"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	ResultAttempted = "attempted"
	ResultSuccess   = "success"
	ResultFailed    = "failed"
)

type Record struct {
	ID         string
	UserID     string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func RecordAttempt(ctx context.Context, db *sql.DB, userID, action, targetType, targetID string) (Record, error) {
	id, err := randomID()
	if err != nil {
		return Record{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `INSERT INTO audit_log(id, user_id, action, target_type, target_id, result, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		id, userID, action, targetType, targetID, ResultAttempted, now, now)
	if err != nil {
		return Record{}, fmt.Errorf("insert audit attempted: %w", err)
	}
	return Record{ID: id, UserID: userID, Action: action, TargetType: targetType, TargetID: targetID, Result: ResultAttempted}, nil
}

func RecordResult(ctx context.Context, db *sql.DB, auditID, result string) error {
	if result != ResultSuccess && result != ResultFailed {
		return fmt.Errorf("audit result must be success or failed")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.ExecContext(ctx, `UPDATE audit_log SET result=?, updated_at=? WHERE id=? AND result=?`,
		result, now, auditID, ResultAttempted)
	if err != nil {
		return fmt.Errorf("update audit result: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("audit row not in attempted state")
	}
	return nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
