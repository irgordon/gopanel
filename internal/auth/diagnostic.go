package auth

type DiagnosticFailure struct {
	Event           string
	PublicMessage   string
	TechnicalDetail string
	UserID          string
	HTTPStatus      int
}

type FailureRecorder func(DiagnosticFailure) string
