package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/irgordon/gopanel/internal/diagnostic"
)

func TestLoadConfigRejectsUnsafeDockerSocketBeforeApplicationPreparation(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	runtime := process{logger: logger, diagnostics: diagnostic.NewRecorder(logger)}

	_, err := runtime.loadConfig([]string{"--dev", "--docker-socket", "tcp://127.0.0.1:2375"})
	if err == nil || !strings.Contains(err.Error(), "Docker configuration is unusable") {
		t.Fatalf("expected Docker configuration rejection, got %v", err)
	}
	if !strings.Contains(output.String(), "event=docker_configuration_rejected") || strings.Contains(output.String(), "127.0.0.1:2375") {
		t.Fatalf("unsafe Docker configuration was not safely recorded: %q", output.String())
	}
}
