package auth

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	maxFormBytes    = 16 << 10
	sessionCookie   = "gopanel_session"
	loginCookieProd = "__Host-gopanel_login"
	loginCookieDev  = "gopanel_login_dev"
)

type Handler struct {
	store       *Store
	service     *Service
	csrf        *CSRF
	clock       func() time.Time
	development bool
	origin      *http.CrossOriginProtection
}

func NewHandler(store *Store, service *Service, csrf *CSRF, clock func() time.Time, development bool, publicURL string) (*Handler, error) {
	origin := http.NewCrossOriginProtection()
	if err := origin.AddTrustedOrigin(publicURL); err != nil {
		return nil, err
	}
	return &Handler{store: store, service: service, csrf: csrf, clock: clock, development: development, origin: origin}, nil
}
func (handler *Handler) Routes(router chi.Router, protected http.Handler) {
	router.Get("/login", handler.GetLogin)
	router.Post("/login", handler.PostLogin)
	router.With(handler.RequireLogin).Post("/logout", handler.PostLogout)
	router.With(handler.RequireLogin).Get("/account/password", handler.GetPassword)
	router.With(handler.RequireLogin).Post("/account/password", handler.PostPassword)
	// Phase 2 foundation: keep "/" public so Phase 1 home tests remain valid
	// Full protected mount will be enabled when Phase 2 integration completes:
	// router.With(handler.RequireLogin).Handle("/", protected)
	_ = protected
}
func (handler *Handler) GetLogin(w http.ResponseWriter, r *http.Request) {
	contextValue, token, expires := handler.loginContext(r)
	if contextValue != "" {
		handler.setLoginCookie(w, contextValue, expires)
	}
	handler.renderLogin(w, http.StatusOK, token, "")
}
func (handler *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	if !handler.validOrigin(w, r) || !handler.parseForm(w, r) {
		return
	}
	cookie, _ := r.Cookie(handler.loginCookieName())
	token := r.FormValue(CSRFField)
	if cookie == nil || !handler.csrf.ValidateLogin(cookie.Value, token, handler.clock()) {
		handler.csrfFailure(w, r, true)
		return
	}
	user, session, expires, err := handler.service.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
	_ = user
	if err != nil {
		fresh, _ := handler.csrf.LoginToken(cookie.Value, handler.clock())
		handler.renderLogin(w, http.StatusUnprocessableEntity, fresh, err.Error())
		return
	}
	handler.clearLoginCookie(w)
	handler.setSessionCookie(w, session, expires)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (handler *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	if !handler.validOrigin(w, r) || !handler.parseForm(w, r) {
		return
	}
	session := handler.sessionToken(r)
	if !handler.csrf.ValidateAuth(session, r.FormValue(CSRFField)) {
		handler.csrfFailure(w, r, false)
		return
	}
	_ = handler.store.DeleteSession(r.Context(), session)
	handler.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (handler *Handler) GetPassword(w http.ResponseWriter, r *http.Request) {
	session := handler.sessionToken(r)
	token, _ := handler.csrf.AuthToken(session)
	handler.renderPassword(w, http.StatusOK, token, "")
}
func (handler *Handler) PostPassword(w http.ResponseWriter, r *http.Request) {
	if !handler.validOrigin(w, r) || !handler.parseForm(w, r) {
		return
	}
	session := handler.sessionToken(r)
	if !handler.csrf.ValidateAuth(session, r.FormValue(CSRFField)) {
		handler.csrfFailure(w, r, false)
		return
	}
	user, _ := UserFromContext(r.Context())
	if err := handler.service.ChangePassword(r.Context(), user, r.FormValue("current_password"), r.FormValue("new_password"), r.FormValue("confirm_password")); err != nil {
		token, _ := handler.csrf.AuthToken(session)
		handler.renderPassword(w, http.StatusUnprocessableEntity, token, err.Error())
		return
	}
	handler.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (handler *Handler) parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		handler.renderDenied(w, r, http.StatusBadRequest, "The submitted form is too large or invalid.")
		return false
	}
	return true
}
func (handler *Handler) validOrigin(w http.ResponseWriter, r *http.Request) bool {
	if err := handler.origin.Check(r); err != nil {
		handler.renderDenied(w, r, http.StatusForbidden, "This request was blocked because it came from another site.")
		return false
	}
	return true
}
func (handler *Handler) csrfFailure(w http.ResponseWriter, r *http.Request, fresh bool) {
	if fresh {
		value, token, expires, err := handler.csrf.NewLoginContext(handler.clock())
		if err == nil {
			handler.setLoginCookie(w, value, expires)
			handler.renderLogin(w, http.StatusForbidden, token, "This form has expired or is invalid. Reload the page and try again.")
			return
		}
	}
	handler.renderDenied(w, r, http.StatusForbidden, "This form has expired or is invalid. Reload the page and try again.")
}
func (handler *Handler) loginContext(r *http.Request) (string, string, time.Time) {
	if cookie, err := r.Cookie(handler.loginCookieName()); err == nil {
		if token, err := handler.csrf.LoginToken(cookie.Value, handler.clock()); err == nil {
			return "", token, time.Time{}
		}
	}
	value, token, expires, _ := handler.csrf.NewLoginContext(handler.clock())
	return value, token, expires
}
func (handler *Handler) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}
func (handler *Handler) loginCookieName() string {
	if handler.development {
		return loginCookieDev
	}
	return loginCookieProd
}
func (handler *Handler) setLoginCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: handler.loginCookieName(), Value: value, Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}
func (handler *Handler) clearLoginCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: handler.loginCookieName(), Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (handler *Handler) setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}
func (handler *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (handler *Handler) renderLogin(w http.ResponseWriter, status int, token, message string) {
	handler.renderForm(w, status, "Sign in", token, message, `<label>Email<input name="email" type="email" autocomplete="username" required></label><p>Enter your account email.</p><label>Password<input name="password" type="password" autocomplete="current-password" required></label><p>Enter your password.</p><button type="submit">Sign in</button>`, "/login")
}
func (handler *Handler) renderPassword(w http.ResponseWriter, status int, token, message string) {
	handler.renderForm(w, status, "Change password", token, message, `<label>Current password<input name="current_password" type="password" required></label><label>New password<input name="new_password" type="password" required></label><p>Use at least 12 characters.</p><label>Confirm new password<input name="confirm_password" type="password" required></label><button type="submit">Change password</button>`, "/account/password")
}
func (handler *Handler) renderForm(w http.ResponseWriter, status int, title, token, message, fields, action string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/static/output.css"><title>%s</title></head><body class="bg-slate-950 text-slate-100"><main class="mx-auto max-w-md p-6"><h1>%s</h1><div role="alert">%s</div><form method="post" action="%s" hx-post="%s" hx-swap="outerHTML"><input type="hidden" name="csrf_token" value="%s">%s</form></main></body></html>`, html.EscapeString(title), html.EscapeString(title), html.EscapeString(message), action, action, html.EscapeString(token), fields)
}
func (handler *Handler) renderDenied(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<div role="alert">%s</div>`, html.EscapeString(message))
}

var _ = strings.TrimSpace

func (handler *Handler) CSRF() *CSRF { return handler.csrf }
