package auth

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/irgordon/gopanel/internal/view/pages/auth"
)

const (
	maxFormBytes        = 16 << 10
	sessionCookieProd   = "__Host-gopanel_session"
	sessionCookieDev    = "gopanel_session_dev"
	legacySessionCookie = "gopanel_session"
	loginCookieProd     = "__Host-gopanel_login"
	loginCookieDev      = "gopanel_login_dev"
)

type Handler struct {
	service       *Service
	csrf          *CSRF
	clock         func() time.Time
	development   bool
	origin        *http.CrossOriginProtection
	logger        *slog.Logger
	recordFailure FailureRecorder
}

type loginResponse struct {
	contextCookie string
	email         string
	session       string
	expires       time.Time
	err           error
}

type passwordResponse struct {
	user    User
	session string
	err     error
}

func NewHandler(service *Service, csrf *CSRF, clock func() time.Time, development bool, publicURL string, logger *slog.Logger, recordFailure FailureRecorder) (*Handler, error) {
	if service == nil {
		return nil, errors.New("auth service is required")
	}
	if csrf == nil {
		return nil, errors.New("CSRF protection is required")
	}
	if clock == nil {
		return nil, errors.New("auth clock is required")
	}
	origin := http.NewCrossOriginProtection()
	if err := origin.AddTrustedOrigin(strings.TrimSuffix(publicURL, "/")); err != nil {
		return nil, err
	}
	if logger == nil {
		return nil, errors.New("auth logger is required")
	}
	if recordFailure == nil {
		return nil, errors.New("auth failure recorder is required")
	}
	return &Handler{service: service, csrf: csrf, clock: clock, development: development, origin: origin, logger: logger, recordFailure: recordFailure}, nil
}
func (handler *Handler) Routes(router chi.Router) {
	router.Get("/login", handler.GetLogin)
	router.With(handler.ProtectLoginPost).Post("/login", handler.PostLogin)
	router.With(handler.RequireLogin, handler.ProtectAuthenticatedPost).Post("/logout", handler.PostLogout)
	router.With(handler.RequireLogin).Get("/account/password", handler.GetPassword)
	router.With(handler.RequireLogin, handler.ProtectAuthenticatedPost).Post("/account/password", handler.PostPassword)
}
func (handler *Handler) GetLogin(w http.ResponseWriter, r *http.Request) {
	contextValue, token, expires, err := handler.loginContext(r)
	if err != nil {
		handler.renderBackendFailure(w, r, "login_form_failed", "The sign-in form could not be prepared. Reload the page and try again.", err, "")
		return
	}
	if contextValue != "" {
		handler.setLoginCookie(w, contextValue, expires)
	}
	handler.renderLogin(w, r, http.StatusOK, token, "", "")
}
func (handler *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	handler.respondToLogin(w, r, handler.loginFromRequest(r))
}

func (handler *Handler) loginFromRequest(r *http.Request) loginResponse {
	cookie, _ := r.Cookie(handler.loginCookieName())
	contextCookie := ""
	if cookie != nil {
		contextCookie = cookie.Value
	}
	email := r.PostForm.Get("email")
	_, session, expires, err := handler.service.Login(r.Context(), email, r.PostForm.Get("password"))
	return loginResponse{contextCookie: contextCookie, email: email, session: session, expires: expires, err: err}
}

func (handler *Handler) respondToLogin(w http.ResponseWriter, r *http.Request, response loginResponse) {
	if response.err != nil {
		if message, status, rejected := loginRejection(response.err); rejected {
			fresh, tokenError := handler.csrf.LoginToken(response.contextCookie, handler.clock())
			if tokenError != nil {
				handler.renderBackendFailure(w, r, "login_form_failed", "The sign-in form could not be prepared. Reload the page and try again.", tokenError, "")
				return
			}
			handler.renderLogin(w, r, status, fresh, message, response.email)
			return
		}
		handler.renderBackendFailure(w, r, "login_failed", "GoPanel could not complete sign-in. Try again.", response.err, "")
		return
	}
	handler.clearLoginCookie(w)
	handler.clearLegacySessionCookie(w)
	handler.setSessionCookie(w, response.session, response.expires)
	handler.redirect(w, r, "/")
}
func (handler *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	handler.respondToLogout(w, r, handler.service.Logout(r.Context(), handler.sessionToken(r)))
}

