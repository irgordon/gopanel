package diagnostic

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
)

func TestMissingDiagnosticReturnsNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(NewRecorder(logger), logger)
	router := chi.NewRouter()
	handler.Routes(router)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/errors/missing", nil)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("missing diagnostic must return 404; got %d", response.Code)
	}
}

func TestDiagnosticRenderFailureHasOneSafeReference(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	recorder := NewRecorder(logger)
	handler := NewHandler(recorder, logger)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/errors", nil)
	component := templ.ComponentFunc(func(_ context.Context, _ io.Writer) error {
		return errors.New("private render detail")
	})

	handler.render(response, request, http.StatusOK, component)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
	records := recorder.Snapshot()
	if len(records) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(records))
	}
	body := response.Body.String()
	if !strings.Contains(body, records[0].ID) || strings.Contains(body, "private render detail") {
		t.Fatalf("expected safe correlated render failure, got %q", body)
	}
}
