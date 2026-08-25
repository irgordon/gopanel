package container

import "errors"

const (
	defaultDockerSocket   = "/var/run/docker.sock"
	alternateDockerSocket = "/run/docker.sock"
)

type Config struct {
	SocketPath string
}

type ValidatedConfig struct {
	socketPath string
}

func ValidateConfig(config Config) (ValidatedConfig, error) {
	if config.SocketPath != defaultDockerSocket && config.SocketPath != alternateDockerSocket {
		return ValidatedConfig{}, errors.New("Docker socket must be /var/run/docker.sock or /run/docker.sock")
	}
	return ValidatedConfig{socketPath: config.SocketPath}, nil
}

func (config ValidatedConfig) host() string {
	return "unix://" + config.socketPath
}
