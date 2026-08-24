package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/irgordon/gopanel/internal/audit"
)

const (
	createServerAction = "create_server"
	serverTargetType   = "server"
)

var ErrNotFound = errors.New("server not found")

type registrationStore interface {
	Create(context.Context, string, ValidatedInput) (Server, error)
	List(context.Context) ([]Server, error)
	Get(context.Context, string) (Server, error)
}

type registrationAuditStore interface {
	RecordAttempt(context.Context, string, string, string, string) (audit.Record, error)
	RecordResult(context.Context, string, string) error
}

type IDGenerator func() (string, error)

type Service struct {
	servers registrationStore
	audits  registrationAuditStore
	newID   IDGenerator
}

type ValidationError struct {
	Fields map[string]string
}

func (failure ValidationError) Error() string { return "server input is invalid" }

type PreparationError struct {
	Cause error
}

func (failure PreparationError) Error() string { return "server registration could not be prepared" }
func (failure PreparationError) Unwrap() error { return failure.Cause }

type AuditUnavailableError struct {
	Cause error
}

func (failure AuditUnavailableError) Error() string {
	return "server registration audit is unavailable"
}
func (failure AuditUnavailableError) Unwrap() error { return failure.Cause }

type CreationError struct {
	AuditID                string
	Cause                  error
	AuditFinalizationCause error
}

func (failure CreationError) Error() string { return "server creation failed" }
func (failure CreationError) Unwrap() error { return failure.Cause }

type AuditIncompleteError struct {
	Server  Server
	AuditID string
	Cause   error
}

func (failure AuditIncompleteError) Error() string { return "server created with incomplete audit" }
func (failure AuditIncompleteError) Unwrap() error { return failure.Cause }

type BackendError struct {
	Operation string
	Cause     error
}

func (failure BackendError) Error() string { return "server storage operation failed" }
func (failure BackendError) Unwrap() error { return failure.Cause }

func NewService(servers *Store, audits *audit.Store) *Service {
	return newService(servers, audits, generateServerID)
}

func newService(servers registrationStore, audits registrationAuditStore, newID IDGenerator) *Service {
	return &Service{servers: servers, audits: audits, newID: newID}
}

func (service *Service) Create(ctx context.Context, userID string, input Input) (Server, string, error) {
	validated, fieldErrors := ValidateInput(input)
	if len(fieldErrors) != 0 {
		return Server{}, "", ValidationError{Fields: fieldErrors}
	}
	serverID, err := service.newID()
	if err != nil {
		return Server{}, "", PreparationError{Cause: err}
	}
	auditRecord, err := service.audits.RecordAttempt(ctx, userID, createServerAction, serverTargetType, serverID)
	if err != nil {
		return Server{}, "", AuditUnavailableError{Cause: err}
	}
	created, err := service.servers.Create(ctx, serverID, validated)
	if err != nil {
		return Server{}, auditRecord.ID, service.creationFailure(ctx, auditRecord.ID, err)
	}
	if err := service.audits.RecordResult(ctx, auditRecord.ID, audit.ResultSuccess); err != nil {
		return created, auditRecord.ID, AuditIncompleteError{Server: created, AuditID: auditRecord.ID, Cause: err}
	}
	return created, auditRecord.ID, nil
}

func (service *Service) List(ctx context.Context) ([]Server, error) {
	servers, err := service.servers.List(ctx)
	if err != nil {
		return nil, BackendError{Operation: "list", Cause: err}
	}
	return servers, nil
}

func (service *Service) Get(ctx context.Context, id string) (Server, error) {
	server, err := service.servers.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Server{}, ErrNotFound
	}
	if err != nil {
		return Server{}, BackendError{Operation: "get", Cause: err}
	}
	return server, nil
}

func (service *Service) creationFailure(ctx context.Context, auditID string, creationError error) error {
	finalizationError := service.audits.RecordResult(ctx, auditID, audit.ResultFailed)
	return CreationError{AuditID: auditID, Cause: creationError, AuditFinalizationCause: finalizationError}
}

func SafeDiagnostic(err error) string {
	var preparation PreparationError
	var unavailable AuditUnavailableError
	var creation CreationError
	var incomplete AuditIncompleteError
	var backend BackendError
	switch {
	case errors.As(err, &preparation):
		return "server identifier generation failed"
	case errors.As(err, &unavailable):
		return "audit attempt insert failed"
	case errors.As(err, &creation) && creation.AuditFinalizationCause != nil:
		return "server persistence failed; audit result update failed"
	case errors.As(err, &creation):
		return "server persistence failed"
	case errors.As(err, &incomplete):
		return "audit result update failed after server persistence"
	case errors.As(err, &backend):
		return "server " + backend.Operation + " failed"
	default:
		return "server operation failed"
	}
}

func generateServerID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
