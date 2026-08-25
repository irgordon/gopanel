package container

import (
	"context"
	"errors"
	"regexp"
	"time"
)

const dockerConnectionType = "docker"

var (
	ErrServerNotFound      = errors.New("registered server not found")
	ErrNotDockerServer     = errors.New("registered server is not a Docker server")
	ErrContainerNotFound   = errors.New("Docker container not found")
	ErrInvalidContainerID  = errors.New("Docker container identifier is invalid")
	ErrLogResponseTooLarge = errors.New("Docker log response exceeds the byte limit")
	containerIDPattern     = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type DockerReader interface {
	Ping(context.Context) error
	ListContainers(context.Context) ([]Container, error)
	ViewLogs(context.Context, string, int) ([]byte, error)
	Close() error
}

type Service struct {
	docker       DockerReader
	lookupServer ServerLookup
	listServers  ServerLister
	timeout      time.Duration
}

type BackendError struct {
	Operation string
	Cause     error
}

func (failure BackendError) Error() string { return "Docker read operation failed" }
func (failure BackendError) Unwrap() error { return failure.Cause }

func NewService(docker DockerReader, lookupServer ServerLookup, listServers ServerLister) *Service {
	return newService(docker, lookupServer, listServers, DefaultTimeout)
}

func newService(docker DockerReader, lookupServer ServerLookup, listServers ServerLister, timeout time.Duration) *Service {
	return &Service{docker: docker, lookupServer: lookupServer, listServers: listServers, timeout: timeout}
}

func (service *Service) TestConnection(ctx context.Context, serverID string) error {
	if _, err := service.GetDockerServer(ctx, serverID); err != nil {
		return err
	}
	callContext, cancel := context.WithTimeout(ctx, service.timeout)
	defer cancel()
	if err := service.docker.Ping(callContext); err != nil {
		return mapOperationError("test connection", err)
	}
	return nil
}

func (service *Service) ListContainers(ctx context.Context, serverID string) ([]Container, error) {
	if _, err := service.GetDockerServer(ctx, serverID); err != nil {
		return nil, err
	}
	callContext, cancel := context.WithTimeout(ctx, service.timeout)
	defer cancel()
	containers, err := service.docker.ListContainers(callContext)
	if err != nil {
		return nil, mapOperationError("list containers", err)
	}
	return containers, nil
}

func (service *Service) CheckStatus(ctx context.Context, serverID string) error {
	return service.TestConnection(ctx, serverID)
}

func (service *Service) ViewLogs(ctx context.Context, serverID, containerID string) ([]byte, error) {
	if _, err := service.GetDockerServer(ctx, serverID); err != nil {
		return nil, err
	}
	if !containerIDPattern.MatchString(containerID) {
		return nil, ErrInvalidContainerID
	}
	callContext, cancel := context.WithTimeout(ctx, service.timeout)
	defer cancel()
	logs, err := service.docker.ViewLogs(callContext, containerID, LogTailLines)
	if err != nil {
		var failure clientFailure
		if errors.As(err, &failure) && failure.kind == failureNotFound {
			return nil, ErrContainerNotFound
		}
		return nil, mapOperationError("view container logs", err)
	}
	return logs, nil
}

func (service *Service) ListDockerServerIDs(ctx context.Context) ([]string, error) {
	servers, err := service.listServers(ctx)
	if err != nil {
		return nil, BackendError{Operation: "list registered servers", Cause: err}
	}
	ids := make([]string, 0, len(servers))
	for _, registered := range servers {
		if registered.ConnectionType == dockerConnectionType {
			ids = append(ids, registered.ID)
		}
	}
	return ids, nil
}

func (service *Service) Close() error {
	return service.docker.Close()
}

func (service *Service) GetDockerServer(ctx context.Context, serverID string) (RegisteredServer, error) {
	registered, found, err := service.lookupServer(ctx, serverID)
	if err != nil {
		return RegisteredServer{}, BackendError{Operation: "load registered server", Cause: err}
	}
	if !found {
		return RegisteredServer{}, ErrServerNotFound
	}
	if registered.ConnectionType != dockerConnectionType {
		return RegisteredServer{}, ErrNotDockerServer
	}
	return registered, nil
}

func mapOperationError(operation string, err error) error {
	return BackendError{Operation: operation, Cause: err}
}
