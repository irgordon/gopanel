package audit

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/irgordon/gopanel/internal/store"
)

func TestRecordResultRejectsSecondTransition(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gopanel.db")
	db := openTestStore(t, dbPath)
	defer db.Close()
	ctx := context.Background()
	userID := createTestUser(t, db)
	auditStore := NewStore(db.SQLDatabase())

	rec, err := auditStore.RecordAttempt(ctx, userID, "create_server", "server", "server-1")
	if err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}
	if err := auditStore.RecordResult(ctx, rec.ID, ResultSuccess); err != nil {
		t.Fatalf("first RecordResult: %v", err)
	}
	// Second transition must fail (row no longer attempted) — GP-014
	err = auditStore.RecordResult(ctx, rec.ID, ResultFailed)
	if err == nil {
		t.Fatal("expected second RecordResult to fail when row already success")
	}
	// Verify row remains success, not silently overwritten
	var result string
	if err := db.SQLDatabase().QueryRowContext(ctx, `SELECT result FROM audit_log WHERE id=?`, rec.ID).Scan(&result); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if result != ResultSuccess {
		t.Fatalf("expected result success, got %q", result)
	}
}

func TestRecordResultAllowsOnlyAttemptedToSuccessOrFailed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gopanel.db")
	db := openTestStore(t, dbPath)
	defer db.Close()
	ctx := context.Background()
	userID := createTestUser(t, db)
	auditStore := NewStore(db.SQLDatabase())
	rec, _ := auditStore.RecordAttempt(ctx, userID, "create_server", "server", "server-1")
	// Invalid result must be rejected
	if err := auditStore.RecordResult(ctx, rec.ID, "invalid"); err == nil {
		t.Fatal("expected invalid result to fail")
	}
}

func createTestUser(t *testing.T, db *store.Store) string {
	t.Helper()
	// Insert minimal user directly to satisfy FK; bypass auth hashing for test speed
	userID := "test-user-123"
	_, err := db.SQLDatabase().ExecContext(context.Background(), `INSERT INTO users(id, email, name, password_hash, role, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`,
		userID, "test@example.com", "Test", "$argon2id$v=19$m=19456,t=2,p=1$salt$hash", "admin", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return userID
}

func openTestStore(t *testing.T, path string) *store.Store {
	t.Helper()
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
