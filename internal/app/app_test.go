package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/irgordon/gopanel/internal/audit"
	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/config"
	"github.com/irgordon/gopanel/internal/container"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/server"
	"github.com/irgordon/gopanel/internal/store"
)

func TestRootRouteRendersCompleteHTML(t *testing.T) {
	application := newTestApplication(t)
	recorder := serveAuthenticatedRequest(t, application, http.MethodGet, "/", false)

	assertStatus(t, recorder, http.StatusOK)
	assertContains(t, recorder.Body.String(), "<!doctype html>")
	assertContains(t, recorder.Body.String(), "Register and manage infrastructure safely.")
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

func TestRootRouteRequiresLogin(t *testing.T) {
	application := newTestApplication(t)
	recorder := serveRequest(application, http.MethodGet, "/", false)

	assertStatus(t, recorder, http.StatusUnauthorized)
	assertContains(t, recorder.Body.String(), "Sign in is required")
}

func TestContainerLogsRouteRequiresAdmin(t *testing.T) {
	application := newTestApplication(t)
	authStore := auth.NewStore(application.store.SQLDatabase())
	viewer, err := authStore.CreateUser(context.Background(), "viewer@example.test", "Viewer", "test-hash", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := authStore.CreateSession(context.Background(), viewer.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/servers/server-1/containers/"+strings.Repeat("a", 64)+"/logs", nil)
	request.AddCookie(&http.Cookie{Name: "gopanel_session_dev", Value: session})
	response := httptest.NewRecorder()

	application.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "Administrator access is required") {
		t.Fatalf("viewer log access was not denied: status=%d body=%q", response.Code, response.Body.String())
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

func TestHTTPServerUsesExplicitResourceBounds(t *testing.T) {
	application := newTestApplication(t)
	if application.server.ReadHeaderTimeout != readHeaderTimeout || application.server.ReadTimeout != readTimeout || application.server.WriteTimeout != writeTimeout || application.server.IdleTimeout != idleTimeout || application.server.MaxHeaderBytes != maximumHeaderBytes {
		t.Fatalf("HTTP server resource bounds are incomplete: %#v", application.server)
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
	authHandler, diagnosticHandler, serverHandler, sessionCleaner := newTestHandlers(t, database, logger, diagnostics)
	containerHandler, dockerService, dockerStatuses := newTestDocker(logger, diagnostics, authHandler)

	_, err := New(invalid, database, logger, diagnostics, authHandler, diagnosticHandler, serverHandler, containerHandler, sessionCleaner, dockerService, dockerStatuses, dockerService)
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
	authHandler, diagnosticHandler, serverHandler, sessionCleaner := newTestHandlers(t, database, logger, diagnostics)
	containerHandler, dockerService, dockerStatuses := newTestDocker(logger, diagnostics, authHandler)
	application, err := New(testConfig(databasePath, listener.Addr().String()), database, logger, diagnostics, authHandler, diagnosticHandler, serverHandler, containerHandler, sessionCleaner, dockerService, dockerStatuses, dockerService)
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

func TestSessionCleanupStopsWithLifecycleContext(t *testing.T) {
	application, _, _, _ := newTestEnvironment(t)
	cleaner := &testSessionCleaner{calls: make(chan struct{}, 1)}
	application.sessionCleaner = cleaner
	application.cleanupInterval = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go application.runSessionCleanup(ctx, done)
	select {
	case <-cleaner.calls:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session cleanup")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("session cleanup did not stop with lifecycle context")
	}
}

func TestSessionCleanupFailureCreatesSafeDiagnostic(t *testing.T) {
	application, diagnostics, _, _ := newTestEnvironment(t)
	application.sessionCleaner = &testSessionCleaner{err: errors.New("private database detail")}

	application.cleanupExpiredSessions(context.Background())

	records := diagnostics.Snapshot()
	if len(records) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(records))
	}
	if strings.Contains(records[0].TechnicalDetail, "private database detail") {
		t.Fatalf("cleanup diagnostic exposed raw detail: %q", records[0].TechnicalDetail)
	}
}

func TestDockerPollerNeverExceedsConfiguredConcurrency(t *testing.T) {
	application := newTestApplication(t)
	serverIDs := make([]string, 21)
	for index := range serverIDs {
		serverIDs[index] = fmt.Sprintf("docker-%02d", index)
	}
	checker := newTrackingDockerChecker(serverIDs)
	application.dockerChecker = checker
	application.dockerStatuses = container.NewStatusCache()
	application.dockerConcurrency = 6
	done := make(chan struct{})
	go func() {
		application.pollDockerStatuses(context.Background())
		close(done)
	}()

	for range application.dockerConcurrency {
		<-checker.started
	}
	if current := checker.current.Load(); current != int64(application.dockerConcurrency) {
		t.Fatalf("expected %d active checks, got %d", application.dockerConcurrency, current)
	}
	checker.release <- struct{}{}
	<-checker.started
	for range len(serverIDs) - 1 {
		checker.release <- struct{}{}
	}
	<-done

	if maximum := checker.maximum.Load(); maximum > int64(application.dockerConcurrency) {
		t.Fatalf("poller exceeded concurrency bound: max=%d bound=%d", maximum, application.dockerConcurrency)
	}
}

func TestDockerPollerStopsWithLifecycleContext(t *testing.T) {
	application, _, _ := newLifecycleEnvironment(t)
	checker := &cancelingDockerChecker{started: make(chan struct{})}
	application.dockerChecker = checker
	application.dockerStatuses = container.NewStatusCache()
	application.dockerInterval = time.Hour
	listener := listenOnLoopback(t)
	ctx, cancel := context.WithCancel(context.Background())
	result := runOnListener(application, ctx, listener)
	waitForSignal(t, checker.started, "Docker status check")
	cancel()

	if err := waitForRunResult(t, result); err != nil {
		t.Fatalf("shutdown under Docker polling failed: %v", err)
	}
}

func TestReadinessRemainsReadyWhenDockerIsUnavailable(t *testing.T) {
	application := newTestApplication(t)
	application.dockerChecker = failingDockerChecker{}
	application.dockerStatuses = container.NewStatusCache()
	application.pollDockerStatuses(context.Background())
	application.pollDockerStatuses(context.Background())

	recorder := serveRequest(application, http.MethodGet, "/readyz", false)
	assertStatus(t, recorder, http.StatusOK)
	if recorder.Body.String() != "ready\n" {
		t.Fatalf("Docker failure changed readiness: %q", recorder.Body.String())
	}
}

func TestBackgroundDockerFailureCorrelatesStatusDiagnosticAndLog(t *testing.T) {
	application, diagnostics, _, logs := newTestEnvironment(t)
	application.dockerChecker = failingDockerChecker{}
	application.dockerStatuses = container.NewStatusCache()
	application.pollDockerStatuses(context.Background())
	application.pollDockerStatuses(context.Background())

	status := application.dockerStatuses.Get("docker-1")
	if status.State != container.StatusUnavailable || status.ErrorReference == "" {
		t.Fatalf("background failure did not update status: %#v", status)
	}
	records := diagnostics.Snapshot()
	if len(records) != 1 || records[0].ID != status.ErrorReference || !strings.Contains(logs.String(), status.ErrorReference) {
		t.Fatalf("background correlation mismatch: status=%#v records=%#v logs=%q", status, records, logs.String())
	}
	if strings.Contains(records[0].TechnicalDetail, "private Docker SDK detail") || strings.Contains(logs.String(), "private Docker SDK detail") {
		t.Fatal("background Docker failure exposed raw SDK detail")
	}
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
	authHandler, diagnosticHandler, serverHandler, sessionCleaner := newTestHandlers(t, database, logger, diagnostics)
	containerHandler, dockerService, dockerStatuses := newTestDocker(logger, diagnostics, authHandler)
	application, err := New(testConfig(databasePath, "127.0.0.1:8080"), database, logger, diagnostics, authHandler, diagnosticHandler, serverHandler, containerHandler, sessionCleaner, dockerService, dockerStatuses, dockerService)
	if err != nil {
		t.Fatalf("construct application: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return application, diagnostics, database, logs
}

func newTestHandlers(t *testing.T, database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder) (*auth.Handler, *diagnostic.Handler, *server.Handler, *auth.Service) {
	t.Helper()
	csrfKey, err := auth.NewCSRFKey()
	if err != nil {
		t.Fatalf("create CSRF key: %v", err)
	}
	csrf := auth.NewCSRF(csrfKey)
	authStore := auth.NewStore(database.SQLDatabase())
	limiter := auth.NewLoginLimiter(time.Now)
	service := auth.NewService(authStore, limiter, time.Now)
	handler, err := auth.NewHandler(service, csrf, time.Now, true, "http://127.0.0.1:8080", logger, diagnostic.AuthFailureRecorder(diagnostics))
	if err != nil {
		t.Fatalf("create auth handler: %v", err)
	}
	diagnosticHandler := diagnostic.NewHandler(diagnostics, logger)
	serverStore := server.NewStore(database.SQLDatabase())
	auditStore := audit.NewStore(database.SQLDatabase())
	serverService := server.NewService(serverStore, auditStore)
	serverHandler := server.NewHandler(serverService, diagnostics, logger, handler.AuthenticatedFormToken, container.NewStatusCache())
	return handler, diagnosticHandler, serverHandler, service
}

func newTestDocker(logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler) (*container.Handler, *container.Service, *container.StatusCache) {
	reader := &testDockerReader{}
	lookup := func(context.Context, string) (container.RegisteredServer, bool, error) {
		return container.RegisteredServer{}, false, nil
	}
	lister := func(context.Context) ([]container.RegisteredServer, error) { return nil, nil }
	service := container.NewService(reader, lookup, lister)
	statuses := container.NewStatusCache()
	handler := container.NewHandler(service, statuses, diagnostics, logger, authHandler.AuthenticatedFormToken)
	return handler, service, statuses
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
	assertContains(t, recorder.Body.String(), "Contact an administrator")

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
	authHandler, diagnosticHandler, serverHandler, sessionCleaner := newTestHandlers(t, database, logger, diagnostics)
	containerHandler, dockerService, dockerStatuses := newTestDocker(logger, diagnostics, authHandler)
	application, err := New(testConfig(databasePath, "127.0.0.1:8080"), database, logger, diagnostics, authHandler, diagnosticHandler, serverHandler, containerHandler, sessionCleaner, dockerService, dockerStatuses, dockerService)
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
		DockerSocket:  "/var/run/docker.sock",
		Development:   true,
	}
}

type testDockerReader struct{}

func (*testDockerReader) Ping(context.Context) error { return nil }
func (*testDockerReader) ListContainers(context.Context) ([]container.Container, error) {
	return nil, nil
}
func (*testDockerReader) ViewLogs(context.Context, string, int) ([]byte, error) { return nil, nil }
func (*testDockerReader) Close() error                                          { return nil }

type trackingDockerChecker struct {
	serverIDs []string
	started   chan struct{}
	release   chan struct{}
	current   atomic.Int64
	maximum   atomic.Int64
}

func newTrackingDockerChecker(serverIDs []string) *trackingDockerChecker {
	return &trackingDockerChecker{serverIDs: serverIDs, started: make(chan struct{}, len(serverIDs)), release: make(chan struct{})}
}

func (checker *trackingDockerChecker) ListDockerServerIDs(context.Context) ([]string, error) {
	return checker.serverIDs, nil
}

func (checker *trackingDockerChecker) CheckStatus(context.Context, string) error {
	current := checker.current.Add(1)
	for {
		maximum := checker.maximum.Load()
		if current <= maximum || checker.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	checker.started <- struct{}{}
	<-checker.release
	checker.current.Add(-1)
	return nil
}

type cancelingDockerChecker struct {
	started chan struct{}
}

func (*cancelingDockerChecker) ListDockerServerIDs(context.Context) ([]string, error) {
	return []string{"docker-1"}, nil
}

func (checker *cancelingDockerChecker) CheckStatus(ctx context.Context, _ string) error {
	close(checker.started)
	<-ctx.Done()
	return ctx.Err()
}

type failingDockerChecker struct{}

func (failingDockerChecker) ListDockerServerIDs(context.Context) ([]string, error) {
	return []string{"docker-1"}, nil
}

func (failingDockerChecker) CheckStatus(context.Context, string) error {
	return errors.New("private Docker SDK detail")
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

func serveAuthenticatedRequest(t *testing.T, application *Application, method, target string, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	authStore := auth.NewStore(application.store.SQLDatabase())
	user, err := authStore.CreateUser(context.Background(), "operator@example.com", "Operator", "test-hash", "admin")
	if err != nil {
		t.Fatalf("create authenticated user: %v", err)
	}
	session, _, err := authStore.CreateSession(context.Background(), user.ID, time.Now())
	if err != nil {
		t.Fatalf("create authenticated session: %v", err)
	}
	request := httptest.NewRequest(method, target, nil)
	request.AddCookie(&http.Cookie{Name: "gopanel_session_dev", Value: session})
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

type testSessionCleaner struct {
	calls chan struct{}
	err   error
}

func (cleaner *testSessionCleaner) CleanupExpired(context.Context) (int64, error) {
	if cleaner.calls != nil {
		select {
		case cleaner.calls <- struct{}{}:
		default:
		}
	}
	return 0, cleaner.err
}
