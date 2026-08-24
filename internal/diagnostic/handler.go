package diagnostic

import (
	"bytes"
	"context"
	"fmt"
	"html"
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
		h.render(w, r, http.StatusOK, diagnosticpages.ErrorListFragment(display))
		return
	}
	h.render(w, r, http.StatusOK, diagnosticpages.ErrorListPage(display))
}

func (h *Handler) HandleDetail(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	id := chi.URLParam(r, "id")
	record, found := h.recorder.Lookup(id)
	if !found {
		h.logger.Info("security", "event", "error_panel_access", "user_id", user.ID, "action", "view_missing_error")
		if isHTMXRequest(r) {
			h.render(w, r, http.StatusNotFound, diagnosticpages.ErrorNotFoundFragment(id))
			return
		}
		h.render(w, r, http.StatusNotFound, diagnosticpages.ErrorNotFoundPage(id))
		return
	}
	h.logger.Info("security", "event", "error_panel_access", "user_id", user.ID, "action", "view_error", "error_ref", record.ID)
	display := toDisplayRecord(record)
	if isHTMXRequest(r) {
		h.render(w, r, http.StatusOK, diagnosticpages.ErrorDetailFragment(display))
		return
	}
	h.render(w, r, http.StatusOK, diagnosticpages.ErrorDetailPage(display))
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

func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	body, err := renderComponent(r.Context(), component)
	if err != nil {
		reference := h.recordRenderFailure(r, err)
		h.writeRenderFailure(w, r, reference)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		h.recorder.Record(Input{
			Event:           "error_panel_response_write_failed",
			Component:       "presentation",
			PublicMessage:   "The Error Log response could not be completed.",
			TechnicalDetail: "diagnostic response write failed",
			UserID:          currentUserID(r),
			Action:          r.URL.Path,
			HTTPStatus:      status,
		})
	}
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

func (h *Handler) recordRenderFailure(r *http.Request, err error) string {
	record := h.recorder.Record(Input{
		Event:           "error_panel_render_failed",
		Component:       "presentation",
		PublicMessage:   "The Error Log page could not be rendered.",
		TechnicalDetail: fmt.Sprintf("diagnostic render failed: error_type=%T", err),
		UserID:          currentUserID(r),
		Action:          r.URL.Path,
		HTTPStatus:      http.StatusInternalServerError,
	})
	return record.ID
}

func (h *Handler) writeRenderFailure(w http.ResponseWriter, r *http.Request, reference string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	message := `The page could not be rendered. Try again. Error reference: <code>` + html.EscapeString(reference) + `</code>.`
	if isHTMXRequest(r) {
		if _, err := w.Write([]byte(`<div role="alert" data-request-region="true">` + message + `</div>`)); err != nil {
			h.logger.Error("diagnostic response write failed", "error_ref", reference)
		}
		return
	}
	if _, err := w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>GoPanel error</title></head><body><main><h1>GoPanel could not render the Error Log</h1><p>` + message + `</p></main></body></html>`)); err != nil {
		h.logger.Error("diagnostic response write failed", "error_ref", reference)
	}
}

func currentUserID(r *http.Request) string {
	user, _ := auth.UserFromContext(r.Context())
	return user.ID
}
