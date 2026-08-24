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
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 5 * time.Second
)

type Application struct {
	server          *http.Server
	store           *store.Store
	logger          *slog.Logger
	diagnostics     *diagnostic.Recorder
	authHandler     *auth.Handler
	shuttingDown    atomic.Bool
	drainTimeout    time.Duration
	listeningSignal chan struct{}
}

func New(applicationConfig config.Config, database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler) (*Application, error) {
	if err := validateDependencies(applicationConfig, database, logger, diagnostics, authHandler); err != nil {
		return nil, err
	}
	staticFiles, err := webassets.StaticFiles()
	if err != nil {
		return nil, err
	}
	application := newApplication(database, logger, diagnostics, authHandler)
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
	serveError := application.runHTTPServer(ctx, listener)
	return application.finish(serveError)
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
	router := chi.NewRouter()
	router.Use(securityHeaders)
	router.Get("/healthz", application.handleHealth)
	router.Get("/readyz", application.handleReadiness)
	router.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(staticFiles)))
	router.Get("/", application.handleHome)
	// Auth foundation routes (login, logout) mounted alongside public home for Phase 2 foundation
	protected := chi.NewRouter()
	protected.NotFound(application.handleNotFound)
	application.authHandler.Routes(router, protected)
	// Phase 2A: Administrator-only Error Panel (diagnostic recorder is process-local, 200-entry)
	diagnosticHandler := diagnostic.NewHandler(application.diagnostics, application.logger)
	router.Group(func(r chi.Router) {
		r.Use(application.authHandler.RequireLogin, auth.RequireAdmin)
		diagnosticHandler.Routes(r)
	})
	// Phase 3: Server registration (privileged, audited, no remote contact)
	serverStore := server.NewStore(application.store.SQLDatabase())
	auditDB := application.store.SQLDatabase()
	// CSRF key is owned by auth handler; reuse same instance for server handler
	serverHandler := server.NewHandler(serverStore, auditDB, application.diagnostics, application.logger, application.authHandler.CSRF())
	router.Group(func(r chi.Router) {
		r.Use(application.authHandler.RequireLogin, auth.RequireAdmin)
		serverHandler.Routes(r)
	})
	router.NotFound(application.handleNotFound)
	return router
}

func newApplication(database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler) *Application {
	return &Application{store: database, logger: logger, diagnostics: diagnostics, authHandler: authHandler, drainTimeout: shutdownTimeout, listeningSignal: make(chan struct{})}
}

func validateDependencies(applicationConfig config.Config, database *store.Store, logger *slog.Logger, diagnostics *diagnostic.Recorder, authHandler *auth.Handler) error {
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
	return nil
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

func (application *Application) recordListening(address string) { application.logger.Info("lifecycle", "event", "listening", "address", address) }
func (application *Application) recordShutdownInitiated()       { application.logger.Info("lifecycle", "event", "shutdown_initiated") }
func (application *Application) recordHTTPDrainCompleted()      { application.logger.Info("lifecycle", "event", "http_drain_completed") }
func (application *Application) recordHTTPDrainFailure(err error) {
	event := "http_drain_failed"
	if errors.Is(err, context.DeadlineExceeded) {
		event = "http_drain_deadline_reached"
	}
	application.diagnostics.Record(diagnostic.Input{Event: event, Component: "http", PublicMessage: "GoPanel could not drain HTTP requests cleanly.", TechnicalDetail: err.Error()})
}
func (application *Application) recordDatabaseClosed(err error) {
	if err == nil {
		application.logger.Info("lifecycle", "event", "database_closed")
		return
	}
	application.diagnostics.Record(diagnostic.Input{Event: "database_close_failed", Component: "sqlite", PublicMessage: "SQLite could not be closed cleanly.", TechnicalDetail: err.Error()})
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
