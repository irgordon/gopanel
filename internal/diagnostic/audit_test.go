package diagnostic

import "testing"

func TestAuditDiagnosticInputRequiresCorrelationForAcceptedPrivilegedOperation(t *testing.T) {
	input := AuditDiagnosticInput("user-123", "create_server", "srv-abc", "audit-row-123", 422, "Server name is required.", "validation failed: name empty")
	if input.AuditCorrelationID == "" {
		t.Fatal("expected AuditCorrelationID to be set for accepted privileged operation")
	}
	if input.Action != "create_server" {
		t.Fatalf("expected action create_server, got %q", input.Action)
	}
	if input.HTTPStatus != 422 {
		t.Fatalf("expected HTTPStatus 422, got %d", input.HTTPStatus)
	}
	// Ensure redaction still applies via Record
	recorder, _ := newTestRecorder()
	record := recorder.Record(input)
	if record.AuditCorrelationID != "audit-row-123" {
		t.Fatalf("expected correlation preserved, got %q", record.AuditCorrelationID)
	}
}

func TestAuditDiagnosticInputPreservesTargetAndUser(t *testing.T) {
	input := AuditDiagnosticInput("admin-1", "create_server", "gopanel-prod", "audit-999", 500, "Could not create server.", "sqlite insert failed")
	if input.Target != "gopanel-prod" || input.UserID != "admin-1" {
		t.Fatalf("expected target and user preserved")
	}
}

func TestAuditUpdateFailureDiagnosticPreservesCorrelationID(t *testing.T) {
	// Simulate Phase 3: audit update fails (row already transitioned), diagnostic must carry same audit ID
	auditID := "audit-correlation-123"
	input := AuditDiagnosticInput("admin-1", "create_server", "srv-xyz", auditID, 500, "Server created but audit update failed.", "UPDATE audit_log failed")
	if input.AuditCorrelationID != auditID {
		t.Fatalf("expected diagnostic to preserve audit correlation %q, got %q", auditID, input.AuditCorrelationID)
	}
	recorder, _ := newTestRecorder()
	record := recorder.Record(input)
	if record.AuditCorrelationID != auditID {
		t.Fatalf("expected recorded correlation %q, got %q", auditID, record.AuditCorrelationID)
	}
	if record.Action != "create_server" || record.Target != "srv-xyz" {
		t.Fatalf("expected action/target preserved")
	}
}
