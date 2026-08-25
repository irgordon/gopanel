package container

import (
	"context"
	"errors"
)

func SafeDiagnostic(err error) string {
	if errors.Is(err, ErrLogResponseTooLarge) {
		return "Docker log response exceeded the bounded byte limit"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Docker request timed out"
	}
	var backend BackendError
	if !errors.As(err, &backend) {
		return "Docker operation failed"
	}
	var clientError clientFailure
	if !errors.As(backend.Cause, &clientError) {
		return "Docker " + backend.Operation + " failed"
	}
	switch clientError.kind {
	case failureUnavailable:
		return "Docker daemon was unavailable"
	case failureTimeout:
		return "Docker request timed out"
	case failurePermission:
		return "Docker daemon denied access"
	case failureNotFound:
		return "Docker container was not found"
	default:
		return "Docker protocol operation failed"
	}
}
