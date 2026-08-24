package server

import (
	"context"
	"errors"
	"testing"

	"github.com/irgordon/gopanel/internal/audit"
)

func TestCreateStopsWhenAuditAttemptFails(t *testing.T) {
	servers := &fakeRegistrationStore{}
	audits := &fakeRegistrationAuditStore{attemptError: errors.New("forced attempt failure")}
	service := newService(servers, audits, fixedServerID("server-123"))

	_, _, err := service.Create(t.Context(), "user-1", validInput())

	var unavailable AuditUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected AuditUnavailableError, got %v", err)
	}
	if servers.createCalled {
		t.Fatal("server creation ran after audit attempt failed")
	}
}

func TestCreateUsesRealTargetAndNormalizedInput(t *testing.T) {
	servers := &fakeRegistrationStore{}
	audits := &fakeRegistrationAuditStore{}
	service := newService(servers, audits, fixedServerID("server-123"))
	input := Input{Name: "  prod-docker  ", Address: "  10.0.0.12  ", ConnectionType: "  docker  "}

	created, auditID, err := service.Create(t.Context(), "user-1", input)

	if err != nil {
		t.Fatal(err)
	}
	if audits.targetID != "server-123" || created.ID != "server-123" || auditID != "audit-123" {
		t.Fatalf("expected real target correlation, got target=%q server=%q audit=%q", audits.targetID, created.ID, auditID)
	}
	if servers.input.Name != "prod-docker" || servers.input.Address != "10.0.0.12" || servers.input.ConnectionType != "docker" {
		t.Fatalf("expected normalized input, got %#v", servers.input)
	}
	if audits.result != audit.ResultSuccess {
		t.Fatalf("expected success finalization, got %q", audits.result)
	}
}

func TestCreateFailureFinalizesAuditAsFailed(t *testing.T) {
	servers := &fakeRegistrationStore{createError: errors.New("forced create failure")}
	audits := &fakeRegistrationAuditStore{}
	service := newService(servers, audits, fixedServerID("server-123"))

	_, auditID, err := service.Create(t.Context(), "user-1", validInput())

	var creation CreationError
	if !errors.As(err, &creation) {
		t.Fatalf("expected CreationError, got %v", err)
	}
	if auditID != "audit-123" || audits.result != audit.ResultFailed {
		t.Fatalf("expected failed audit finalization, got audit=%q result=%q", auditID, audits.result)
	}
}

func TestCreateSuccessWithAuditFailureReturnsPartialCompletion(t *testing.T) {
	servers := &fakeRegistrationStore{}
	audits := &fakeRegistrationAuditStore{resultError: errors.New("forced finalization failure")}
	service := newService(servers, audits, fixedServerID("server-123"))

	created, auditID, err := service.Create(t.Context(), "user-1", validInput())

	var incomplete AuditIncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("expected AuditIncompleteError, got %v", err)
	}
	if created.ID != "server-123" || incomplete.Server.ID != created.ID || auditID != "audit-123" {
		t.Fatalf("expected created server and audit correlation, got server=%q audit=%q", created.ID, auditID)
	}
}

type fakeRegistrationStore struct {
	createCalled bool
	createError  error
	input        ValidatedInput
}

func (store *fakeRegistrationStore) Create(_ context.Context, id string, input ValidatedInput) (Server, error) {
	store.createCalled = true
	store.input = input
	if store.createError != nil {
		return Server{}, store.createError
	}
	return Server{ID: id, Name: input.Name, Address: input.Address, ConnectionType: input.ConnectionType}, nil
}

func (store *fakeRegistrationStore) List(context.Context) ([]Server, error) { return nil, nil }
func (store *fakeRegistrationStore) Get(context.Context, string) (Server, error) {
	return Server{}, nil
}

type fakeRegistrationAuditStore struct {
	attemptError error
	resultError  error
	targetID     string
	result       string
}

func (store *fakeRegistrationAuditStore) RecordAttempt(_ context.Context, userID, action, targetType, targetID string) (audit.Record, error) {
	store.targetID = targetID
	if store.attemptError != nil {
		return audit.Record{}, store.attemptError
	}
	return audit.Record{ID: "audit-123", UserID: userID, Action: action, TargetType: targetType, TargetID: targetID, Result: audit.ResultAttempted}, nil
}

func (store *fakeRegistrationAuditStore) RecordResult(_ context.Context, _ string, result string) error {
	store.result = result
	return store.resultError
}

func fixedServerID(id string) IDGenerator {
	return func() (string, error) { return id, nil }
}

func validInput() Input {
	return Input{Name: "prod-docker", Address: "10.0.0.12", ConnectionType: "docker"}
}
