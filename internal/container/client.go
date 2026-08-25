package container

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	containerderrors "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type failureKind string

const (
	failureUnavailable failureKind = "unavailable"
	failureTimeout     failureKind = "timeout"
	failurePermission  failureKind = "permission_denied"
	failureNotFound    failureKind = "not_found"
	failureProtocol    failureKind = "protocol"
)

type clientFailure struct {
	kind  failureKind
	cause error
}

func (failure clientFailure) Error() string { return "Docker client operation failed" }
func (failure clientFailure) Unwrap() error { return failure.cause }

type engineAPI interface {
	Ping(context.Context, client.PingOptions) (client.PingResult, error)
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	Close() error
}

type Client struct {
	engine engineAPI
}

func NewClient(config Config) (*Client, error) {
	validated, err := ValidateConfig(config)
	if err != nil {
		return nil, err
	}
	engine, err := client.New(client.WithHost(validated.host()))
	if err != nil {
		return nil, clientFailure{kind: failureProtocol, cause: err}
	}
	return &Client{engine: engine}, nil
}

func (docker *Client) Ping(ctx context.Context) error {
	_, err := docker.engine.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true})
	return classifyClientFailure(err)
}

func (docker *Client) ListContainers(ctx context.Context) ([]Container, error) {
	result, err := docker.engine.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, classifyClientFailure(err)
	}
	containers := make([]Container, len(result.Items))
	for index, item := range result.Items {
		containers[index] = prepareContainer(item)
	}
	return containers, nil
}

func (docker *Client) ViewLogs(ctx context.Context, containerID string, tail int) ([]byte, error) {
	inspection, err := docker.engine.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return nil, classifyClientFailure(err)
	}
	stream, err := docker.engine.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       decimalTail(tail),
	})
	if err != nil {
		return nil, classifyClientFailure(err)
	}
	return readLogs(stream, inspection.Container.Config != nil && inspection.Container.Config.Tty)
}

func (docker *Client) Close() error {
	return classifyClientFailure(docker.engine.Close())
}

func prepareContainer(item containertypes.Summary) Container {
	name := item.ID
	if len(item.Names) != 0 {
		name = strings.TrimPrefix(item.Names[0], "/")
	}
	return Container{ID: item.ID, Name: name, Image: item.Image, State: string(item.State), Status: item.Status}
}

func readLogs(stream io.ReadCloser, tty bool) ([]byte, error) {
	data, readError := io.ReadAll(io.LimitReader(stream, MaxLogBytes+1))
	closeError := stream.Close()
	if readError != nil {
		return nil, clientFailure{kind: failureProtocol, cause: readError}
	}
	if closeError != nil {
		return nil, clientFailure{kind: failureProtocol, cause: closeError}
	}
	if len(data) > MaxLogBytes {
		return nil, ErrLogResponseTooLarge
	}
	if tty {
		return boundLogLines([]byte(strings.ToValidUTF8(string(data), "�")), LogTailLines), nil
	}
	var output bytes.Buffer
	if _, err := stdcopy.StdCopy(&output, &output, bytes.NewReader(data)); err != nil {
		return nil, clientFailure{kind: failureProtocol, cause: err}
	}
	return boundLogLines([]byte(strings.ToValidUTF8(output.String(), "�")), LogTailLines), nil
}

func boundLogLines(data []byte, maximum int) []byte {
	endsWithNewline := bytes.HasSuffix(data, []byte("\n"))
	lines := bytes.Split(data, []byte("\n"))
	if endsWithNewline {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > maximum {
		lines = lines[len(lines)-maximum:]
	}
	bounded := bytes.Join(lines, []byte("\n"))
	if endsWithNewline {
		bounded = append(bounded, '\n')
	}
	return bounded
}

func classifyClientFailure(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded), containerderrors.IsDeadlineExceeded(err):
		return clientFailure{kind: failureTimeout, cause: err}
	case errors.Is(err, os.ErrPermission), containerderrors.IsPermissionDenied(err), containerderrors.IsUnauthorized(err):
		return clientFailure{kind: failurePermission, cause: err}
	case containerderrors.IsNotFound(err):
		return clientFailure{kind: failureNotFound, cause: err}
	case client.IsErrConnectionFailed(err), containerderrors.IsUnavailable(err):
		return clientFailure{kind: failureUnavailable, cause: err}
	default:
		return clientFailure{kind: failureProtocol, cause: err}
	}
}

func decimalTail(tail int) string {
	return strconv.Itoa(tail)
}
