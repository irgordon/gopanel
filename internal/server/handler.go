package server

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/container"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/view/pages/serverpages"
)

type Handler struct {
	service      *Service
	diagnostics  *diagnostic.Recorder
	logger       *slog.Logger
	issueToken   FormTokenIssuer
	dockerStatus *container.StatusCache
}

type FormTokenIssuer func(*http.Request) (string, error)

type createResponse struct {
	userID  string
	input   Input
	server  Server
	auditID string
	err     error
}

func NewHandler(service *Service, diagnostics *diagnostic.Recorder, logger *slog.Logger, issueToken FormTokenIssuer, dockerStatus *container.StatusCache) *Handler {
	return &Handler{service: service, diagnostics: diagnostics, logger: logger, issueToken: issueToken, dockerStatus: dockerStatus}
}

func (h *Handler) Routes(router chi.Router, protectPost func(http.Handler) http.Handler) {
	router.Get("/servers", h.HandleList)
	router.Get("/servers/new", h.HandleNewForm)
	router.With(protectPost).Post("/servers", h.HandleCreate)
	router.Get("/servers/{id}", h.HandleDetail)
}

func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	servers, err := h.service.List(r.Context())
	if err != nil {
		h.renderServerError(w, r, "list_servers", err)
		return
	}
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusOK, serverpages.ServerListFragment(toDisplayServers(servers)))
		return
	}
	h.render(w, r, http.StatusOK, serverpages.ServerListPage(toDisplayServers(servers)))
}

func (h *Handler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srv, err := h.service.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		h.renderNotFound(w, r, id)
		return
	}
	if err != nil {
		h.renderServerError(w, r, "get_server", err)
		return
	}
	detail, err := h.prepareDetail(r, srv)
	if err != nil {
		h.renderServerError(w, r, "prepare_server_detail", err)
		return
	}
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusOK, serverpages.ServerDetailFragment(detail))
		return
	}
	h.render(w, r, http.StatusOK, serverpages.ServerDetailPage(detail))
}

func (h *Handler) prepareDetail(r *http.Request, srv Server) (serverpages.DetailModel, error) {
	detail := serverpages.DetailModel{Server: toDisplayServer(srv)}
	if srv.ConnectionType != "docker" {
		return detail, nil
	}
	token, err := h.issueToken(r)
	if err != nil {
		return serverpages.DetailModel{}, err
	}
	docker := container.PrepareSummaryModel(srv.ID, h.dockerStatus.Get(srv.ID), token, "")
	detail.Docker = &docker
	return detail, nil
}

func (h *Handler) HandleNewForm(w http.ResponseWriter, r *http.Request) {
	token, err := h.issueToken(r)
	if err != nil {
		h.renderServerError(w, r, "prepare_server_form", err)
		return
	}
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusOK, serverpages.ServerFormFragment(token, serverpages.DisplayInput{}, nil, ""))
		return
	}
	h.render(w, r, http.StatusOK, serverpages.ServerFormPage(token, serverpages.DisplayInput{}, nil, ""))
}

func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	h.respondToCreate(w, r, h.createFromRequest(r))
}

func (h *Handler) createFromRequest(r *http.Request) createResponse {
	user, _ := auth.UserFromContext(r.Context())
	input := readInput(r)
	created, auditID, err := h.service.Create(r.Context(), user.ID, input)
	return createResponse{userID: user.ID, input: input, server: created, auditID: auditID, err: err}
}

func readInput(r *http.Request) Input {
	return Input{
		Name:           r.PostForm.Get("name"),
		Address:        r.PostForm.Get("address"),
		ConnectionType: r.PostForm.Get("connection_type"),
	}
}

func (h *Handler) respondToCreate(w http.ResponseWriter, r *http.Request, response createResponse) {
	if response.err != nil {
		h.handleCreateFailure(w, r, response.userID, response.input, response.server, response.auditID, response.err)
		return
	}
	h.logger.Info("audit", "event", "create_server_success", "audit_id", response.auditID, "server_id", response.server.ID, "user_id", response.userID)
	if isHTMXRequest(r) {
		w.Header().Set("HX-Push-Url", "/servers/"+response.server.ID)
		detail, err := h.prepareDetail(r, response.server)
		if err != nil {
			h.renderServerError(w, r, "prepare_server_detail", err)
			return
		}
		h.render(w, r, http.StatusCreated, serverpages.ServerDetailFragment(detail))
		return
	}
	http.Redirect(w, r, "/servers/"+response.server.ID, http.StatusSeeOther)
}

func (h *Handler) handleCreateFailure(w http.ResponseWriter, r *http.Request, userID string, input Input, created Server, auditID string, err error) {
	var validation ValidationError
	if errors.As(err, &validation) {
		h.renderValidation(w, r, input, validation.Fields, r.PostForm.Get(auth.CSRFField))
		return
	}
	var incomplete AuditIncompleteError
	if errors.As(err, &incomplete) {
		h.logger.Error("audit integrity failure", "event", "create_server_audit_incomplete", "audit_id", incomplete.AuditID, "server_id", incomplete.Server.ID)
		reference := h.recordFailure(userID, incomplete.Server.ID, incomplete.AuditID, "Server was created, but its audit record is incomplete.", err)
		h.renderPartialCompletion(w, r, incomplete.Server, reference)
		return
	}
	var creation CreationError
	if errors.As(err, &creation) && creation.AuditFinalizationCause != nil {
		h.logger.Error("audit integrity failure", "event", "create_server_failure_audit_incomplete", "audit_id", creation.AuditID)
	}
	reference := h.recordFailure(userID, input.Name, auditID, "Server could not be created.", err)
	h.renderCreateError(w, r, reference)
}

