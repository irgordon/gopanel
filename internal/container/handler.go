package container

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/view/pages/containerpages"
)

const (
	listContainersAction = "list_containers"
	testDockerAction     = "test_docker_connection"
	viewLogsAction       = "view_container_logs"
)

type FormTokenIssuer func(*http.Request) (string, error)

type Handler struct {
	service     *Service
	statuses    *StatusCache
	diagnostics *diagnostic.Recorder
	logger      *slog.Logger
	issueToken  FormTokenIssuer
	clock       func() time.Time
}

func NewHandler(service *Service, statuses *StatusCache, diagnostics *diagnostic.Recorder, logger *slog.Logger, issueToken FormTokenIssuer) *Handler {
	return &Handler{service: service, statuses: statuses, diagnostics: diagnostics, logger: logger, issueToken: issueToken, clock: time.Now}
}

func (handler *Handler) Routes(router chi.Router, protectPost func(http.Handler) http.Handler) {
	router.Get("/servers/{id}/containers", handler.HandleList)
	router.Get("/servers/{id}/containers/{containerID}/logs", handler.HandleLogs)
	router.With(protectPost).Post("/servers/{id}/test-docker", handler.HandleTestConnection)
}

func (handler *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	registered, err := handler.service.GetDockerServer(r.Context(), serverID)
	if err != nil {
		handler.renderOperationError(w, r, serverID, listContainersAction, err)
		return
	}
	containers, err := handler.service.ListContainers(r.Context(), serverID)
	if err != nil {
		handler.renderOperationError(w, r, serverID, listContainersAction, err)
		return
	}
	handler.statuses.MarkConnected(serverID, handler.clock())
	model := containerpages.ContainerListModel{ServerID: serverID, ServerName: registered.Name, Containers: displayContainers(containers), CanViewLogs: currentUserIsAdmin(r)}
	if isHTMXRequest(r) {
		handler.render(w, r, http.StatusOK, containerpages.ContainerListMain(model))
		return
	}
	handler.render(w, r, http.StatusOK, containerpages.ContainerListPage(model))
}

func (handler *Handler) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	if err := handler.service.TestConnection(r.Context(), serverID); err != nil {
		handler.renderOperationError(w, r, serverID, testDockerAction, err)
		return
	}
	handler.statuses.MarkConnected(serverID, handler.clock())
	handler.logger.Info("security", "event", "docker_connection_test_succeeded", "server_id", serverID, "user_id", currentUserID(r))
	if !isHTMXRequest(r) {
		http.Redirect(w, r, "/servers/"+serverID, http.StatusSeeOther)
		return
	}
	model, err := handler.SummaryModel(r, serverID, "Docker connection succeeded.")
	if err != nil {
		handler.renderOperationError(w, r, serverID, testDockerAction, err)
		return
	}
	handler.render(w, r, http.StatusOK, containerpages.DockerSummary(model))
}

func (handler *Handler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	containerID := chi.URLParam(r, "containerID")
	logs, err := handler.service.ViewLogs(r.Context(), serverID, containerID)
	if err != nil {
		handler.renderOperationError(w, r, serverID, viewLogsAction, err)
		return
	}
	model := containerpages.LogsModel{ServerID: serverID, ContainerID: containerID, ContainerRef: shortContainerID(containerID), Content: string(logs)}
	handler.render(w, r, http.StatusOK, containerpages.LogsPage(model))
}

func (handler *Handler) SummaryModel(r *http.Request, serverID, successMessage string) (containerpages.DockerSummaryModel, error) {
	token, err := handler.issueToken(r)
	if err != nil {
		return containerpages.DockerSummaryModel{}, BackendError{Operation: "prepare Docker form", Cause: err}
	}
	return PrepareSummaryModel(serverID, handler.statuses.Get(serverID), token, successMessage), nil
}

func (handler *Handler) renderOperationError(w http.ResponseWriter, r *http.Request, serverID, action string, err error) {
	status, title, message, expected := operationErrorPresentation(err)
	if expected {
		handler.renderExpectedError(w, r, containerpages.ErrorModel{ServerID: serverID, Title: title, Message: message}, status)
		return
	}
	record := handler.recordFailure(r, serverID, action, status, title, err)
	handler.statuses.MarkUnavailable(serverID, handler.clock(), record.ID)
	if action == testDockerAction && isHTMXRequest(r) {
		model := PrepareSummaryModel(serverID, handler.statuses.Get(serverID), r.PostForm.Get(auth.CSRFField), "")
		handler.render(w, r, status, containerpages.DockerSummary(model))
		return
	}
	handler.renderExpectedError(w, r, containerpages.ErrorModel{ServerID: serverID, Title: title, Message: message, ErrorReference: record.ID, ShowErrorLog: currentUserIsAdmin(r)}, status)
}

