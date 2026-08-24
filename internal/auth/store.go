package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

const sessionLifetime = 12 * time.Hour

var ErrNotFound = errors.New("authentication record not found")

type User struct {
	ID           string
	Email        string
	Name         string
	PasswordHash string
	Role         string
}

type Store struct{ database *sql.DB }

func NewStore(database *sql.DB) *Store { return &Store{database: database} }

func NormalizeEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(normalized)
	if err != nil || address.Address != normalized || len(normalized) > 254 {
		return "", errors.New("enter a valid email address")
	}
	return normalized, nil
}

func (store *Store) CreateUser(ctx context.Context, email, name, passwordHash, role string) (User, error) {
	id, err := randomID()
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = store.database.ExecContext(ctx, `INSERT INTO users(id,email,name,password_hash,role,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, id, email, strings.TrimSpace(name), passwordHash, role, now, now)
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return User{ID: id, Email: email, Name: strings.TrimSpace(name), PasswordHash: passwordHash, Role: role}, nil
}

func (store *Store) FindUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := store.database.QueryRowContext(ctx, `SELECT id,email,name,password_hash,role FROM users WHERE email=?`, email).Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user: %w", err)
	}
	return user, nil
}

func (store *Store) CreateSession(ctx context.Context, userID string, now time.Time) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("generate session: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(token))
	expires := now.Add(sessionLifetime)
	_, err := store.database.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)`, hex.EncodeToString(digest[:]), userID, expires.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return token, expires, nil
}

func (store *Store) FindSession(ctx context.Context, token string, now time.Time) (User, error) {
	digest := sha256.Sum256([]byte(token))
	var user User
	var expiresText string
	err := store.database.QueryRowContext(ctx, `SELECT u.id,u.email,u.name,u.password_hash,u.role,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, hex.EncodeToString(digest[:])).Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.Role, &expiresText)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find session: %w", err)
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresText)
	if err != nil || !expires.After(now) {
		return User{}, ErrNotFound
	}
	return user, nil
}

func (store *Store) DeleteSession(ctx context.Context, token string) error {
	digest := sha256.Sum256([]byte(token))
	_, err := store.database.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, hex.EncodeToString(digest[:]))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
func (store *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := store.database.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}
func (store *Store) UpdatePassword(ctx context.Context, userID, hash string) error {
	tx, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin password update: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET password_hash=?,updated_at=? WHERE id=?`, hash, time.Now().UTC().Format(time.RFC3339Nano), userID); err != nil {
		return rollbackPasswordUpdate(tx, fmt.Errorf("update password: %w", err))
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID); err != nil {
		return rollbackPasswordUpdate(tx, fmt.Errorf("invalidate sessions: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit password update: %w", err)
	}
	return nil
}
func (store *Store) CleanupExpired(ctx context.Context, now time.Time) (int64, error) {
	result, err := store.database.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read expired session cleanup result: %w", err)
	}
	return removed, nil
}

func rollbackPasswordUpdate(transaction *sql.Tx, updateError error) error {
	rollbackError := transaction.Rollback()
	if rollbackError == nil {
		return updateError
	}
	return errors.Join(updateError, fmt.Errorf("rollback password update: %w", rollbackError))
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
