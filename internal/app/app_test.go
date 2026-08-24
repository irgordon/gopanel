package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/config"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/store"
)

func TestRootRouteRendersCompleteHTML(t *testing.T) {
	application := newTestApplication(t)
	recorder := serveRequest(application, http.MethodGet, "/", false)

	assertStatus(t, recorder, http.StatusOK)
	assertContains(t, recorder.Body.String(), "<!doctype html>")
	assertContains(t, recorder.Body.String(), "The GoPanel scaffold is running.")
	assertContains(t, recorder.Body.String(), "src=\"/static/htmx-1.9.12.min.js\"")
	assertContains(t, recorder.Body.String(), "src=\"/static/application.js\"")
	assertContains(t, recorder.Body.String(), "href=\"/static/output.css\"")
	assertContains(t, recorder.Body.String(), "\"allowEval\":false")
	assertContains(t, recorder.Body.String(), "\"selfRequestsOnly\":true")

	if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("expected HTML content type, got %q", recorder.Header().Get("Content-Type"))
	}
	if strings.Contains(recorder.Body.String(), "https://") || strings.Contains(recorder.Body.String(), "http://") {
		t.Fatal("expected the page to use same-origin assets only")
	}
	if strings.Count(recorder.Body.String(), "<script") != strings.Count(recorder.Body.String(), "<script src=") {
		t.Fatal("expected every script element to load an external source")
	}
}

func TestRootRouteSetsBrowserSecurityHeaders(t *testing.T) {
	application := newTestApplication(t)
	recorder := serveRequest(application, http.MethodGet, "/", false)

	assertContains(t, recorder.Header().Get("Content-Security-Policy"), "default-src 'self'")
	assertContains(t, recorder.Header().Get("Content-Security-Policy"), "script-src 'self'")
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", recorder.Header().Get("X-Content-Type-Options"))
	}
	if recorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("expected no-referrer policy, got %q", recorder.Header().Get("Referrer-Policy"))
	}
}

func TestStaticAssetsAreServedLocally(t *testing.T) {
	application := newTestApplication(t)
	tests := []struct {
		path    string
		content string
	}{
		{path: "/static/htmx-1.9.12.min.js", content: "htmx"},
		{path: "/static/application.js", content: "htmx:beforeSwap"},
		{path: "/static/output.css", content: "--tw-"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder := serveRequest(application, http.MethodGet, test.path, false)
			assertStatus(t, recorder, http.StatusOK)
			assertContains(t, recorder.Body.String(), test.content)
		})
	}
}

func TestNotFoundReturnsSafeHTMLFragment(t *testing.T) {
	application := newTestApplication(t)
	recorder := serveRequest(application, http.MethodGet, "/missing", true)

	assertStatus(t, recorder, http.StatusNotFound)
	assertContains(t, recorder.Body.String(), "GoPanel could not find that page.")
	if strings.Contains(recorder.Body.String(), "<!doctype html>") {
		t.Fatal("expected an HTMX not-found fragment")
	}
}

func TestRenderFailureDoesNotExposeRawError(t *testing.T) {
	for _, htmx := range []bool{false, true} {
		application, diagnostics, _, logs := newTestEnvironment(t)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		if htmx {
			request.Header.Set("HX-Request", "true")
		}
		component := templ.ComponentFunc(func(context.Context, io.Writer) error {
			return errors.New("private rendering detail")
		})

		application.respondHTML(recorder, request, http.StatusOK, component)
		assertRenderFailureCorrelation(t, recorder, diagnostics, logs.String(), htmx)
	}
}

func TestHealthReportsProcessLiveness(t *testing.T) {
	application := newTestApplication(t)
	recorder := serveRequest(application, http.MethodGet, "/healthz", false)

	assertStatus(t, recorder, http.StatusOK)
	if recorder.Body.String() != "alive\n" {
		t.Fatalf("expected stable liveness response, got %q", recorder.Body.String())
	}
}

func TestReadinessReportsReadyAfterSQLiteAndMigrations(t *testing.T) {
	application := newTestApplication(t)
	recorder := serveRequest(application, http.MethodGet, "/readyz", false)

	assertStatus(t, recorder, http.StatusOK)
	if recorder.Body.String() != "ready\n" {
		t.Fatalf("expected stable readiness response, got %q", recorder.Body.String())
	}
}

func TestReadinessRejectsUnavailableSQLite(t *testing.T) {
	application, _, database, _ := newTestEnvironment(t)
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	recorder := serveRequest(application, http.MethodGet, "/readyz", false)
	assertStatus(t, recorder, http.StatusServiceUnavailable)
	if recorder.Body.String() != "not ready\n" {
		t.Fatalf("expected stable not-ready response, got %q", recorder.Body.String())
	}
}