func (handler *Handler) respondToLogout(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		handler.renderBackendFailure(w, r, "logout_failed", "GoPanel could not sign you out. Reload the page and try again.", err, currentUserID(r))
		return
	}
	handler.clearSessionCookie(w)
	handler.clearLegacySessionCookie(w)
	handler.redirect(w, r, "/login")
}
func (handler *Handler) GetPassword(w http.ResponseWriter, r *http.Request) {
	session := handler.sessionToken(r)
	token, err := handler.csrf.AuthToken(session)
	if err != nil {
		handler.renderBackendFailure(w, r, "password_form_failed", "The password form could not be prepared. Reload the page and try again.", err, currentUserID(r))
		return
	}
	handler.renderPassword(w, r, http.StatusOK, token, "")
}
func (handler *Handler) PostPassword(w http.ResponseWriter, r *http.Request) {
	handler.respondToPasswordChange(w, r, handler.changePasswordFromRequest(r))
}

func (handler *Handler) changePasswordFromRequest(r *http.Request) passwordResponse {
	session := handler.sessionToken(r)
	user, _ := UserFromContext(r.Context())
	err := handler.service.ChangePassword(r.Context(), user, r.PostForm.Get("current_password"), r.PostForm.Get("new_password"), r.PostForm.Get("confirm_password"))
	return passwordResponse{user: user, session: session, err: err}
}

