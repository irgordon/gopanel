package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	basestore "github.com/irgordon/gopanel/internal/store"
)

func TestSessionStorageHashesCredentialAndPasswordChangeInvalidatesAll(t *testing.T) {
	database, authStore := openAuthTestStore(t)
	defer database.Close()
	ctx := context.Background()
	hash, err := HashPassword("current-password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := authStore.CreateUser(ctx, "admin@example.com", "Admin", hash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	first, _, err := authStore.CreateSession(ctx, user.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := authStore.CreateSession(ctx, user.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := database.SQLDatabase().QueryRowContext(ctx, `SELECT token_hash FROM sessions LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == first || stored == second || len(stored) != 64 {
		t.Fatalf("expected SHA-256 session hash, got %q", stored)
	}
	service := NewService(authStore, NewLoginLimiter(func() time.Time { return now }), func() time.Time { return now })
	if err := service.ChangePassword(ctx, user, "current-password", "replacement-password", "replacement-password"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first, second} {
		if _, err := authStore.FindSession(ctx, token, now); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected session invalidation, got %v", err)
		}
	}
}

func TestCleanupExpiredRemovesOnlyExpiredSessions(t *testing.T) {
	database, authStore := openAuthTestStore(t)
	defer database.Close()
	ctx := context.Background()
	user, err := authStore.CreateUser(ctx, "admin@example.com", "Admin", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expired, _, err := authStore.CreateSession(ctx, user.ID, now.Add(-sessionLifetime-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	active, _, err := authStore.CreateSession(ctx, user.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := authStore.CleanupExpired(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("expected one removed session, got %d", removed)
	}
	if _, err := authStore.FindSession(ctx, expired, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected expired session removal, got %v", err)
	}
	if _, err := authStore.FindSession(ctx, active, now); err != nil {
		t.Fatalf("expected active session, got %v", err)
	}
}

func TestLoginUsesSameFailureForUnknownAndWrongPassword(t *testing.T) {
	database, authStore := openAuthTestStore(t)
	defer database.Close()
	ctx := context.Background()
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authStore.CreateUser(ctx, "admin@example.com", "Admin", hash, "admin"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	service := NewService(authStore, NewLoginLimiter(func() time.Time { return now }), func() time.Time { return now })
	_, _, _, unknownError := service.Login(ctx, "unknown@example.com", "wrong-password")
	_, _, _, wrongError := service.Login(ctx, "admin@example.com", "wrong-password")
	if !errors.Is(unknownError, ErrInvalidCredentials) || !errors.Is(wrongError, ErrInvalidCredentials) || unknownError.Error() != wrongError.Error() {
		t.Fatalf("expected identical credential failure, got unknown=%v wrong=%v", unknownError, wrongError)
	}
}

func openAuthTestStore(t *testing.T) (*basestore.Store, *Store) {
	t.Helper()
	database, err := basestore.Open(context.Background(), filepath.Join(t.TempDir(), "gopanel.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(context.Background()); err != nil {
		database.Close()
		t.Fatal(err)
	}
	return database, NewStore(database.SQLDatabase())
}
