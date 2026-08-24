package server

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/irgordon/gopanel/internal/audit"
	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/diagnostic"
	basestore "github.com/irgordon/gopanel/internal/store"
)

func TestHandleCreateRejectsHostileOrigin(t *testing.T) {
	csrf := auth.NewCSRF(bytes.Repeat([]byte{3}, 32))
	session := "active-session"
	token, err := csrf.AuthToken(session)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"csrf_token": {token}}
	request := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "https://attacker.example")
	request.AddCookie(&http.Cookie{Name: "gopanel_session_dev", Value: session})
	response := httptest.NewRecorder()
	handler := &Handler{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authService := auth.NewService(nil, auth.NewLoginLimiter(time.Now), time.Now)
	authHandler, err := auth.NewHandler(authService, csrf, time.Now, true, "http://127.0.0.1:8080", logger, func(auth.DiagnosticFailure) string { return "err_test" })
	if err != nil {
		t.Fatal(err)
	}

	authHandler.ProtectAuthenticatedPost(http.HandlerFunc(handler.HandleCreate)).ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("hostile origin must be rejected with 403; got %d", response.Code)
	}
}

func TestHandleCreatePushesCreatedResourceURLForHTMX(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	diagnostics := diagnostic.NewRecorder(logger)
	service := newService(&fakeRegistrationStore{}, &fakeRegistrationAuditStore{}, fixedServerID("server-123"))
	handler := NewHandler(service, diagnostics, logger, nil)
	request := httptest.NewRequest(http.MethodPost, "/servers", nil)
	request.Header.Set("HX-Request", "true")
	request.PostForm = url.Values{
		"name":            {"prod-docker"},
		"address":         {"10.0.0.12"},
		"connection_type": {"docker"},
	}
	response := httptest.NewRecorder()

	handler.HandleCreate(response, request)

	if response.Code != http.StatusCreated || response.Header().Get("HX-Push-Url") != "/servers/server-123" {
		t.Fatalf("expected created URL push, got status=%d push=%q", response.Code, response.Header().Get("HX-Push-Url"))
	}
}

func TestHandleCreateShowsAuditPartialCompletion(t *testing.T) {
	ctx := context.Background()
	database, err := basestore.Open(ctx, filepath.Join(t.TempDir(), "gopanel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	authStore := auth.NewStore(database.SQLDatabase())
	user, err := authStore.CreateUser(ctx, "admin@example.com", "Admin", "test-hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := authStore.CreateSession(ctx, user.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.SQLDatabase().ExecContext(ctx, `CREATE TRIGGER fail_audit_update BEFORE UPDATE OF result ON audit_log BEGIN SELECT RAISE(FAIL, 'forced audit update failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	diagnostics := diagnostic.NewRecorder(logger)
	csrf := auth.NewCSRF(bytes.Repeat([]byte{4}, 32))
	authHandler, err := auth.NewHandler(auth.NewService(authStore, auth.NewLoginLimiter(time.Now), time.Now), csrf, time.Now, true, "http://127.0.0.1:8080", logger, diagnostic.AuthFailureRecorder(diagnostics))
	if err != nil {
		t.Fatal(err)
	}
	serverService := NewService(NewStore(database.SQLDatabase()), audit.NewStore(database.SQLDatabase()))
	handler := NewHandler(serverService, diagnostics, logger, authHandler.AuthenticatedFormToken)
	token, err := csrf.AuthToken(session)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		auth.CSRFField:    {token},
		"name":            {"prod-docker"},
		"address":         {"10.0.0.12"},
		"connection_type": {"docker"},
	}
	request := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	request.AddCookie(&http.Cookie{Name: "gopanel_session_dev", Value: session})
	response := httptest.NewRecorder()
	protected := authHandler.RequireLogin(authHandler.RequireAdmin(authHandler.ProtectAuthenticatedPost(http.HandlerFunc(handler.HandleCreate))))

	protected.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 partial completion, got %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Server created, but audit is incomplete") || !strings.Contains(body, "Do not submit the form again") {
		t.Fatalf("expected explicit partial-completion guidance, got %q", body)
	}
	var result string
	if err := database.SQLDatabase().QueryRowContext(ctx, `SELECT result FROM audit_log LIMIT 1`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	if result != audit.ResultAttempted {
		t.Fatalf("expected attempted audit row, got %q", result)
	}
	records := diagnostics.Snapshot()
	if len(records) != 1 || records[0].AuditCorrelationID == "" {
		t.Fatalf("expected one correlated diagnostic, got %#v", records)
	}
}
