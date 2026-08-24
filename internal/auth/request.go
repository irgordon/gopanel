package auth

import "net/http"

func (handler *Handler) ProtectLoginPost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.preparePost(w, r) {
			return
		}
		cookie, err := r.Cookie(handler.loginCookieName())
		if err != nil || !handler.csrf.ValidateLogin(cookie.Value, r.PostForm.Get(CSRFField), handler.clock()) {
			handler.recordSecurityRejection("login_csrf_rejected")
			handler.csrfFailure(w, r, true)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (handler *Handler) ProtectAuthenticatedPost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !handler.preparePost(w, r) {
			return
		}
		session := handler.sessionToken(r)
		if !handler.csrf.ValidateAuth(session, r.PostForm.Get(CSRFField)) {
			handler.recordSecurityRejection("authenticated_csrf_rejected")
			handler.csrfFailure(w, r, false)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (handler *Handler) preparePost(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := handler.origin.Check(r); err != nil {
		handler.recordSecurityRejection("cross_origin_request_rejected")
		handler.renderDenied(w, r, http.StatusForbidden, "This request was blocked because it came from another site.")
		return false
	}
	if err := r.ParseForm(); err != nil {
		handler.renderDenied(w, r, http.StatusUnprocessableEntity, "The submitted form is too large or invalid. Check the form and try again.")
		return false
	}
	return true
}

func (handler *Handler) recordSecurityRejection(event string) {
	handler.logger.Warn("security", "event", event)
}
