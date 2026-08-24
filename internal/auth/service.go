package auth

import (
	"context"
	"errors"
	"time"
)

type Service struct {
	store   *Store
	limiter *LoginLimiter
	clock   func() time.Time
}

func NewService(store *Store, limiter *LoginLimiter, clock func() time.Time) *Service {
	return &Service{store: store, limiter: limiter, clock: clock}
}
func (service *Service) Login(ctx context.Context, email, password string) (User, string, time.Time, error) {
	if !service.limiter.AllowGlobal() {
		return User{}, "", time.Time{}, ErrRateLimited
	}
	normalized, err := NormalizeEmail(email)
	if err != nil {
		VerifyUnknownPassword(password)
		return User{}, "", time.Time{}, ErrInvalidCredentials
	}
	if !service.limiter.AllowAccount(normalized) {
		return User{}, "", time.Time{}, ErrRateLimited
	}
	user, err := service.store.FindUserByEmail(ctx, normalized)
	if errors.Is(err, ErrNotFound) {
		VerifyUnknownPassword(password)
		return User{}, "", time.Time{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, "", time.Time{}, BackendError{Operation: "user lookup", Cause: err}
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return User{}, "", time.Time{}, ErrInvalidCredentials
	}
	service.limiter.Reset(normalized)
	token, expires, err := service.store.CreateSession(ctx, user.ID, service.clock())
	if err != nil {
		return User{}, "", time.Time{}, BackendError{Operation: "session creation", Cause: err}
	}
	return user, token, expires, nil
}
func (service *Service) ChangePassword(ctx context.Context, user User, current, next, confirm string) error {
	if !VerifyPassword(user.PasswordHash, current) {
		return ErrInvalidCredentials
	}
	if next != confirm {
		return ErrPasswordMismatch
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	if err := service.store.UpdatePassword(ctx, user.ID, hash); err != nil {
		return BackendError{Operation: "password update", Cause: err}
	}
	return nil
}

func (service *Service) UserForSession(ctx context.Context, token string) (User, error) {
	user, err := service.store.FindSession(ctx, token, service.clock())
	if errors.Is(err, ErrNotFound) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, BackendError{Operation: "session lookup", Cause: err}
	}
	return user, nil
}

func (service *Service) Logout(ctx context.Context, token string) error {
	if err := service.store.DeleteSession(ctx, token); err != nil {
		return BackendError{Operation: "session deletion", Cause: err}
	}
	return nil
}

func (service *Service) CleanupExpired(ctx context.Context) (int64, error) {
	removed, err := service.store.CleanupExpired(ctx, service.clock())
	if err != nil {
		return 0, BackendError{Operation: "session cleanup", Cause: err}
	}
	return removed, nil
}
