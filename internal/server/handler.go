package server

import (
	"bytes"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/irgordon/gopanel/internal/audit"
	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/view/pages/serverpages"
)

type Handler struct {
	store       *Store
	auditDB     *sql.DB
	diagnostics *diagnostic.Recorder
	logger      *slog.Logger
	csrf        *auth.CSRF
}

func NewHandler(store *Store, auditDB *sql.DB, diagnostics *diagnostic.Recorder, logger *slog.Logger, csrf *auth.CSRF) *Handler {
	return &Handler{store: store, auditDB: auditDB, diagnostics: diagnostics, logger: logger, csrf: csrf}
}

func (h *Handler) Routes(router chi.Router) {
	router.Get("/servers", h.HandleList)
	router.Get("/servers/new", h.HandleNewForm)
	router.Post("/servers", h.HandleCreate)
	router.Get("/servers/{id}", h.HandleDetail)
}

func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	servers, err := h.store.List(r.Context())
	if err != nil {
		h.renderServerError(w, r, "list_servers", err)
		return
	}
	if isHTMXRequest(r) {
		renderServer(w, r, serverpages.ServerListFragment(toDisplayServers(servers)))
		return
	}
	renderServer(w, r, serverpages.ServerListPage(toDisplayServers(servers)))
}

func (h *Handler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srv, err := h.store.Get(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		h.renderNotFound(w, r, id)
		return
	}
	if err != nil {
		h.renderServerError(w, r, "get_server", err)
		return
	}
	if isHTMXRequest(r) {
		renderServer(w, r, serverpages.ServerDetailFragment(toDisplayServer(srv)))
		return
	}
	renderServer(w, r, serverpages.ServerDetailPage(toDisplayServer(srv)))
}

func (h *Handler) HandleNewForm(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrf.AuthToken(h.sessionToken(r))
	if isHTMXRequest(r) {
		renderServer(w, r, serverpages.ServerFormFragment(token, serverpages.DisplayInput{}, nil, ""))
		return
	}
	renderServer(w, r, serverpages.ServerFormPage(token, serverpages.DisplayInput{}, nil, ""))
}

