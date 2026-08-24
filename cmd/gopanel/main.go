package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/irgordon/gopanel/internal/app"
	"github.com/irgordon/gopanel/internal/audit"
	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/config"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/server"
	"github.com/irgordon/gopanel/internal/store"
)

func main() {
	logger := newLogger()
	diagnostics := diagnostic.NewRecorder(logger)
	if len(os.Args) > 1 && os.Args[1] == "user" {
		if err := runUserCommand(os.Args[2:], diagnostics); err != nil {
			recordProcessExit(logger, err)
			os.Exit(1)
		}
		return
	}
	if err := runProcess(os.Args[1:], logger, diagnostics); err != nil {
		recordProcessExit(logger, err)
		os.Exit(1)
	}
}

func runProcess(arguments []string, logger *slog.Logger, diagnostics *diagnostic.Recorder) error {
	runtime := process{logger: logger, diagnostics: diagnostics}
	return runtime.run(arguments)
}

func (runtime process) run(arguments []string) error {
	recordStartupInitiated(runtime.logger)
	applicationConfig, err := runtime.loadConfig(arguments)
	if err != nil {
		return err
	}
	ctx, stop := newProcessContext()
	defer stop()
	return runtime.runConfigured(ctx, applicationConfig)
}

func (runtime process) runConfigured(ctx context.Context, applicationConfig config.Config) error {
	application, err := runtime.prepareApplication(ctx, applicationConfig)
	if err != nil {
		return err
	}
	return runtime.serveApplication(ctx, application)
}

func (runtime process) prepareApplication(ctx context.Context, applicationConfig config.Config) (*app.Application, error) {
	database, err := runtime.openMigratedDatabase(ctx, applicationConfig.DatabasePath)
	if err != nil {
		return nil, err
	}
	return runtime.buildApplication(applicationConfig, database)
}

func (runtime process) openMigratedDatabase(ctx context.Context, databasePath string) (*store.Store, error) {
	database, err := runtime.openDatabase(ctx, databasePath)
	if err != nil {
		return nil, err
	}
	if err := runtime.migrateDatabase(ctx, database); err != nil {
		return nil, err
	}
	return database, nil
}

func (runtime process) loadConfig(arguments []string) (config.Config, error) {
	applicationConfig, err := config.Load(arguments)
	if err != nil {
		return config.Config{}, runtime.recordFailure("configuration_rejected", "config", "Configuration is unusable.", err)
	}
	return applicationConfig, nil
}

func (runtime process) openDatabase(ctx context.Context, databasePath string) (*store.Store, error) {
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		return nil, runtime.recordFailure("database_open_failed", "sqlite", "SQLite could not be opened.", err)
	}
	recordDatabaseOpened(runtime.logger, databasePath)
	return database, nil
}

func (runtime process) migrateDatabase(ctx context.Context, database *store.Store) error {
	if err := database.Migrate(ctx); err != nil {
		return runtime.closeAfterFailure(database, "migration_failed", "SQLite migration failed.", err)
	}
	recordMigrationCompleted(runtime.logger)
	return nil
}

func (runtime process) buildApplication(applicationConfig config.Config, database *store.Store) (*app.Application, error) {
	authentication, err := runtime.buildAuthentication(applicationConfig, database)
	if err != nil {
		return nil, err
	}
	diagnosticHandler := diagnostic.NewHandler(runtime.diagnostics, runtime.logger)
	serverHandler := runtime.buildServerHandler(database, authentication.handler)
	application, err := app.New(applicationConfig, database, runtime.logger, runtime.diagnostics, authentication.handler, diagnosticHandler, serverHandler, authentication.service)
	if err != nil {
		return nil, runtime.closeAfterFailure(database, "application_construction_failed", "GoPanel could not be constructed.", err)
	}
	return application, nil
}

func (runtime process) buildAuthentication(applicationConfig config.Config, database *store.Store) (authenticationDependencies, error) {
	csrf, err := runtime.createCSRF(database)
	if err != nil {
		return authenticationDependencies{}, err
	}
	authStore := auth.NewStore(database.SQLDatabase())
	service := auth.NewService(authStore, auth.NewLoginLimiter(time.Now), time.Now)
	handler, err := auth.NewHandler(service, csrf, time.Now, applicationConfig.Development, applicationConfig.PublicURL, runtime.logger, diagnostic.AuthFailureRecorder(runtime.diagnostics))
	if err != nil {
		return authenticationDependencies{}, runtime.closeAfterFailure(database, "auth_handler_failed", "GoPanel could not create auth handler.", err)
	}
	return authenticationDependencies{handler: handler, service: service}, nil
}

func (runtime process) createCSRF(database *store.Store) (*auth.CSRF, error) {
	csrfKey, err := auth.NewCSRFKey()
	if err != nil {
		return nil, runtime.closeAfterFailure(database, "csrf_key_failed", "GoPanel could not create CSRF key.", err)
	}
	return auth.NewCSRF(csrfKey), nil
}

func (runtime process) buildServerHandler(database *store.Store, authHandler *auth.Handler) *server.Handler {
	serverStore := server.NewStore(database.SQLDatabase())
	auditStore := audit.NewStore(database.SQLDatabase())
	serverService := server.NewService(serverStore, auditStore)
	return server.NewHandler(serverService, runtime.diagnostics, runtime.logger, authHandler.AuthenticatedFormToken)
}

func (runtime process) serveApplication(ctx context.Context, application *app.Application) error {
	if err := application.Run(ctx); err != nil {
		return runtime.recordFailure("application_stopped", "application", "GoPanel stopped unexpectedly.", err)
	}
	return nil
}

func newLogger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }
func recordStartupInitiated(logger *slog.Logger) {
	logger.Info("lifecycle", "event", "startup_initiated")
}
func recordDatabaseOpened(logger *slog.Logger, databasePath string) {
	logger.Info("lifecycle", "event", "database_opened", "database_path", databasePath)
}
func recordMigrationCompleted(logger *slog.Logger) {
	logger.Info("lifecycle", "event", "migration_completed")
}
func (runtime process) closeAfterFailure(database *store.Store, event string, message string, cause error) error {
	recordedFailure := runtime.recordFailure(event, "sqlite", message, cause)
	closeError := database.Close()
	if closeError == nil {
		runtime.logger.Info("lifecycle", "event", "database_closed")
		return recordedFailure
	}
	return errors.Join(recordedFailure, closeError)
}
func (runtime process) recordFailure(event string, component string, message string, cause error) error {
	record := runtime.diagnostics.Record(diagnostic.Input{Event: event, Component: component, PublicMessage: message, TechnicalDetail: fmt.Sprintf("%s failed: error_type=%T", event, cause)})
	return startupFailure{message: message, reference: record.ID, cause: cause}
}
func newProcessContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
func recordProcessExit(logger *slog.Logger, err error) {
	var failure startupFailure
	if errors.As(err, &failure) {
		logger.Error("process exit", "event", "process_exit", "error_ref", failure.reference, "message", failure.message)
		return
	}
	logger.Error("process exit", "event", "process_exit", "message", "GoPanel stopped unexpectedly.")
}

type startupFailure struct {
	message   string
	reference string
	cause     error
}
type process struct {
	logger      *slog.Logger
	diagnostics *diagnostic.Recorder
}

type authenticationDependencies struct {
	handler *auth.Handler
	service *auth.Service
}

func (failure startupFailure) Error() string {
	return failure.message + " Error reference: " + failure.reference
}
func (failure startupFailure) Unwrap() error { return failure.cause }
