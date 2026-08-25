package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/config"
	"github.com/irgordon/gopanel/internal/container"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/server"
	"github.com/irgordon/gopanel/internal/store"
	webassets "github.com/irgordon/gopanel/web"
)

const (
	readHeaderTimeout      = 5 * time.Second
	readTimeout            = 15 * time.Second
	writeTimeout           = 15 * time.Second
	idleTimeout            = 60 * time.Second
	maximumHeaderBytes     = 16 * 1024
	shutdownTimeout        = 5 * time.Second
	sessionCleanupInterval = 15 * time.Minute
	dockerPollInterval     = 30 * time.Second
	dockerPollConcurrency  = 6
)

type SessionCleaner interface {
	CleanupExpired(context.Context) (int64, error)
}

type DockerStatusChecker interface {
	ListDockerServerIDs(context.Context) ([]string, error)
	CheckStatus(context.Context, string) error
}

type DockerClientCloser interface {
	Close() error
}

type lifecycleTask struct {
	stop context.CancelFunc
	done <-chan struct{}
}

type Application struct {
	server            *http.Server
	store             *store.Store
	logger            *slog.Logger
	diagnostics       *diagnostic.Recorder
	authHandler       *auth.Handler
	diagnosticHandler *diagnostic.Handler
	serverHandler     *server.Handler
	containerHandler  *container.Handler
	sessionCleaner    SessionCleaner
	dockerChecker     DockerStatusChecker
	dockerStatuses    *container.StatusCache
	dockerCloser      DockerClientCloser
	shuttingDown      atomic.Bool
	drainTimeout      time.Duration
	cleanupInterval   time.Duration
	dockerInterval    time.Duration
	dockerConcurrency int
	clock             func() time.Time
	listeningSignal   chan struct{}
}

func New(applicationConfig config.Config, database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler, diagnosticHandler *diagnostic.Handler, serverHandler *server.Handler, containerHandler *container.Handler, sessionCleaner SessionCleaner, dockerChecker DockerStatusChecker, dockerStatuses *container.StatusCache, dockerCloser DockerClientCloser) (*Application, error) {
	if err := validateDependencies(applicationConfig, database, logger, diagnostics, authHandler, diagnosticHandler, serverHandler, containerHandler, sessionCleaner, dockerChecker, dockerStatuses, dockerCloser); err != nil {
		return nil, err
	}
	staticFiles, err := webassets.StaticFiles()
	if err != nil {
		return nil, err
	}
	application := newApplication(database, logger, diagnostics, authHandler, diagnosticHandler, serverHandler, containerHandler, sessionCleaner, dockerChecker, dockerStatuses, dockerCloser)
	application.server = newHTTPServer(applicationConfig.ListenAddress, application.routes(staticFiles))
	return application, nil
}

func (application *Application) Handler() http.Handler { return application.server.Handler }

func (application *Application) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", application.server.Addr)
	if err != nil {
		return application.finish(fmt.Errorf("listen on configured address: %w", err))
	}
	return application.runLifecycle(ctx, listener)
}

func (application *Application) runLifecycle(ctx context.Context, listener net.Listener) error {
	cleanup := application.startSessionCleanup(ctx)
	dockerPoller := application.startDockerStatusPoller(ctx)
	serveError := application.runHTTPServer(ctx, listener)
	dockerPoller.stopAndWait()
	cleanup.stopAndWait()
	return application.finish(serveError)
}

func (application *Application) startSessionCleanup(ctx context.Context) lifecycleTask {
	cleanupContext, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go application.runSessionCleanup(cleanupContext, done)
	return lifecycleTask{stop: stop, done: done}
}

func (task lifecycleTask) stopAndWait() {
	task.stop()
	<-task.done
}

func (application *Application) runHTTPServer(ctx context.Context, listener net.Listener) error {
	application.recordListening(listener.Addr().String())
	close(application.listeningSignal)
	serveErrors := make(chan error, 1)
	go application.serveHTTP(listener, serveErrors)
	select {
	case err := <-serveErrors:
		return normalizeServeError(err)
	case <-ctx.Done():
		return application.shutdownHTTPServer(serveErrors)
	}
}

func (application *Application) shutdownHTTPServer(serveErrors <-chan error) error {
	application.shuttingDown.Store(true)
	application.recordShutdownInitiated()
	shutdownContext, cancel := context.WithTimeout(context.Background(), application.drainTimeout)
	defer cancel()
	shutdownError := application.server.Shutdown(shutdownContext)
	if shutdownError == nil {
		application.recordHTTPDrainCompleted()
	} else {
		application.recordHTTPDrainFailure(shutdownError)
		shutdownError = errors.Join(shutdownError, application.server.Close())
	}
	serveError := normalizeServeError(<-serveErrors)
	return errors.Join(wrapShutdownError(shutdownError), serveError)
}

