package container

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/diagnostic"
	basestore "github.com/irgordon/gopanel/internal/store"
)

func TestConnectionRouteRequiresAdminAndValidBodyCSRF(t *testing.T) {
	reader := &fakeDockerReader{}
	viewer := newProtectedContainerRoutes(t, "viewer", reader)
	viewerRequest := viewer.request(http.MethodPost, "/servers/server-1/test-docker", url.Values{auth.CSRFField: {viewer.csrfToken}})
	viewerResponse := httptest.NewRecorder()
	viewer.router.ServeHTTP(viewerResponse, viewerRequest)
	if viewerResponse.Code != http.StatusForbidden || reader.pingCalls != 0 {
		t.Fatalf("viewer reached Docker test: status=%d calls=%d", viewerResponse.Code, reader.pingCalls)
	}

	admin := newProtectedContainerRoutes(t, "admin", reader)
	invalidRequest := admin.request(http.MethodPost, "/servers/server-1/test-docker?csrf_token="+url.QueryEscape(admin.csrfToken), nil)
	invalidResponse := httptest.NewRecorder()
	admin.router.ServeHTTP(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusForbidden || reader.pingCalls != 0 {
		t.Fatalf("query or missing CSRF reached Docker test: status=%d calls=%d", invalidResponse.Code, reader.pingCalls)
	}

	validRequest := admin.request(http.MethodPost, "/servers/server-1/test-docker", url.Values{auth.CSRFField: {admin.csrfToken}})
	validResponse := httptest.NewRecorder()
	admin.router.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusSeeOther || reader.pingCalls != 1 {
		t.Fatalf("valid admin Docker test failed: status=%d calls=%d", validResponse.Code, reader.pingCalls)
	}
}

func TestAnonymousCannotReadContainers(t *testing.T) {
	routes := newProtectedContainerRoutes(t, "admin", &fakeDockerReader{})
	request := httptest.NewRequest(http.MethodGet, "/servers/server-1/containers", nil)
	response := httptest.NewRecorder()

	routes.router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous container read returned %d", response.Code)
	}
}

func TestViewerCannotRetrieveContainerLogs(t *testing.T) {
	reader := &fakeDockerReader{logs: []byte("untrusted workload output")}
	routes := newProtectedContainerRoutes(t, "viewer", reader)
	request := routes.request(http.MethodGet, "/servers/server-1/containers/"+fullContainerID("e")+"/logs", nil)
	response := httptest.NewRecorder()

	routes.router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || reader.logCalls != 0 || strings.Contains(response.Body.String(), "untrusted workload output") {
		t.Fatalf("viewer log boundary failed: status=%d calls=%d body=%q", response.Code, reader.logCalls, response.Body.String())
	}
}

func TestContainerListRendersLoadedEmptyFullPageAndHTMX(t *testing.T) {
	for _, test := range []struct {
		name       string
		containers []Container
		htmx       bool
		want       string
		noDoctype  bool
	}{
		{name: "loaded full page", containers: []Container{{ID: fullContainerID("f"), Name: "nginx", Image: "nginx:1.29", State: "running", Status: "Up 3 days"}}, want: "nginx:1.29"},
		{name: "empty full page", want: "No containers are available"},
		{name: "loaded HTMX", containers: []Container{{ID: fullContainerID("a"), Name: "api", Image: "api:v1", State: "running", Status: "Up 1 hour"}}, htmx: true, want: "api:v1", noDoctype: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			routes := newProtectedContainerRoutes(t, "admin", &fakeDockerReader{containers: test.containers})
			request := routes.request(http.MethodGet, "/servers/server-1/containers", nil)
			if test.htmx {
				request.Header.Set("HX-Request", "true")
			}
			response := httptest.NewRecorder()
			routes.router.ServeHTTP(response, request)

			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("unexpected list response: status=%d body=%q", response.Code, response.Body.String())
			}
			if test.noDoctype == strings.Contains(response.Body.String(), "<!doctype html>") {
				t.Fatalf("unexpected full-page state for HTMX=%t", test.htmx)
			}
		})
	}
}

func TestDockerFailureHasOneSafeCorrelatedReference(t *testing.T) {
	rawDetail := "Authorization: Bearer raw-docker-token socket=/private/unsafe.sock"
	reader := &fakeDockerReader{listError: clientFailure{kind: failureUnavailable, cause: errors.New(rawDetail)}}
	routes := newProtectedContainerRoutes(t, "admin", reader)
	request := routes.request(http.MethodGet, "/servers/server-1/containers", nil)
	request.Header.Set("HX-Request", "true")
	response := httptest.NewRecorder()

	routes.router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected dependency failure, got %d", response.Code)
	}
	records := routes.diagnostics.Snapshot()
	if len(records) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(records))
	}
	reference := records[0].ID
	for _, surface := range []string{response.Body.String(), routes.logs.String(), records[0].TechnicalDetail} {
		if strings.Contains(surface, "raw-docker-token") || strings.Contains(surface, "/private/unsafe.sock") {
			t.Fatalf("raw Docker detail escaped: %q", surface)
		}
	}
	if !strings.Contains(response.Body.String(), reference) || !strings.Contains(routes.logs.String(), reference) {
		t.Fatalf("missing correlated reference %q", reference)
	}
}

