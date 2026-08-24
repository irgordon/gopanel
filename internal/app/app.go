package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/config"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/server"
	"github.com/irgordon/gopanel/internal/store"
	webassets "github.com/irgordon/gopanel/web"
)

const (
	readHeaderTimeout      = 5 * time.Second
	idleTimeout            = 60 * time.Second
	shutdownTimeout        = 5 * time.Second
	sessionCleanupInterval = 15 * time.Minute
)

type SessionCleaner interface {
	CleanupExpired(context.Context) (int64, error)
}

type cleanupTask struct {
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
	sessionCleaner    SessionCleaner
	shuttingDown      atomic.Bool
	drainTimeout      time.Duration
	cleanupInterval   time.Duration
	listeningSignal   chan struct{}
}

func New(applicationConfig config.Config, database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler, diagnosticHandler *diagnostic.Handler, serverHandler *server.Handler, sessionCleaner SessionCleaner) (*Application, error) {
	if err := validateDependencies(applicationConfig, database, logger, diagnostics, authHandler, diagnosticHandler, serverHandler, sessionCleaner); err != nil {
		return nil, err
	}
	staticFiles, err := webassets.StaticFiles()
	if err != nil {
		return nil, err
	}
	application := newApplication(database, logger, diagnostics, authHandler, diagnosticHandler, serverHandler, sessionCleaner)
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
	serveError := application.runHTTPServer(ctx, listener)
	cleanup.stopAndWait()
	return application.finish(serveError)
}

func (application *Application) startSessionCleanup(ctx context.Context) cleanupTask {
	cleanupContext, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go application.runSessionCleanup(cleanupContext, done)
	return cleanupTask{stop: stop, done: done}
}

func (task cleanupTask) stopAndWait() {
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
	closeError := application.store.Close()
	application.recordDatabaseClosed(closeError)
	if application.shuttingDown.Load() {
		application.recordShutdownCompleted(runError, closeError)
	}
	return errors.Join(runError, closeError)
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

func newApplication(database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler, diagnosticHandler *diagnostic.Handler, serverHandler *server.Handler, sessionCleaner SessionCleaner) *Application {
	return &Application{store: database, logger: logger, diagnostics: diagnostics, authHandler: authHandler, diagnosticHandler: diagnosticHandler, serverHandler: serverHandler, sessionCleaner: sessionCleaner, drainTimeout: shutdownTimeout, cleanupInterval: sessionCleanupInterval, listeningSignal: make(chan struct{})}
}

func validateDependencies(applicationConfig config.Config, database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler, diagnosticHandler *diagnostic.Handler, serverHandler *server.Handler, sessionCleaner SessionCleaner) error {
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
	if sessionCleaner == nil {
		return errors.New("session cleaner is required")
	}
	return nil
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
	return &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: readHeaderTimeout, IdleTimeout: idleTimeout}
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