func (application *Application) finish(runError error) error {
	dockerCloseError := application.dockerCloser.Close()
	application.recordDockerClosed(dockerCloseError)
	closeError := application.store.Close()
	application.recordDatabaseClosed(closeError)
	if application.shuttingDown.Load() {
		application.recordShutdownCompleted(runError, closeError)
	}
	return errors.Join(runError, dockerCloseError, closeError)
}

func (application *Application) serveHTTP(listener net.Listener, serveErrors chan<- error) {
	serveErrors <- application.server.Serve(listener)
}

func (application *Application) routes(staticFiles fs.FS) http.Handler {
	router := newRouter()
	application.registerPublicRoutes(router, staticFiles)
	application.registerAuthenticatedRoutes(router)
	application.registerDiagnosticRoutes(router)
	application.registerServerRoutes(router)
	application.registerContainerRoutes(router)
	router.NotFound(application.handleNotFound)
	return router
}

func newRouter() chi.Router {
	router := chi.NewRouter()
	router.Use(securityHeaders)
	return router
}

func (application *Application) registerPublicRoutes(router chi.Router, staticFiles fs.FS) {
	router.Get("/healthz", application.handleHealth)
	router.Get("/readyz", application.handleReadiness)
	router.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(staticFiles)))
	application.authHandler.Routes(router)
}

func (application *Application) registerAuthenticatedRoutes(router chi.Router) {
	router.With(application.authHandler.RequireLogin).Get("/", application.handleHome)
}

func (application *Application) registerDiagnosticRoutes(router chi.Router) {
	router.Group(func(r chi.Router) {
		r.Use(application.authHandler.RequireLogin, application.authHandler.RequireAdmin)
		application.diagnosticHandler.Routes(r)
	})
}

func (application *Application) registerServerRoutes(router chi.Router) {
	router.Group(func(r chi.Router) {
		r.Use(application.authHandler.RequireLogin, application.authHandler.RequireAdmin)
		application.serverHandler.Routes(r, application.authHandler.ProtectAuthenticatedPost)
	})
}

func (application *Application) registerContainerRoutes(router chi.Router) {
	router.Group(func(r chi.Router) {
		r.Use(application.authHandler.RequireLogin, application.authHandler.RequireAdmin)
		application.containerHandler.Routes(r, application.authHandler.ProtectAuthenticatedPost)
	})
}

func newApplication(database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler, diagnosticHandler *diagnostic.Handler, serverHandler *server.Handler, containerHandler *container.Handler, sessionCleaner SessionCleaner, dockerChecker DockerStatusChecker, dockerStatuses *container.StatusCache, dockerCloser DockerClientCloser) *Application {
	return &Application{store: database, logger: logger, diagnostics: diagnostics, authHandler: authHandler, diagnosticHandler: diagnosticHandler, serverHandler: serverHandler, containerHandler: containerHandler, sessionCleaner: sessionCleaner, dockerChecker: dockerChecker, dockerStatuses: dockerStatuses, dockerCloser: dockerCloser, drainTimeout: shutdownTimeout, cleanupInterval: sessionCleanupInterval, dockerInterval: dockerPollInterval, dockerConcurrency: dockerPollConcurrency, clock: time.Now, listeningSignal: make(chan struct{})}
}

func validateDependencies(applicationConfig config.Config, database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler, diagnosticHandler *diagnostic.Handler, serverHandler *server.Handler, containerHandler *container.Handler, sessionCleaner SessionCleaner, dockerChecker DockerStatusChecker, dockerStatuses *container.StatusCache, dockerCloser DockerClientCloser) error {
	if err := applicationConfig.Validate(); err != nil {
		return fmt.Errorf("validate application configuration: %w", err)
	}
	if database == nil {
		return errors.New("SQLite store is required")
	}
	if logger == nil {
		return errors.New("logger is required")
	}
	if diagnostics == nil {
		return errors.New("diagnostic recorder is required")
	}
	if authHandler == nil {
		return errors.New("auth handler is required")
	}
	if diagnosticHandler == nil {
		return errors.New("diagnostic handler is required")
	}
	if serverHandler == nil {
		return errors.New("server handler is required")
	}
	if containerHandler == nil {
		return errors.New("container handler is required")
	}
	if sessionCleaner == nil {
		return errors.New("session cleaner is required")
	}
	if dockerChecker == nil || dockerStatuses == nil || dockerCloser == nil {
		return errors.New("Docker lifecycle dependencies are required")
	}
	return nil
}

func (application *Application) startDockerStatusPoller(ctx context.Context) lifecycleTask {
	pollContext, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go application.runDockerStatusPoller(pollContext, done)
	return lifecycleTask{stop: stop, done: done}
}

func (application *Application) runDockerStatusPoller(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	application.pollDockerStatuses(ctx)
	ticker := time.NewTicker(application.dockerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			application.pollDockerStatuses(ctx)
		}
	}
}

