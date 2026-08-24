package diagnosticpages

import "time"

type DisplayRecord struct {
	ID                 string
	CreatedAt          time.Time
	Event              string
	Component          string
	PublicMessage      string
	TechnicalDetail    string
	UserID             string
	Action             string
	Target             string
	HTTPStatus         int
	AuditCorrelationID string
}
