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
	"github.com/irgordon/gopanel/internal/container"
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
	if _, err := container.ValidateConfig(container.Config{SocketPath: applicationConfig.DockerSocket}); err != nil {
		return config.Config{}, runtime.recordFailure("docker_configuration_rejected", "docker", "Docker configuration is unusable.", err)
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
		return runtime.closeAfterFailure(database, "migration_failed", "sqlite", "SQLite migration failed.", err)
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
	serverStore := server.NewStore(database.SQLDatabase())
	serverService := server.NewService(serverStore, audit.NewStore(database.SQLDatabase()))
	docker, err := runtime.buildDocker(applicationConfig, serverStore, authentication.handler)
	if err != nil {
		return nil, runtime.closeAfterFailure(database, "docker_client_failed", "docker", "Docker client configuration is unusable.", err)
	}
	serverHandler := server.NewHandler(serverService, runtime.diagnostics, runtime.logger, authentication.handler.AuthenticatedFormToken, docker.statuses)
	application, err := app.New(applicationConfig, database, runtime.logger, runtime.diagnostics, authentication.handler, diagnosticHandler, serverHandler, docker.handler, authentication.service, docker.service, docker.statuses, docker.service)
	if err != nil {
		closeError := docker.service.Close()
		return nil, runtime.closeAfterFailure(database, "application_construction_failed", "application", "GoPanel could not be constructed.", errors.Join(err, closeError))
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
		return authenticationDependencies{}, runtime.closeAfterFailure(database, "auth_handler_failed", "auth", "GoPanel could not create auth handler.", err)
	}
	return authenticationDependencies{handler: handler, service: service}, nil
}

func (runtime process) createCSRF(database *store.Store) (*auth.CSRF, error) {
	csrfKey, err := auth.NewCSRFKey()
	if err != nil {
		return nil, runtime.closeAfterFailure(database, "csrf_key_failed", "auth", "GoPanel could not create CSRF key.", err)
	}
	return auth.NewCSRF(csrfKey), nil
}

func (runtime process) buildDocker(applicationConfig config.Config, servers *server.Store, authHandler *auth.Handler) (dockerDependencies, error) {
	dockerClient, err := container.NewClient(container.Config{SocketPath: applicationConfig.DockerSocket})
	if err != nil {
		return dockerDependencies{}, err
	}
	statuses := container.NewStatusCache()
	service := container.NewService(dockerClient, dockerServerLookup(servers), dockerServerLister(servers))
	handler := container.NewHandler(service, statuses, runtime.diagnostics, runtime.logger, authHandler.AuthenticatedFormToken)
	return dockerDependencies{service: service, handler: handler, statuses: statuses}, nil
}

func dockerServerLookup(servers *server.Store) container.ServerLookup {
	return func(ctx context.Context, serverID string) (container.RegisteredServer, bool, error) {
		registered, err := servers.Get(ctx, serverID)
		if errors.Is(err, server.ErrNotFound) {
			return container.RegisteredServer{}, false, nil
		}
		if err != nil {
			return container.RegisteredServer{}, false, err
		}
		return toRegisteredDockerServer(registered), true, nil
	}
}

func dockerServerLister(servers *server.Store) container.ServerLister {
	return func(ctx context.Context) ([]container.RegisteredServer, error) {
		registered, err := servers.List(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]container.RegisteredServer, len(registered))
		for index, item := range registered {
			result[index] = toRegisteredDockerServer(item)
		}
		return result, nil
	}
}

func toRegisteredDockerServer(registered server.Server) container.RegisteredServer {
	return container.RegisteredServer{ID: registered.ID, Name: registered.Name, ConnectionType: registered.ConnectionType}
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
func (runtime process) closeAfterFailure(database *store.Store, event, component, message string, cause error) error {
	recordedFailure := runtime.recordFailure(event, component, message, cause)
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

type dockerDependencies struct {
	service  *container.Service
	handler  *container.Handler
	statuses *container.StatusCache
}

func (failure startupFailure) Error() string {
	return failure.message + " Error reference: " + failure.reference
}
func (failure startupFailure) Unwrap() error { return failure.cause }
