package auth

import (
	"context"
	"errors"
	"net/http"
)

type contextKey struct{}
type sessionContextKey struct{}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(contextKey{}).(User)
	return user, ok
}
func SessionTokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(sessionContextKey{}).(string)
	return token, ok
}
func (handler *Handler) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := handler.sessionToken(r)
		user, err := handler.service.UserForSession(r.Context(), token)
		if errors.Is(err, ErrNotFound) {
			handler.logger.Warn("security", "event", "session_rejected")
			handler.clearSessionCookie(w)
			handler.clearLegacySessionCookie(w)
			handler.renderDenied(w, r, http.StatusUnauthorized, "Sign in is required. Please sign in with an administrator account and try again. If you do not have access, contact an administrator.")
			return
		}
		if err != nil {
			handler.renderBackendFailure(w, r, "session_lookup_failed", "GoPanel could not verify your session. Reload the page and try again.", err, "")
			return
		}
		ctx := context.WithValue(r.Context(), contextKey{}, user)
		ctx = context.WithValue(ctx, sessionContextKey{}, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (handler *Handler) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok || user.Role != "admin" {
			handler.logger.Warn("security", "event", "administrator_role_rejected", "user_id", user.ID)
			handler.renderDenied(w, r, http.StatusForbidden, "Administrator access is required. Sign in with an administrator account or contact an administrator to request access.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
