package containerpages

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestContainerResourceStatesAndResponsiveRepresentations(t *testing.T) {
	loading := renderView(t, ContainerLoadingFragment())
	if !strings.Contains(loading, "Checking containers") || !strings.Contains(loading, `role="status"`) {
		t.Fatalf("loading state is incomplete: %q", loading)
	}

	empty := renderView(t, ContainerListFragment(ContainerListModel{ServerID: "server-1"}))
	if !strings.Contains(empty, "No containers are available") || !strings.Contains(empty, "Try again") {
		t.Fatalf("empty state is incomplete: %q", empty)
	}

	loaded := renderView(t, ContainerListFragment(ContainerListModel{ServerID: "server-1", CanViewLogs: true, Containers: []DisplayContainer{{ID: strings.Repeat("a", 64), ShortID: strings.Repeat("a", 12), Name: "nginx", Image: "nginx:1.29", State: "running", Status: "Up 3 days"}}}))
	for _, expected := range []string{"md:block", "md:hidden", "nginx:1.29", "View bounded logs", "View logs"} {
		if !strings.Contains(loaded, expected) {
			t.Fatalf("loaded state missing %q", expected)
		}
	}
	for _, mutation := range []string{"Stop container", "Start container", "Restart container"} {
		if strings.Contains(loaded, mutation) {
			t.Fatalf("Phase 4 view exposed mutation %q", mutation)
		}
	}

	errorView := renderView(t, DockerErrorFragment(ErrorModel{ServerID: "server-1", Title: "Docker did not respond", Message: "Try again.", ErrorReference: "err_test", ShowErrorLog: true}))
	if !strings.Contains(errorView, `role="alert"`) || !strings.Contains(errorView, "err_test") || !strings.Contains(errorView, "See Error Log") {
		t.Fatalf("error state is incomplete: %q", errorView)
	}
}

func TestDockerSummaryHasFreshnessLabelsAndIntentionalLoadingFeedback(t *testing.T) {
	html := renderView(t, DockerSummary(DockerSummaryModel{ServerID: "server-1", State: "connected", StatusLabel: "Docker connected", Freshness: "Checked 2026-08-24T12:00:00Z", CSRFToken: "csrf"}))
	for _, expected := range []string{"Docker connected", "Checked 2026-08-24T12:00:00Z", "Test Docker connection", "Testing Docker", "Checking containers", `name="csrf_token"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("Docker summary missing %q", expected)
		}
	}
}

func TestLogsPageWarnsThatBoundedOutputIsSensitive(t *testing.T) {
	html := renderView(t, LogsPage(LogsModel{ServerID: "server-1", Content: "<script>not markup</script>"}))
	for _, expected := range []string{"Administrator only", "Last 100 lines", "untrusted, potentially sensitive", "&lt;script&gt;not markup&lt;/script&gt;"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("logs page missing %q", expected)
		}
	}
}

func renderView(t *testing.T, component interface {
	Render(context.Context, io.Writer) error
}) string {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(t.Context(), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}