func TestReadinessRejectsShuttingDownApplication(t *testing.T) {
	application := newTestApplication(t)
	application.shuttingDown.Store(true)

	recorder := serveRequest(application, http.MethodGet, "/readyz", false)
	assertStatus(t, recorder, http.StatusServiceUnavailable)
}

func TestNewRejectsInvalidConfigurationBeforeListening(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	database := openMigratedStore(t, databasePath)
	defer closeTestStore(t, database)
	logger, diagnostics, _ := testObservability()
	invalid := testConfig(databasePath, "not-an-address")
	authHandler := newTestAuthHandler(t, database)

	_, err := New(invalid, database, logger, diagnostics, authHandler)
	if err == nil || !strings.Contains(err.Error(), "validate application configuration") {
		t.Fatalf("expected invalid configuration rejection, got %v", err)
	}
}

func TestRunPropagatesListenFailureAndClosesDatabase(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test address: %v", err)
	}
	defer listener.Close()

	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	database := openMigratedStore(t, databasePath)
	logger, diagnostics, _ := testObservability()
	authHandler := newTestAuthHandler(t, database)
	application, err := New(testConfig(databasePath, listener.Addr().String()), database, logger, diagnostics, authHandler)
	if err != nil {
		t.Fatalf("construct application: %v", err)
	}

	err = application.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "listen on configured address") {
		t.Fatalf("expected listen failure, got %v", err)
	}
	if err := database.Ready(context.Background()); err == nil {
		t.Fatal("expected database to close after listen failure")
	}
}

func TestRunHandlesSIGTERMWhileIdle(t *testing.T) {
	application, logs, database := newLifecycleEnvironment(t)
	listener := listenOnLoopback(t)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := runOnListener(application, ctx, listener)
	waitForListening(t, application)

	sendSIGTERM(t)
	if err := waitForRunResult(t, result); err != nil {
		t.Fatalf("stop idle application: %v", err)
	}
	if err := database.Ready(context.Background()); err == nil {
		t.Fatal("expected database to close after shutdown")
	}
	assertLifecycleEvents(t, logs.String(),
		"listening",
		"shutdown_initiated",
		"http_drain_completed",
		"database_closed",
		"shutdown_completed",
	)
}

func TestRunDrainsInFlightRequestOnSIGTERM(t *testing.T) {
	application, _, _ := newLifecycleEnvironment(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	installControlledHandler(application, entered, release)
	listener := listenOnLoopback(t)
	address := listener.Addr().String()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := runOnListener(application, ctx, listener)
	waitForListening(t, application)

	requestResult := requestControlledPath(address)
	waitForSignal(t, entered, "in-flight request")
	sendSIGTERM(t)
	waitForShutdownState(t, application)

	readiness := serveRequest(application, http.MethodGet, "/readyz", false)
	assertStatus(t, readiness, http.StatusServiceUnavailable)
	assertStillRunning(t, result)

	close(release)
	if err := <-requestResult; err != nil {
		t.Fatalf("complete in-flight request: %v", err)
	}
	if err := waitForRunResult(t, result); err != nil {
		t.Fatalf("drain application: %v", err)
	}
}

func TestRunBoundsShutdownDeadline(t *testing.T) {
	application, logs, database := newLifecycleEnvironment(t)
	application.drainTimeout = 25 * time.Millisecond
	entered := make(chan struct{})
	release := make(chan struct{})
	installControlledHandler(application, entered, release)
	listener := listenOnLoopback(t)
	address := listener.Addr().String()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := runOnListener(application, ctx, listener)
	waitForListening(t, application)

	requestResult := requestControlledPath(address)
	waitForSignal(t, entered, "deadline request")
	sendSIGTERM(t)

	runError := waitForRunResult(t, result)
	if runError == nil || !strings.Contains(runError.Error(), "context deadline exceeded") {
		t.Fatalf("expected bounded drain error, got %v", runError)
	}
	if err := database.Ready(context.Background()); err == nil {
		t.Fatal("expected database to close after drain deadline")
	}
	assertContains(t, logs.String(), "event=http_drain_deadline_reached")
	assertContains(t, logs.String(), "error_ref=err_")
	close(release)
	<-requestResult
}

func newTestApplication(t *testing.T) *Application {
	t.Helper()
	application, _, _, _ := newTestEnvironment(t)
	return application
}

func newTestEnvironment(t *testing.T) (*Application, *diagnostic.Recorder, *store.Store, *bytes.Buffer) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	database := openMigratedStore(t, databasePath)
	logger, diagnostics, logs := testObservability()
	authHandler := newTestAuthHandler(t, database)
	application, err := New(testConfig(databasePath, "127.0.0.1:8080"), database, logger, diagnostics, authHandler)
	if err != nil {
		t.Fatalf("construct application: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return application, diagnostics, database, logs
}

func newTestAuthHandler(t *testing.T, database *store.Store) *auth.Handler {
	t.Helper()
	csrfKey, err := auth.NewCSRFKey()
	if err != nil {
		t.Fatalf("create CSRF key: %v", err)
	}
	csrf := auth.NewCSRF(csrfKey)
	authStore := auth.NewStore(database.SQLDatabase())
	limiter := auth.NewLoginLimiter(time.Now)
	service := auth.NewService(authStore, limiter, time.Now)
	handler, err := auth.NewHandler(authStore, service, csrf, time.Now, true, "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("create auth handler: %v", err)
	}
	return handler
}

func assertRenderFailureCorrelation(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	diagnostics *diagnostic.Recorder,
	logs string,
	htmx bool,
) {
	t.Helper()
	assertStatus(t, recorder, http.StatusInternalServerError)
	assertContains(t, recorder.Header().Get("Content-Type"), "text/html")
	assertContains(t, recorder.Body.String(), "The page could not be rendered. Try again.")
	assertContains(t, recorder.Body.String(), "See Error Log")

	records := diagnostics.Snapshot()
	if len(records) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(records))
	}
	assertContains(t, recorder.Body.String(), records[0].ID)
	assertContains(t, logs, "error_ref="+records[0].ID)
	if strings.Contains(recorder.Body.String(), "private rendering detail") {
		t.Fatal("expected raw rendering error to remain out of the response")
	}
	if htmx == strings.Contains(recorder.Body.String(), "<!doctype html>") {
		t.Fatalf("unexpected full-page state for htmx=%t", htmx)
	}
}

