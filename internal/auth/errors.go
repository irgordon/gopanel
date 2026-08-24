package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("email or password is incorrect")
	ErrRateLimited        = errors.New("too many sign-in attempts")
	ErrPasswordMismatch   = errors.New("new passwords do not match")
	ErrPasswordTooShort   = errors.New("password is too short")
	ErrPasswordTooLong    = errors.New("password is too long")
)

type BackendError struct {
	Operation string
	Cause     error
}

func (failure BackendError) Error() string { return "authentication backend failure" }
func (failure BackendError) Unwrap() error { return failure.Cause }

func safeTechnicalDetail(err error) string {
	var failure BackendError
	if errors.As(err, &failure) {
		return "authentication " + failure.Operation + " failed"
	}
	return "authentication request failed"
}
