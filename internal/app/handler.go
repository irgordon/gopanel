package app

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/view"
)

const readinessTimeout = time.Second

func (application *Application) handleHome(w http.ResponseWriter, r *http.Request) {
	application.respondHTML(w, r, http.StatusOK, view.HomePage())
}

func (application *Application) handleHealth(w http.ResponseWriter, _ *http.Request) {
	application.writeProbeResponse(w, http.StatusOK, "alive\n")
}

func (application *Application) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if application.shuttingDown.Load() {
		application.writeProbeResponse(w, http.StatusServiceUnavailable, "not ready\n")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	if err := application.store.Ready(ctx); err != nil {
		application.writeProbeResponse(w, http.StatusServiceUnavailable, "not ready\n")
		return
	}

	application.writeProbeResponse(w, http.StatusOK, "ready\n")
}

func (application *Application) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if isHTMXRequest(r) {
		application.respondHTML(w, r, http.StatusNotFound, view.NotFoundFragment())
		return
	}

	application.respondHTML(w, r, http.StatusNotFound, view.NotFoundPage())
}

func (application *Application) respondHTML(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	body, err := renderComponent(r.Context(), component)
	if err != nil {
		record := application.recordHTTPFailure("render_response_failed", err)
		application.writeSafeRenderError(w, r, record.ID)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		application.recordHTTPFailure("write_response_failed", err)
	}
}

func (application *Application) recordHTTPFailure(event string, err error) diagnostic.Record {
	return application.diagnostics.Record(diagnostic.Input{
		Event:           event,
		Component:       "http",
		PublicMessage:   "The HTTP response could not be completed.",
		TechnicalDetail: fmt.Sprintf("error_type=%T", err),
	})
}

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func renderComponent(ctx context.Context, component templ.Component) ([]byte, error) {
	var output bytes.Buffer
	if err := component.Render(ctx, &output); err != nil {
		return nil, fmt.Errorf("render Templ component: %w", err)
	}

	return output.Bytes(), nil
}

func (application *Application) writeProbeResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		application.recordHTTPFailure("write_probe_response_failed", err)
	}
}

func (application *Application) writeSafeRenderError(w http.ResponseWriter, r *http.Request, reference string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)

	safeReference := html.EscapeString(reference)
	message := `The page could not be rendered. Try again. See Error Log. Error reference: <code>` + safeReference + `</code>.`
	if isHTMXRequest(r) {
		application.writeErrorResponse(w, `<div role="alert">`+message+`</div>`)
		return
	}

	application.writeErrorResponse(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>GoPanel error</title></head><body><main><h1>GoPanel could not render this page</h1><p>`+message+`</p></main></body></html>`)
}

func (application *Application) writeErrorResponse(w http.ResponseWriter, body string) {
	if _, err := w.Write([]byte(body)); err != nil {
		application.recordHTTPFailure("write_error_response_failed", err)
	}
}