func (handler *Handler) respondToPasswordChange(w http.ResponseWriter, r *http.Request, response passwordResponse) {
	if response.err != nil {
		if message, rejected := passwordRejection(response.err); rejected {
			token, tokenError := handler.csrf.AuthToken(response.session)
			if tokenError != nil {
				handler.renderBackendFailure(w, r, "password_form_failed", "The password form could not be prepared. Reload the page and try again.", tokenError, response.user.ID)
				return
			}
			handler.renderPassword(w, r, http.StatusUnprocessableEntity, token, message)
			return
		}
		handler.renderBackendFailure(w, r, "password_change_failed", "GoPanel could not change your password. Try again.", response.err, response.user.ID)
		return
	}
	handler.clearSessionCookie(w)
	handler.clearLegacySessionCookie(w)
	handler.redirect(w, r, "/login")
}
func (handler *Handler) csrfFailure(w http.ResponseWriter, r *http.Request, fresh bool) {
	if fresh {
		value, token, expires, err := handler.csrf.NewLoginContext(handler.clock())
		if err != nil {
			handler.renderBackendFailure(w, r, "login_form_failed", "The sign-in form could not be prepared. Reload the page and try again.", err, "")
			return
		}
		handler.setLoginCookie(w, value, expires)
		handler.renderLogin(w, r, http.StatusForbidden, token, "This form has expired or is invalid. Reload the page and try again.", r.PostForm.Get("email"))
		return
	}
	handler.renderDenied(w, r, http.StatusForbidden, "This form has expired or is invalid. Reload the page and try again.")
}
func (handler *Handler) loginContext(r *http.Request) (string, string, time.Time, error) {
	if cookie, err := r.Cookie(handler.loginCookieName()); err == nil {
		if token, err := handler.csrf.LoginToken(cookie.Value, handler.clock()); err == nil {
			return "", token, time.Time{}, nil
		}
	}
	return handler.csrf.NewLoginContext(handler.clock())
}
func (handler *Handler) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(handler.sessionCookieName())
	if err != nil {
		return ""
	}
	return cookie.Value
}
func (handler *Handler) sessionCookieName() string {
	if handler.development {
		return sessionCookieDev
	}
	return sessionCookieProd
}
func (handler *Handler) loginCookieName() string {
	if handler.development {
		return loginCookieDev
	}
	return loginCookieProd
}
func (handler *Handler) setLoginCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: handler.loginCookieName(), Value: value, Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: handler.cookieMaxAge(expires)})
}
func (handler *Handler) clearLoginCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: handler.loginCookieName(), Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (handler *Handler) setSessionCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: handler.sessionCookieName(), Value: value, Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: handler.cookieMaxAge(expires)})
}
func (handler *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: handler.sessionCookieName(), Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (handler *Handler) clearLegacySessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: legacySessionCookie, Path: "/", HttpOnly: true, Secure: !handler.development, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (handler *Handler) cookieMaxAge(expires time.Time) int {
	return int(expires.Sub(handler.clock()).Seconds())
}
func (handler *Handler) redirect(w http.ResponseWriter, r *http.Request, target string) {
	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
func (handler *Handler) renderLogin(w http.ResponseWriter, r *http.Request, status int, token, message, email string) {
	component := authpages.LoginPage(token, message, email)
	if isHTMXRequest(r) {
		component = authpages.LoginFragment(token, message, email)
	}
	handler.renderAuthComponent(w, r, status, component)
}
func (handler *Handler) renderPassword(w http.ResponseWriter, r *http.Request, status int, token, message string) {
	component := authpages.PasswordPage(token, message)
	if isHTMXRequest(r) {
		component = authpages.PasswordFragment(token, message)
	}
	handler.renderAuthComponent(w, r, status, component)
}
func (handler *Handler) renderDenied(w http.ResponseWriter, r *http.Request, status int, message string) {
	component := authpages.DeniedPage(message)
	if isHTMXRequest(r) {
		component = authpages.DeniedFragment(message)
	}
	handler.renderAuthComponent(w, r, status, component)
}

func (handler *Handler) renderAuthComponent(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	var output bytes.Buffer
	if err := component.Render(r.Context(), &output); err != nil {
		handler.renderAuthFallback(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(output.Bytes()); err != nil {
		handler.recordFailure(DiagnosticFailure{
			Event:           "auth_response_write_failed",
			PublicMessage:   "The response could not be completed.",
			TechnicalDetail: "authentication response write failed",
			UserID:          currentUserID(r),
			HTTPStatus:      status,
		})
	}
}

func (handler *Handler) AuthenticatedFormToken(r *http.Request) (string, error) {
	session, ok := SessionTokenFromContext(r.Context())
	if !ok {
		return "", ErrNotFound
	}
	return handler.csrf.AuthToken(session)
}

func loginRejection(err error) (string, int, bool) {
	if errors.Is(err, ErrInvalidCredentials) {
		return "Email or password is incorrect.", http.StatusUnprocessableEntity, true
	}
	if errors.Is(err, ErrRateLimited) {
		return "Too many sign-in attempts. Wait briefly and try again.", http.StatusTooManyRequests, true
	}
	return "", 0, false
}

func passwordRejection(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return "Current password is incorrect.", true
	case errors.Is(err, ErrPasswordMismatch):
		return "New passwords do not match.", true
	case errors.Is(err, ErrPasswordTooShort):
		return "New password must contain at least 12 characters.", true
	case errors.Is(err, ErrPasswordTooLong):
		return "New password must contain at most 1024 characters.", true
	default:
		return "", false
	}
}

func (handler *Handler) renderBackendFailure(w http.ResponseWriter, r *http.Request, event, publicMessage string, err error, userID string) {
	reference := handler.recordFailure(DiagnosticFailure{
		Event:           event,
		PublicMessage:   publicMessage,
		TechnicalDetail: safeTechnicalDetail(err),
		UserID:          userID,
		HTTPStatus:      http.StatusInternalServerError,
	})
	component := authpages.FailurePage(publicMessage, reference)
	if isHTMXRequest(r) {
		component = authpages.FailureFragment(publicMessage, reference)
	}
	handler.renderAuthComponent(w, r, http.StatusInternalServerError, component)
}

func (handler *Handler) renderAuthFallback(w http.ResponseWriter, r *http.Request, err error) {
	reference := handler.recordFailure(DiagnosticFailure{
		Event:           "auth_render_failed",
		PublicMessage:   "The page could not be rendered.",
		TechnicalDetail: safeTechnicalDetail(BackendError{Operation: "response rendering", Cause: err}),
		UserID:          currentUserID(r),
		HTTPStatus:      http.StatusInternalServerError,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprintf(w, `<div role="alert" data-request-region="true">The page could not be rendered. Error reference: <code>%s</code>.</div>`, html.EscapeString(reference))
}

func currentUserID(r *http.Request) string {
	user, _ := UserFromContext(r.Context())
	return user.ID
}

func isHTMXRequest(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }
