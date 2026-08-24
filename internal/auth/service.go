package auth

import (
	"context"
	"errors"
	"time"
)

const genericLoginFailure = "Email or password is incorrect."

type Service struct {
	store   *Store
	limiter *LoginLimiter
	clock   func() time.Time
}

func NewService(store *Store, limiter *LoginLimiter, clock func() time.Time) *Service {
	return &Service{store: store, limiter: limiter, clock: clock}
}
func (service *Service) Login(ctx context.Context, email, password string) (User, string, time.Time, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		VerifyUnknownPassword(password)
		return User{}, "", time.Time{}, errors.New(genericLoginFailure)
	}
	if !service.limiter.Allow(normalized) {
		return User{}, "", time.Time{}, errors.New("Too many sign-in attempts. Wait briefly and try again.")
	}
	user, err := service.store.FindUserByEmail(ctx, normalized)
	if err != nil {
		VerifyUnknownPassword(password)
		return User{}, "", time.Time{}, errors.New(genericLoginFailure)
	}
	if !VerifyPassword(user.PasswordHash, password) {
		return User{}, "", time.Time{}, errors.New(genericLoginFailure)
	}
	service.limiter.Reset(normalized)
	token, expires, err := service.store.CreateSession(ctx, user.ID, service.clock())
	return user, token, expires, err
}
func (service *Service) ChangePassword(ctx context.Context, user User, current, next, confirm string) error {
	if !VerifyPassword(user.PasswordHash, current) {
		return errors.New(genericLoginFailure)
	}
	if next != confirm {
		return errors.New("New passwords do not match.")
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	return service.store.UpdatePassword(ctx, user.ID, hash)
}