func TestConnectionTestRendersUnavailableAndTimeoutStates(t *testing.T) {
	for _, test := range []struct {
		name string
		kind failureKind
	}{
		{name: "unavailable", kind: failureUnavailable},
		{name: "timeout", kind: failureTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &fakeDockerReader{ping: func(context.Context) error {
				return clientFailure{kind: test.kind, cause: errors.New("raw Docker SDK detail")}
			}}
			routes := newProtectedContainerRoutes(t, "admin", reader)
			request := routes.request(http.MethodPost, "/servers/server-1/test-docker", url.Values{auth.CSRFField: {routes.csrfToken}})
			request.Header.Set("HX-Request", "true")
			response := httptest.NewRecorder()

			routes.router.ServeHTTP(response, request)

			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "Docker did not respond") {
				t.Fatalf("expected visible Docker %s state, got status=%d body=%q", test.name, response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "raw Docker SDK detail") || len(routes.diagnostics.Snapshot()) != 1 {
				t.Fatal("connection-test failure leaked raw detail or lost correlation")
			}
		})
	}
}

func TestMissingContainerLogsReturnNotFoundWithoutDiagnostic(t *testing.T) {
	reader := &fakeDockerReader{logError: clientFailure{kind: failureNotFound, cause: errors.New("raw missing detail")}}
	routes := newProtectedContainerRoutes(t, "admin", reader)
	request := routes.request(http.MethodGet, "/servers/server-1/containers/"+fullContainerID("c")+"/logs", nil)
	response := httptest.NewRecorder()

	routes.router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "Container not found") {
		t.Fatalf("unexpected missing-container response: status=%d body=%q", response.Code, response.Body.String())
	}
	if len(routes.diagnostics.Snapshot()) != 0 || strings.Contains(response.Body.String(), "raw missing detail") {
		t.Fatal("expected known not-found result without backend diagnostic")
	}
}

func TestAdminLogsAreBoundedAndNeverEnterDiagnostics(t *testing.T) {
	reader := &fakeDockerReader{logs: []byte("potentially-sensitive workload output")}
	routes := newProtectedContainerRoutes(t, "admin", reader)
	request := routes.request(http.MethodGet, "/servers/server-1/containers/"+fullContainerID("b")+"/logs", nil)
	response := httptest.NewRecorder()

	routes.router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || reader.tail != LogTailLines || !strings.Contains(response.Body.String(), "potentially-sensitive workload output") {
		t.Fatalf("bounded admin logs failed: status=%d tail=%d", response.Code, reader.tail)
	}
	if len(routes.diagnostics.Snapshot()) != 0 || strings.Contains(routes.logs.String(), "potentially-sensitive workload output") {
		t.Fatal("untrusted container logs entered diagnostics or structured logs")
	}
}

type protectedContainerRoutes struct {
	router      http.Handler
	session     string
	csrfToken   string
	diagnostics *diagnostic.Recorder
	logs        *bytes.Buffer
}

func newProtectedContainerRoutes(t *testing.T, role string, reader DockerReader) protectedContainerRoutes {
	t.Helper()
	ctx := context.Background()
	database, err := basestore.Open(ctx, filepath.Join(t.TempDir(), "gopanel.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	passwordHash, err := auth.HashPassword("test-only-password")
	if err != nil {
		t.Fatal(err)
	}
	authStore := auth.NewStore(database.SQLDatabase())
	user, err := authStore.CreateUser(ctx, role+"@example.test", strings.ToUpper(role), passwordHash, role)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := authStore.CreateSession(ctx, user.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	csrf := auth.NewCSRF(bytes.Repeat([]byte{8}, 32))
	token, err := csrf.AuthToken(session)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	diagnostics := diagnostic.NewRecorder(logger)
	authService := auth.NewService(authStore, auth.NewLoginLimiter(time.Now), time.Now)
	authHandler, err := auth.NewHandler(authService, csrf, time.Now, true, "http://127.0.0.1:8080", logger, diagnostic.AuthFailureRecorder(diagnostics))
	if err != nil {
		t.Fatal(err)
	}
	service := newService(reader, dockerServerLookupForTests, emptyServerLister, time.Second)
	handler := NewHandler(service, NewStatusCache(), diagnostics, logger, authHandler.AuthenticatedFormToken)
	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(authHandler.RequireLogin, authHandler.RequireAdmin)
		handler.Routes(r, authHandler.ProtectAuthenticatedPost)
	})
	return protectedContainerRoutes{router: router, session: session, csrfToken: token, diagnostics: diagnostics, logs: &logs}
}

func (routes protectedContainerRoutes) request(method, target string, form url.Values) *http.Request {
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	request := httptest.NewRequest(method, target, body)
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	request.AddCookie(&http.Cookie{Name: "gopanel_session_dev", Value: routes.session})
	return request
}
