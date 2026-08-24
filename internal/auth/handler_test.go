package auth

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestPostLoginRejectsQuerySourcedCSRF(t *testing.T) {
	now := time.Now().UTC()
	csrf := NewCSRF(bytes.Repeat([]byte{1}, 32))
	limiter := NewLoginLimiter(func() time.Time { return now })
	service := NewService(&Store{}, limiter, func() time.Time { return now })
	handler, err := newBoundaryTestHandler(&Store{}, service, csrf, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	contextCookie, token, _, err := csrf.NewLoginContext(now)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"email": {"not-an-email"}, "password": {"not-a-password"}}
	request := httptest.NewRequest(http.MethodPost, "/login?csrf_token="+url.QueryEscape(token), strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: loginCookieDev, Value: contextCookie})
	response := httptest.NewRecorder()

	handler.ProtectLoginPost(http.HandlerFunc(handler.PostLogin)).ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("query-sourced CSRF token must be rejected with 403; got %d", response.Code)
	}
}

func TestPostLogoutDoesNotHideSessionDeletionFailure(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	csrf := NewCSRF(bytes.Repeat([]byte{2}, 32))
	handler, err := newBoundaryTestHandler(&Store{database: database}, nil, csrf, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	session := "active-session"
	token, err := csrf.AuthToken(session)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{CSRFField: {token}}
	request := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: sessionCookieDev, Value: session})
	response := httptest.NewRecorder()

	handler.ProtectAuthenticatedPost(http.HandlerFunc(handler.PostLogout)).ServeHTTP(response, request)

	if response.Code == http.StatusSeeOther {
		t.Fatalf("logout reported success after server-side deletion failed; got %d", response.Code)
	}
}

func TestLoginMalformedEmailConsumesGlobalBudget(t *testing.T) {
	now := time.Now().UTC()
	limiter := NewLoginLimiter(func() time.Time { return now })
	service := NewService(nil, limiter, func() time.Time { return now })

	_, _, _, _ = service.Login(t.Context(), "not-an-email", "not-a-password")

	if limiter.tokens == globalBurst {
		t.Fatalf("invalid email attempt bypassed process-wide limiter; tokens=%v", limiter.tokens)
	}
}

func TestLoginPageIncludesHTMXEnhancement(t *testing.T) {
	now := time.Now().UTC()
	csrf := NewCSRF(bytes.Repeat([]byte{5}, 32))
	handler, err := newBoundaryTestHandler(nil, nil, csrf, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()

	handler.GetLogin(response, request)

	body := response.Body.String()
	if !strings.Contains(body, "/static/htmx-1.9.12.min.js") || !strings.Contains(body, "/static/application.js") {
		t.Fatal("login page is not HTMX-enhanced through the base layout")
	}
}

func TestPostPasswordDoesNotExposeDatabaseError(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	currentPassword := "current-password"
	currentHash, err := HashPassword(currentPassword)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	csrf := NewCSRF(bytes.Repeat([]byte{6}, 32))
	store := &Store{database: database}
	service := NewService(store, NewLoginLimiter(func() time.Time { return now }), func() time.Time { return now })
	handler, err := newBoundaryTestHandler(store, service, csrf, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	session := "active-session"
	token, err := csrf.AuthToken(session)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		CSRFField:          {token},
		"current_password": {currentPassword},
		"new_password":     {"replacement-password"},
		"confirm_password": {"replacement-password"},
	}
	request := httptest.NewRequest(http.MethodPost, "/account/password", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: sessionCookieDev, Value: session})
	request = request.WithContext(context.WithValue(request.Context(), contextKey{}, User{ID: "user-1", PasswordHash: currentHash, Role: "admin"}))
	response := httptest.NewRecorder()

	handler.ProtectAuthenticatedPost(http.HandlerFunc(handler.PostPassword)).ServeHTTP(response, request)

	if strings.Contains(strings.ToLower(response.Body.String()), "database is closed") {
		t.Fatalf("password failure exposed raw database detail: %q", response.Body.String())
	}
}

func TestProductionSessionCookieUsesHostPrefix(t *testing.T) {
	handler := &Handler{development: false, clock: time.Now}
	response := httptest.NewRecorder()

	handler.setSessionCookie(response, "session", time.Now().Add(time.Hour))

	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if !strings.HasPrefix(cookie.Name, "__Host-") || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("production session cookie violates host-cookie contract: %#v", cookie)
	}
}

func TestSuccessfulHTMXLoginRedirectsBrowserURL(t *testing.T) {
	now := time.Now().UTC()
	csrf := NewCSRF(bytes.Repeat([]byte{7}, 32))
	handler, err := newBoundaryTestHandler(nil, NewService(nil, NewLoginLimiter(func() time.Time { return now }), func() time.Time { return now }), csrf, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.Header.Set("HX-Request", "true")
	response := httptest.NewRecorder()

	handler.respondToLogin(response, request, loginResponse{session: "new-session", expires: now.Add(time.Hour)})

	if response.Code != http.StatusNoContent || response.Header().Get("HX-Redirect") != "/" {
		t.Fatalf("expected HTMX redirect to home, got status=%d redirect=%q", response.Code, response.Header().Get("HX-Redirect"))
	}
}

func newBoundaryTestHandler(store *Store, service *Service, csrf *CSRF, clock func() time.Time) (*Handler, error) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	recordFailure := func(DiagnosticFailure) string { return "err_test" }
	if service == nil {
		service = NewService(store, NewLoginLimiter(clock), clock)
	}
	return NewHandler(service, csrf, clock, true, "http://127.0.0.1:8080", logger, recordFailure)
}
