package auth

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"time"
)

type contextKey struct{}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(contextKey{}).(User)
	return user, ok
}
func (handler *Handler) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := handler.sessionToken(r)
		user, err := handler.store.FindSession(r.Context(), token, handler.clock())
		if err != nil {
			handler.renderDenied(w, r, http.StatusUnauthorized, "Sign in is required. Please sign in with an administrator account and try again. If you do not have access, contact an administrator.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, user)))
	})
}
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok || user.Role != "admin" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `<div role="alert" data-request-region="true"><p class="font-semibold">Administrator access is required.</p><p class="mt-2 text-sm">The Error Log is available only to administrators. Your session does not have the administrator role, so this action was denied.</p><p class="mt-2 text-sm">Safe next steps: (a) sign in with an account that has the administrator role, or (b) contact an administrator to request access.</p><a href="/login" class="mt-4 inline-flex min-h-11 items-center rounded-xl border border-white/15 px-4 py-2 text-sm font-semibold text-white">Sign in as administrator</a><p class="mt-2 font-mono text-xs">%s</p></div>`, html.EscapeString("403 — Administrator access is required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

var _ = time.Now