func newLifecycleEnvironment(t *testing.T) (*Application, *bytes.Buffer, *store.Store) {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	database := openMigratedStore(t, databasePath)
	logger, diagnostics, logs := testObservability()
	authHandler := newTestAuthHandler(t, database)
	application, err := New(testConfig(databasePath, "127.0.0.1:8080"), database, logger, diagnostics, authHandler)
	if err != nil {
		t.Fatalf("construct lifecycle application: %v", err)
	}
	return application, logs, database
}

func openMigratedStore(t *testing.T, databasePath string) *store.Store {
	t.Helper()
	database, err := store.Open(context.Background(), databasePath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate test store: %v", err)
	}
	return database
}

func closeTestStore(t *testing.T, database *store.Store) {
	t.Helper()
	if err := database.Close(); err != nil {
		t.Fatalf("close test store: %v", err)
	}
}

func testConfig(databasePath string, address string) config.Config {
	return config.Config{
		ListenAddress: address,
		DatabasePath:  databasePath,
		PublicURL:     "http://127.0.0.1:8080",
		Development:   true,
	}
}

func testObservability() (*slog.Logger, *diagnostic.Recorder, *bytes.Buffer) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	return logger, diagnostic.NewRecorder(logger), &logs
}

func listenOnLoopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for lifecycle test: %v", err)
	}
	return listener
}

func runOnListener(application *Application, ctx context.Context, listener net.Listener) <-chan error {
	result := make(chan error, 1)
	go func() {
		result <- application.runLifecycle(ctx, listener)
	}()
	return result
}

func waitForListening(t *testing.T, application *Application) {
	t.Helper()
	waitForSignal(t, application.listeningSignal, "application listening")
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func sendSIGTERM(t *testing.T) {
	t.Helper()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
}

func waitForShutdownState(t *testing.T, application *Application) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for shutdown state")
		case <-ticker.C:
			if application.shuttingDown.Load() {
				return
			}
		}
	}
}

func waitForRunResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for application to stop")
		return nil
	}
}

func assertStillRunning(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("application stopped before request drained: %v", err)
	default:
	}
}

func installControlledHandler(application *Application, entered chan<- struct{}, release <-chan struct{}) {
	original := application.server.Handler
	application.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/controlled" {
			original.ServeHTTP(w, r)
			return
		}

		close(entered)
		select {
		case <-release:
			w.WriteHeader(http.StatusNoContent)
		case <-r.Context().Done():
		}
	})
}

func requestControlledPath(address string) <-chan error {
	result := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + address + "/controlled")
		if err != nil {
			result <- err
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			result <- errors.New("controlled request returned an unexpected status")
			return
		}
		result <- nil
	}()
	return result
}

func serveRequest(application *Application, method string, target string, htmx bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	if htmx {
		request.Header.Set("HX-Request", "true")
	}

	recorder := httptest.NewRecorder()
	application.Handler().ServeHTTP(recorder, request)
	return recorder
}

func assertStatus(t *testing.T, recorder *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if recorder.Code != expected {
		t.Fatalf("expected status %d, got %d", expected, recorder.Code)
	}
}

func assertContains(t *testing.T, content string, expected string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Fatalf("expected content to contain %q", expected)
	}
}

func assertLifecycleEvents(t *testing.T, logs string, events ...string) {
	t.Helper()
	for _, event := range events {
		assertContains(t, logs, "event="+event)
	}
}