func (h *Handler) recordFailure(userID, target, auditID, publicMessage string, err error) string {
	record := h.diagnostics.Record(diagnostic.Input{
		Event:              createServerAction,
		Component:          "server",
		PublicMessage:      publicMessage,
		TechnicalDetail:    SafeDiagnostic(err),
		UserID:             userID,
		Action:             createServerAction,
		Target:             target,
		HTTPStatus:         http.StatusInternalServerError,
		AuditCorrelationID: auditID,
	})
	return record.ID
}

func (h *Handler) renderCreateError(w http.ResponseWriter, r *http.Request, reference string) {
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusInternalServerError, serverpages.ServerErrorFragment(reference))
		return
	}
	h.render(w, r, http.StatusInternalServerError, serverpages.ServerErrorPage(reference))
}

func (h *Handler) renderPartialCompletion(w http.ResponseWriter, r *http.Request, server Server, reference string) {
	display := toDisplayServer(server)
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusInternalServerError, serverpages.ServerAuditIncompleteFragment(display, reference))
		return
	}
	h.render(w, r, http.StatusInternalServerError, serverpages.ServerAuditIncompletePage(display, reference))
}

func (h *Handler) renderValidation(w http.ResponseWriter, r *http.Request, input Input, errs map[string]string, token string) {
	if token == "" {
		var err error
		token, err = h.issueToken(r)
		if err != nil {
			h.renderServerError(w, r, "prepare_server_form", err)
			return
		}
	}
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusUnprocessableEntity, serverpages.ServerFormFragment(token, toDisplayInput(input), errs, ""))
		return
	}
	h.render(w, r, http.StatusUnprocessableEntity, serverpages.ServerFormPage(token, toDisplayInput(input), errs, ""))
}

func (h *Handler) renderServerError(w http.ResponseWriter, r *http.Request, action string, err error) {
	user, _ := auth.UserFromContext(r.Context())
	rec := h.diagnostics.Record(diagnostic.Input{
		Event: action, Component: "server", PublicMessage: "Server could not be loaded. Try again.",
		TechnicalDetail: SafeDiagnostic(err), UserID: user.ID, HTTPStatus: http.StatusInternalServerError,
	})
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusInternalServerError, serverpages.ServerErrorFragment(rec.ID))
		return
	}
	h.render(w, r, http.StatusInternalServerError, serverpages.ServerErrorPage(rec.ID))
}

func (h *Handler) renderNotFound(w http.ResponseWriter, r *http.Request, id string) {
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusNotFound, serverpages.ServerNotFoundFragment(id))
		return
	}
	h.render(w, r, http.StatusNotFound, serverpages.ServerNotFoundPage(id))
}

func isHTMXRequest(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

func currentUserID(r *http.Request) string {
	user, _ := auth.UserFromContext(r.Context())
	return user.ID
}

func toDisplayServer(srv Server) serverpages.DisplayServer {
	return serverpages.DisplayServer{ID: srv.ID, Name: srv.Name, Address: srv.Address, ConnectionType: srv.ConnectionType}
}

func toDisplayServers(servers []Server) []serverpages.DisplayServer {
	out := make([]serverpages.DisplayServer, len(servers))
	for i, s := range servers {
		out[i] = toDisplayServer(s)
	}
	return out
}

func toDisplayInput(input Input) serverpages.DisplayInput {
	return serverpages.DisplayInput{Name: input.Name, Address: input.Address, ConnectionType: input.ConnectionType}
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	var buf bytes.Buffer
	if err := component.Render(r.Context(), &buf); err != nil {
		h.renderFallback(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		h.diagnostics.Record(diagnostic.Input{
			Event:           "server_response_write_failed",
			Component:       "presentation",
			PublicMessage:   "The server response could not be completed.",
			TechnicalDetail: "server response write failed",
			UserID:          currentUserID(r),
			Action:          r.URL.Path,
			HTTPStatus:      http.StatusInternalServerError,
		})
	}
}

func (h *Handler) renderFallback(w http.ResponseWriter, r *http.Request, err error) {
	record := h.diagnostics.Record(diagnostic.Input{
		Event:           "server_response_render_failed",
		Component:       "presentation",
		PublicMessage:   "The server page could not be rendered.",
		TechnicalDetail: "server response render failed",
		UserID:          currentUserID(r),
		Action:          r.URL.Path,
		HTTPStatus:      http.StatusInternalServerError,
	})
	h.logger.Error("server response render failed", "error_ref", record.ID, "error_type", fmt.Sprintf("%T", err))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprintf(w, `<div role="alert" data-request-region="true">The page could not be rendered. Error reference: <code>%s</code>.</div>`, record.ID)
}
