package audit

import (
	"context"
	"crypto/rand"
	"database/sql"
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

type Store struct {
	database *sql.DB
	clock    func() time.Time
}

func NewStore(database *sql.DB) *Store {
	return &Store{database: database, clock: time.Now}
}

func (store *Store) RecordAttempt(ctx context.Context, userID, action, targetType, targetID string) (Record, error) {
	id, err := randomID()
	if err != nil {
		return Record{}, err
	}
	now := store.clock().UTC()
	timestamp := now.Format(time.RFC3339Nano)
	_, err = store.database.ExecContext(ctx, `INSERT INTO audit_log(id, user_id, action, target_type, target_id, result, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		id, userID, action, targetType, targetID, ResultAttempted, timestamp, timestamp)
	if err != nil {
		return Record{}, fmt.Errorf("insert audit attempted: %w", err)
	}
	return Record{ID: id, UserID: userID, Action: action, TargetType: targetType, TargetID: targetID, Result: ResultAttempted, CreatedAt: now, UpdatedAt: now}, nil
}

func (store *Store) RecordResult(ctx context.Context, auditID, result string) error {
	if result != ResultSuccess && result != ResultFailed {
		return fmt.Errorf("audit result must be success or failed")
	}
	now := store.clock().UTC().Format(time.RFC3339Nano)
	res, err := store.database.ExecContext(ctx, `UPDATE audit_log SET result=?, updated_at=? WHERE id=? AND result=?`,
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