func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderValidation(w, r, Input{}, map[string]string{"form": "The submitted form is too large or invalid."}, "")
		return
	}
	session := h.sessionToken(r)
	if !h.csrf.ValidateAuth(session, r.FormValue("csrf_token")) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<div role="alert" data-request-region="true">This form has expired or is invalid. Reload the page and try again.</div>`))
		return
	}
	user, _ := auth.UserFromContext(r.Context())
	input := Input{
		Name:                r.FormValue("name"),
		Address:             r.FormValue("address"),
		ConnectionType:      r.FormValue("connection_type"),
		CredentialReference: r.FormValue("credential_reference"),
	}
	if errs := ValidateInput(input); len(errs) > 0 {
		// 422 validation must swap cleanly under HTMX (application.js swaps 4xx text/html)
		token, _ := h.csrf.AuthToken(session)
		h.renderValidation(w, r, input, errs, token)
		return
	}
	// Audit: attempted before mutation (GP-013)
	auditRecord, err := audit.RecordAttempt(r.Context(), h.auditDB, user.ID, "create_server", "server", "pending")
	if err != nil {
		h.renderDiagnosticError(w, r, auditRecord.ID, "audit_attempt_failed", err, http.StatusInternalServerError, "Could not record audit. Try again.")
		return
	}
	// No remote contact — only persist validated configuration (Phase 3)
	srv, err := h.store.Create(r.Context(), input)
	if err != nil {
		_ = audit.RecordResult(r.Context(), h.auditDB, auditRecord.ID, audit.ResultFailed)
		// Update audit target to actual server id if created? For create failure, target remains pending
		rec := h.diagnostics.Record(diagnostic.AuditDiagnosticInput(user.ID, "create_server", input.Name, auditRecord.ID, http.StatusInternalServerError, "Server could not be created.", err.Error()))
		h.logger.Error("audit", "event", "create_server_failed", "audit_id", auditRecord.ID, "error_ref", rec.ID)
		h.renderDiagnosticError(w, r, rec.ID, "create_server", err, http.StatusInternalServerError, "Server could not be created. Try again.")
		return
	}
	// Success: update audit row to success with real target id
	if err := audit.RecordResult(r.Context(), h.auditDB, auditRecord.ID, audit.ResultSuccess); err != nil {
		rec := h.diagnostics.Record(diagnostic.AuditDiagnosticInput(user.ID, "create_server", srv.ID, auditRecord.ID, http.StatusInternalServerError, "Server created but audit update failed.", err.Error()))
		h.logger.Error("audit", "event", "audit_update_failed", "audit_id", auditRecord.ID, "error_ref", rec.ID)
		// Row remains attempted per GP-014; still show success to user but log high-severity
	}
	// Structured log correlation: audit ID is correlation ID
	h.logger.Info("audit", "event", "create_server_success", "audit_id", auditRecord.ID, "server_id", srv.ID, "user_id", user.ID)
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		renderServer(w, r, serverpages.ServerDetailFragment(toDisplayServer(srv)))
		return
	}
	http.Redirect(w, r, "/servers/"+srv.ID, http.StatusSeeOther)
}

func (h *Handler) renderValidation(w http.ResponseWriter, r *http.Request, input Input, errs map[string]string, token string) {
	if token == "" {
		token, _ = h.csrf.AuthToken(h.sessionToken(r))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if isHTMXRequest(r) {
		renderServer(w, r, serverpages.ServerFormFragment(token, toDisplayInput(input), errs, ""))
		return
	}
	renderServer(w, r, serverpages.ServerFormPage(token, toDisplayInput(input), errs, ""))
}

func (h *Handler) renderServerError(w http.ResponseWriter, r *http.Request, action string, err error) {
	user, _ := auth.UserFromContext(r.Context())
	rec := h.diagnostics.Record(diagnostic.Input{
		Event: action, Component: "server", PublicMessage: "Server could not be loaded. Try again.",
		TechnicalDetail: err.Error(), UserID: user.ID, HTTPStatus: http.StatusInternalServerError,
	})
	h.logger.Error("diagnostic", "event", action, "error_ref", rec.ID, "detail", rec.TechnicalDetail)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	renderServer(w, r, serverpages.ServerErrorFragment(rec.ID))
}

func (h *Handler) renderNotFound(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if isHTMXRequest(r) {
		renderServer(w, r, serverpages.ServerNotFoundFragment(id))
		return
	}
	renderServer(w, r, serverpages.ServerNotFoundPage(id))
}

func (h *Handler) renderDiagnosticError(w http.ResponseWriter, r *http.Request, auditID, action string, err error, status int, publicMsg string) {
	user, _ := auth.UserFromContext(r.Context())
	rec := h.diagnostics.Record(diagnostic.AuditDiagnosticInput(user.ID, action, "", auditID, status, publicMsg, err.Error()))
	h.logger.Error("diagnostic", "event", action, "error_ref", rec.ID, "audit_id", auditID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	renderServer(w, r, serverpages.ServerErrorFragment(rec.ID))
}

func (h *Handler) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie("gopanel_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func isHTMXRequest(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

func toDisplayServer(srv Server) serverpages.DisplayServer {
	return serverpages.DisplayServer{ID: srv.ID, Name: srv.Name, Address: srv.Address, ConnectionType: srv.ConnectionType, CredentialReference: srv.CredentialReference}
}

func toDisplayServers(servers []Server) []serverpages.DisplayServer {
	out := make([]serverpages.DisplayServer, len(servers))
	for i, s := range servers {
		out[i] = toDisplayServer(s)
	}
	return out
}

func toDisplayInput(input Input) serverpages.DisplayInput {
	return serverpages.DisplayInput{Name: input.Name, Address: input.Address, ConnectionType: input.ConnectionType, CredentialReference: input.CredentialReference}
}

func renderServer(w http.ResponseWriter, r *http.Request, component templ.Component) {
	var buf bytes.Buffer
	if err := component.Render(r.Context(), &buf); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<div role="alert" data-request-region="true">The page could not be rendered. Try again.</div>`))
		return
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	_, _ = w.Write(buf.Bytes())
}