func (application *Application) pollDockerStatuses(ctx context.Context) {
	serverIDs, err := application.dockerChecker.ListDockerServerIDs(ctx)
	if err != nil {
		application.recordDockerStatusFailure("", "list_docker_servers", err)
		return
	}
	jobs := make(chan string)
	workerCount := min(application.dockerConcurrency, len(serverIDs))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go application.runDockerStatusWorker(ctx, jobs, &workers)
	}
	for _, serverID := range serverIDs {
		select {
		case jobs <- serverID:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func (application *Application) runDockerStatusWorker(ctx context.Context, jobs <-chan string, workers *sync.WaitGroup) {
	defer workers.Done()
	for serverID := range jobs {
		application.checkDockerStatus(ctx, serverID)
	}
}

func (application *Application) checkDockerStatus(ctx context.Context, serverID string) {
	previous := application.dockerStatuses.Get(serverID)
	application.dockerStatuses.MarkChecking(serverID)
	err := application.dockerChecker.CheckStatus(ctx, serverID)
	if ctx.Err() != nil {
		return
	}
	checkedAt := application.clock()
	if err == nil {
		application.dockerStatuses.MarkConnected(serverID, checkedAt)
		return
	}
	if previous.State == container.StatusUnavailable && previous.ErrorReference != "" {
		application.dockerStatuses.MarkUnavailable(serverID, checkedAt, previous.ErrorReference)
		return
	}
	reference := application.recordDockerStatusFailure(serverID, "check_docker_status", err)
	application.dockerStatuses.MarkUnavailable(serverID, checkedAt, reference)
}

func (application *Application) recordDockerStatusFailure(serverID, action string, err error) string {
	record := application.diagnostics.Record(diagnostic.Input{Event: action, Component: "docker", PublicMessage: "Docker status could not be checked.", TechnicalDetail: container.SafeDiagnostic(err), Action: action, Target: serverID, HTTPStatus: http.StatusServiceUnavailable})
	return record.ID
}

func (application *Application) runSessionCleanup(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(application.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			application.cleanupExpiredSessions(ctx)
		}
	}
}

func (application *Application) cleanupExpiredSessions(ctx context.Context) {
	removed, err := application.sessionCleaner.CleanupExpired(ctx)
	if err != nil {
		application.diagnostics.Record(diagnostic.Input{
			Event:           "session_cleanup_failed",
			Component:       "auth",
			PublicMessage:   "Expired sessions could not be cleaned up.",
			TechnicalDetail: "authentication session cleanup failed",
			Action:          "cleanup_expired_sessions",
			HTTPStatus:      http.StatusInternalServerError,
		})
		return
	}
	if removed > 0 {
		application.logger.Info("security", "event", "expired_sessions_removed", "count", removed)
	}
}

func newHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: readHeaderTimeout, ReadTimeout: readTimeout, WriteTimeout: writeTimeout, IdleTimeout: idleTimeout, MaxHeaderBytes: maximumHeaderBytes}
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve HTTP: %w", err)
}

func wrapShutdownError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("stop HTTP server: %w", err)
}

func (application *Application) recordListening(address string) {
	application.logger.Info("lifecycle", "event", "listening", "address", address)
}
func (application *Application) recordShutdownInitiated() {
	application.logger.Info("lifecycle", "event", "shutdown_initiated")
}
func (application *Application) recordHTTPDrainCompleted() {
	application.logger.Info("lifecycle", "event", "http_drain_completed")
}
func (application *Application) recordHTTPDrainFailure(err error) {
	event := "http_drain_failed"
	if errors.Is(err, context.DeadlineExceeded) {
		event = "http_drain_deadline_reached"
	}
	application.diagnostics.Record(diagnostic.Input{Event: event, Component: "http", PublicMessage: "GoPanel could not drain HTTP requests cleanly.", TechnicalDetail: fmt.Sprintf("HTTP drain failed: error_type=%T", err)})
}
func (application *Application) recordDatabaseClosed(err error) {
	if err == nil {
		application.logger.Info("lifecycle", "event", "database_closed")
		return
	}
	application.diagnostics.Record(diagnostic.Input{Event: "database_close_failed", Component: "sqlite", PublicMessage: "SQLite could not be closed cleanly.", TechnicalDetail: fmt.Sprintf("SQLite close failed: error_type=%T", err)})
}
func (application *Application) recordDockerClosed(err error) {
	if err == nil {
		application.logger.Info("lifecycle", "event", "docker_client_closed")
		return
	}
	application.diagnostics.Record(diagnostic.Input{Event: "docker_client_close_failed", Component: "docker", PublicMessage: "Docker client could not be closed cleanly.", TechnicalDetail: container.SafeDiagnostic(err)})
}
func (application *Application) recordShutdownCompleted(runError error, closeError error) {
	if runError == nil && closeError == nil {
		application.logger.Info("lifecycle", "event", "shutdown_completed")
		return
	}
	application.logger.Error("lifecycle", "event", "shutdown_completed", "result", "failed")
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
