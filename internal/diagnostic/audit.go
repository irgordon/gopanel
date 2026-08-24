package diagnostic

// AuditDiagnosticInput builds a diagnostic Input that preserves audit
// correlation for every accepted privileged operation (GP-013, GP-014).
// Callers must always set AuditCorrelationID to the audit row ID
// (the "attempted" row) when an operation is accepted, and must create
// a diagnostic entry on both known failure and on audit-update failure
// using the SAME correlation ID. Diagnostic entries must never omit the
// correlation ID for accepted operations.
func AuditDiagnosticInput(userID, action, target, auditID string, httpStatus int, publicMessage, technicalDetail string) Input {
	return Input{
		Event:              action,
		Component:          "audit",
		PublicMessage:      publicMessage,
		TechnicalDetail:    technicalDetail,
		UserID:             userID,
		Action:             action,
		Target:             target,
		HTTPStatus:         httpStatus,
		AuditCorrelationID: auditID,
	}
}