func (handler *Handler) recordFailure(r *http.Request, serverID, action string, status int, publicMessage string, err error) diagnostic.Record {
	return handler.diagnostics.Record(diagnostic.Input{
		Event:           action,
		Component:       "docker",
		PublicMessage:   publicMessage,
		TechnicalDetail: SafeDiagnostic(err),
		UserID:          currentUserID(r),
		Action:          action,
		Target:          serverID,
		HTTPStatus:      status,
	})
}

func (handler *Handler) renderExpectedError(w http.ResponseWriter, r *http.Request, model containerpages.ErrorModel, status int) {
	if isHTMXRequest(r) {
		handler.render(w, r, status, containerpages.DockerErrorFragment(model))
		return
	}
	handler.render(w, r, status, containerpages.DockerErrorPage(model))
}

func (handler *Handler) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	var output bytes.Buffer
	if err := component.Render(r.Context(), &output); err != nil {
		handler.renderFallback(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(output.Bytes()); err != nil {
		handler.recordFailure(r, r.URL.Path, "docker_response_write_failed", http.StatusInternalServerError, "The Docker response could not be completed.", err)
	}
}

func (handler *Handler) renderFallback(w http.ResponseWriter, r *http.Request, err error) {
	record := handler.recordFailure(r, r.URL.Path, "docker_response_render_failed", http.StatusInternalServerError, "The Docker page could not be rendered.", err)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprintf(w, `<div role="alert" data-request-region="true">The page could not be rendered. Error reference: <code>%s</code>.</div>`, record.ID)
}

func PrepareSummaryModel(serverID string, status Status, token, successMessage string) containerpages.DockerSummaryModel {
	label, freshness := statusPresentation(status)
	return containerpages.DockerSummaryModel{ServerID: serverID, State: string(status.State), StatusLabel: label, Freshness: freshness, ErrorReference: status.ErrorReference, CSRFToken: token, SuccessMessage: successMessage}
}

func statusPresentation(status Status) (string, string) {
	switch status.State {
	case StatusChecking:
		return "Checking Docker", "The current check has not completed yet."
	case StatusConnected:
		return "Docker connected", "Checked " + status.CheckedAt.Format(time.RFC3339)
	case StatusUnavailable:
		return "Docker unavailable", "Checked " + status.CheckedAt.Format(time.RFC3339)
	default:
		return "Docker not checked", "Waiting for the first process-local status check."
	}
}

func operationErrorPresentation(err error) (int, string, string, bool) {
	switch {
	case errors.Is(err, ErrServerNotFound):
		return http.StatusNotFound, "Server not found", "No registered server matches that identifier.", true
	case errors.Is(err, ErrNotDockerServer):
		return http.StatusUnprocessableEntity, "Docker is not configured for this server", "Choose a registered server whose connection type is Docker.", true
	case errors.Is(err, ErrContainerNotFound), errors.Is(err, ErrInvalidContainerID):
		return http.StatusNotFound, "Container not found", "Docker could not find that container. Return to the container list and try again.", true
	default:
		return http.StatusServiceUnavailable, "Docker did not respond", "Try again. If the problem continues, use the error reference in the Error Log.", false
	}
}

func displayContainers(containers []Container) []containerpages.DisplayContainer {
	display := make([]containerpages.DisplayContainer, len(containers))
	for index, item := range containers {
		display[index] = containerpages.DisplayContainer{ID: item.ID, ShortID: shortContainerID(item.ID), Name: item.Name, Image: item.Image, State: item.State, Status: item.Status}
	}
	return display
}

func shortContainerID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func currentUserID(r *http.Request) string {
	user, _ := auth.UserFromContext(r.Context())
	return user.ID
}

func currentUserIsAdmin(r *http.Request) bool {
	user, _ := auth.UserFromContext(r.Context())
	return user.Role == "admin"
}

func isHTMXRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}
