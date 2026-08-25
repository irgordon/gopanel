package container

import "testing"

func TestValidateConfigAcceptsOnlyOwnedDockerSockets(t *testing.T) {
	for _, socket := range []string{"/var/run/docker.sock", "/run/docker.sock"} {
		t.Run(socket, func(t *testing.T) {
			validated, err := ValidateConfig(Config{SocketPath: socket})
			if err != nil {
				t.Fatalf("expected owned socket to pass: %v", err)
			}
			if validated.host() != "unix://"+socket {
				t.Fatalf("unexpected Docker host %q", validated.host())
			}
		})
	}
}

func TestValidateConfigRejectsOutboundAndFilesystemEscapeBeforeClientConstruction(t *testing.T) {
	for _, value := range []string{"", "tcp://127.0.0.1:2375", "http://metadata.internal", "unix:///tmp/docker.sock", "/tmp/docker.sock", "/var/run/other.sock", "../docker.sock"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ValidateConfig(Config{SocketPath: value}); err == nil {
				t.Fatalf("unsafe Docker configuration %q was accepted", value)
			}
		})
	}
}
