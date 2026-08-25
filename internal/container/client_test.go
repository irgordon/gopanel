package container

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	containerderrors "github.com/containerd/errdefs"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestClientListsPreparedContainers(t *testing.T) {
	engine := &fakeEngine{listResult: client.ContainerListResult{Items: []containertypes.Summary{{ID: fullContainerID("a"), Names: []string{"/nginx"}, Image: "nginx:1.29", State: "running", Status: "Up 3 days"}}}}
	docker := &Client{engine: engine}

	containers, err := docker.ListContainers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 1 || containers[0].Name != "nginx" || containers[0].Image != "nginx:1.29" || containers[0].State != "running" || containers[0].Status != "Up 3 days" {
		t.Fatalf("unexpected prepared containers: %#v", containers)
	}
}

func TestClientRequestsExactlyBoundedLogs(t *testing.T) {
	var multiplexed bytes.Buffer
	payload := []byte("line one\nline two\n")
	header := make([]byte, 8)
	header[0] = 1
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	multiplexed.Write(header)
	multiplexed.Write(payload)
	engine := &fakeEngine{inspectResult: client.ContainerInspectResult{Container: containertypes.InspectResponse{Config: &containertypes.Config{Tty: false}}}, logData: multiplexed.Bytes()}
	docker := &Client{engine: engine}

	logs, err := docker.ViewLogs(t.Context(), fullContainerID("b"), LogTailLines)
	if err != nil {
		t.Fatal(err)
	}
	if engine.logOptions.Tail != "100" || engine.logOptions.Follow {
		t.Fatalf("unbounded Docker log options: %#v", engine.logOptions)
	}
	if string(logs) != "line one\nline two\n" {
		t.Fatalf("unexpected decoded logs %q", logs)
	}
}

func TestClientRejectsOversizedLogResponse(t *testing.T) {
	engine := &fakeEngine{inspectResult: client.ContainerInspectResult{Container: containertypes.InspectResponse{Config: &containertypes.Config{Tty: true}}}, logData: bytes.Repeat([]byte("x"), MaxLogBytes+1)}
	docker := &Client{engine: engine}

	_, err := docker.ViewLogs(t.Context(), fullContainerID("c"), LogTailLines)
	if !errors.Is(err, ErrLogResponseTooLarge) {
		t.Fatalf("expected bounded log rejection, got %v", err)
	}
}

func TestClientEnforcesLastOneHundredLinesLocally(t *testing.T) {
	var source strings.Builder
	for line := 1; line <= 150; line++ {
		fmt.Fprintf(&source, "line-%03d\n", line)
	}
	engine := &fakeEngine{inspectResult: client.ContainerInspectResult{Container: containertypes.InspectResponse{Config: &containertypes.Config{Tty: true}}}, logData: []byte(source.String())}
	docker := &Client{engine: engine}

	logs, err := docker.ViewLogs(t.Context(), fullContainerID("d"), LogTailLines)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(logs), "\n") != LogTailLines || strings.Contains(string(logs), "line-050") || !strings.Contains(string(logs), "line-051") || !strings.Contains(string(logs), "line-150") {
		t.Fatalf("log line bound was not enforced: %q", logs)
	}
}

func TestClientMapsTypedDockerErrorsWithoutRawDetail(t *testing.T) {
	raw := errors.New("Authorization: Bearer raw-docker-token")
	engine := &fakeEngine{listError: containerderrors.ErrUnavailable.WithMessage(raw.Error())}
	docker := &Client{engine: engine}

	_, err := docker.ListContainers(t.Context())
	if err == nil || strings.Contains(err.Error(), "raw-docker-token") {
		t.Fatalf("expected safe typed client failure, got %v", err)
	}
	var failure clientFailure
	if !errors.As(err, &failure) || failure.kind != failureUnavailable {
		t.Fatalf("expected unavailable classification, got %#v", err)
	}
}

type fakeEngine struct {
	listResult    client.ContainerListResult
	listError     error
	inspectResult client.ContainerInspectResult
	inspectError  error
	logData       []byte
	logError      error
	logOptions    client.ContainerLogsOptions
	pingError     error
	closed        bool
}

func (engine *fakeEngine) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, engine.pingError
}

func (engine *fakeEngine) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	return engine.listResult, engine.listError
}

func (engine *fakeEngine) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return engine.inspectResult, engine.inspectError
}

func (engine *fakeEngine) ContainerLogs(_ context.Context, _ string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	engine.logOptions = options
	if engine.logError != nil {
		return nil, engine.logError
	}
	return io.NopCloser(bytes.NewReader(engine.logData)), nil
}

func (engine *fakeEngine) Close() error {
	engine.closed = true
	return nil
}

func fullContainerID(character string) string { return strings.Repeat(character, 64) }
