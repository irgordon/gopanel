package diagnostic

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/view/pages/diagnosticpages"
)

type Handler struct {
	recorder *Recorder
	logger   *slog.Logger
}

func NewHandler(recorder *Recorder, logger *slog.Logger) *Handler {
	return &Handler{recorder: recorder, logger: logger}
}

func (h *Handler) Routes(router chi.Router) {
	router.Get("/errors", h.HandleList)
	router.Get("/errors/{id}", h.HandleDetail)
}

func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	h.logger.Info("security", "event", "error_panel_access", "user_id", user.ID, "action", "list_errors")
	records := h.recorder.Snapshot()
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
	display := toDisplayRecords(records)
	if isHTMXRequest(r) {
		renderDiagnostic(w, r, diagnosticpages.ErrorListFragment(display))
		return
	}
	renderDiagnostic(w, r, diagnosticpages.ErrorListPage(display))
}

func (h *Handler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	record, found := h.recorder.Lookup(id)
	if !found {
		if isHTMXRequest(r) {
			renderDiagnostic(w, r, diagnosticpages.ErrorNotFoundFragment(id))
			return
		}
		renderDiagnostic(w, r, diagnosticpages.ErrorNotFoundPage(id))
		return
	}
	h.logger.Info("security", "event", "error_panel_access", "user_id", user.ID, "action", "view_error", "error_ref", record.ID)
	display := toDisplayRecord(record)
	if isHTMXRequest(r) {
		renderDiagnostic(w, r, diagnosticpages.ErrorDetailFragment(display))
		return
	}
	renderDiagnostic(w, r, diagnosticpages.ErrorDetailPage(display))
}

func toDisplayRecord(r Record) diagnosticpages.DisplayRecord {
	return diagnosticpages.DisplayRecord{
		ID: r.ID, CreatedAt: r.CreatedAt, Event: r.Event, Component: r.Component,
		PublicMessage: r.PublicMessage, TechnicalDetail: r.TechnicalDetail,
		UserID: r.UserID, Action: r.Action, Target: r.Target,
		HTTPStatus: r.HTTPStatus, AuditCorrelationID: r.AuditCorrelationID,
	}
}

func toDisplayRecords(records []Record) []diagnosticpages.DisplayRecord {
	out := make([]diagnosticpages.DisplayRecord, len(records))
	for i, r := range records {
		out[i] = toDisplayRecord(r)
	}
	return out
}

func renderDiagnostic(w http.ResponseWriter, r *http.Request, component templ.Component) {
	body, err := renderComponent(r.Context(), component)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<div role=\"alert\">The page could not be rendered. Try again.</div>"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func renderComponent(ctx context.Context, component templ.Component) ([]byte, error) {
	var buf bytes.Buffer
	if err := component.Render(ctx, &buf); err != nil {
		return nil, fmt.Errorf("render diagnostic component: %w", err)
	}
	return buf.Bytes(), nil
}

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
