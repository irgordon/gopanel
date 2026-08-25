package container

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceAcceptsDockerServerAndRejectsOtherConnectionTypes(t *testing.T) {
	reader := &fakeDockerReader{}
	service := newService(reader, fixedServerLookup(RegisteredServer{ID: "docker-server", Name: "Docker", ConnectionType: "docker"}), emptyServerLister, time.Second)
	if err := service.TestConnection(t.Context(), "docker-server"); err != nil {
		t.Fatalf("valid Docker server failed: %v", err)
	}
	if reader.pingCalls != 1 {
		t.Fatalf("expected one Docker ping, got %d", reader.pingCalls)
	}

	reader = &fakeDockerReader{}
	service = newService(reader, fixedServerLookup(RegisteredServer{ID: "caddy-server", ConnectionType: "caddy"}), emptyServerLister, time.Second)
	if err := service.TestConnection(t.Context(), "caddy-server"); !errors.Is(err, ErrNotDockerServer) {
		t.Fatalf("expected wrong connection type rejection, got %v", err)
	}
	if reader.pingCalls != 0 {
		t.Fatal("wrong connection type reached the Docker client")
	}
}

func TestServiceReturnsLoadedAndEmptyDockerLists(t *testing.T) {
	for _, containers := range [][]Container{nil, {{ID: fullContainerID("d"), Name: "api", Image: "api:v1", State: "running", Status: "Up 1 hour"}}} {
		reader := &fakeDockerReader{containers: containers}
		service := newService(reader, dockerServerLookupForTests, emptyServerLister, time.Second)
		result, err := service.ListContainers(t.Context(), "server-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(result) != len(containers) {
			t.Fatalf("expected %d containers, got %d", len(containers), len(result))
		}
	}
}

func TestServiceBoundsEveryDockerCallWithCallerCancellation(t *testing.T) {
	reader := &fakeDockerReader{ping: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	service := newService(reader, dockerServerLookupForTests, emptyServerLister, 5*time.Millisecond)

	err := service.CheckStatus(t.Context(), "server-1")
	if err == nil || !strings.Contains(SafeDiagnostic(err), "timed out") {
		t.Fatalf("expected bounded timeout, got %v (%s)", err, SafeDiagnostic(err))
	}
}

func TestServiceRejectsInvalidContainerIDBeforeDockerCall(t *testing.T) {
	reader := &fakeDockerReader{}
	service := newService(reader, dockerServerLookupForTests, emptyServerLister, time.Second)

	_, err := service.ViewLogs(t.Context(), "server-1", "../../etc/passwd")
	if !errors.Is(err, ErrInvalidContainerID) {
		t.Fatalf("expected invalid container ID rejection, got %v", err)
	}
	if reader.logCalls != 0 {
		t.Fatal("invalid container ID reached Docker")
	}
}

func TestServiceListsOnlyRegisteredDockerServersForPolling(t *testing.T) {
	service := newService(&fakeDockerReader{}, dockerServerLookupForTests, func(context.Context) ([]RegisteredServer, error) {
		return []RegisteredServer{{ID: "docker-1", ConnectionType: "docker"}, {ID: "caddy-1", ConnectionType: "caddy"}, {ID: "docker-2", ConnectionType: "docker"}}, nil
	}, time.Second)

	ids, err := service.ListDockerServerIDs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "docker-1" || ids[1] != "docker-2" {
		t.Fatalf("unexpected Docker polling targets: %#v", ids)
	}
}

type fakeDockerReader struct {
	containers []Container
	listError  error
	ping       func(context.Context) error
	pingCalls  int
	logs       []byte
	logError   error
	logCalls   int
	tail       int
}

func (reader *fakeDockerReader) Ping(ctx context.Context) error {
	reader.pingCalls++
	if reader.ping != nil {
		return reader.ping(ctx)
	}
	return nil
}

func (reader *fakeDockerReader) ListContainers(context.Context) ([]Container, error) {
	return reader.containers, reader.listError
}

func (reader *fakeDockerReader) ViewLogs(_ context.Context, _ string, tail int) ([]byte, error) {
	reader.logCalls++
	reader.tail = tail
	return reader.logs, reader.logError
}

func (*fakeDockerReader) Close() error { return nil }

func fixedServerLookup(server RegisteredServer) ServerLookup {
	return func(context.Context, string) (RegisteredServer, bool, error) { return server, true, nil }
}

func dockerServerLookupForTests(context.Context, string) (RegisteredServer, bool, error) {
	return RegisteredServer{ID: "server-1", Name: "Docker server", ConnectionType: "docker"}, true, nil
}

func emptyServerLister(context.Context) ([]RegisteredServer, error) { return nil, nil }
