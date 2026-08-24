package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDevelopmentConfig(t *testing.T) {
	applicationConfig, err := Load([]string{"--dev"})
	if err != nil {
		t.Fatalf("load development config: %v", err)
	}

	if !applicationConfig.Development {
		t.Fatal("expected development mode")
	}
	if applicationConfig.ListenAddress != developmentAddress {
		t.Fatalf("expected address %q, got %q", developmentAddress, applicationConfig.ListenAddress)
	}
	if applicationConfig.DatabasePath != developmentDatabasePath {
		t.Fatalf("expected path %q, got %q", developmentDatabasePath, applicationConfig.DatabasePath)
	}
	if applicationConfig.PublicURL != developmentPublicURL {
		t.Fatalf("expected URL %q, got %q", developmentPublicURL, applicationConfig.PublicURL)
	}
}

func TestLoadProductionConfig(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "gopanel.db")
	arguments := []string{
		"--listen-address", "0.0.0.0:8443",
		"--database-path", databasePath,
		"--public-url", "https://panel.example.com",
	}

	applicationConfig, err := Load(arguments)
	if err != nil {
		t.Fatalf("load production config: %v", err)
	}

	if applicationConfig.Development {
		t.Fatal("expected production mode")
	}
}

func TestValidateRejectsInvalidConfiguration(t *testing.T) {
	valid := Config{
		ListenAddress: "127.0.0.1:8080",
		DatabasePath:  filepath.Join(t.TempDir(), "gopanel.db"),
		PublicURL:     "http://127.0.0.1:8080",
		Development:   true,
	}

	tests := []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{name: "listen format", change: func(c *Config) { c.ListenAddress = "localhost" }, want: "host:port"},
		{name: "listen port", change: func(c *Config) { c.ListenAddress = "127.0.0.1:70000" }, want: "between 1 and 65535"},
		{name: "database path", change: func(c *Config) { c.DatabasePath = ":memory:" }, want: "filesystem path"},
		{name: "database parent", change: func(c *Config) { c.DatabasePath = filepath.Join(t.TempDir(), "missing", "gopanel.db") }, want: "parent directory"},
		{name: "public URL", change: func(c *Config) { c.PublicURL = "https://user:pass@example.com" }, want: "without credentials"},
		{name: "development listener", change: func(c *Config) { c.ListenAddress = "0.0.0.0:8080" }, want: "loopback"},
		{name: "development URL", change: func(c *Config) { c.PublicURL = "https://127.0.0.1:8080" }, want: "http on loopback"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			applicationConfig := valid
			test.change(&applicationConfig)

			err := applicationConfig.Validate()
			if err == nil {
				t.Fatal("expected invalid configuration to fail")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %q", test.want, err)
			}
		})
	}
}

func TestLoadRejectsUnknownFlag(t *testing.T) {
	_, err := Load([]string{"--unknown"})
	if err == nil {
		t.Fatal("expected unknown flag to fail")
	}
	if !strings.Contains(err.Error(), "parse process arguments") {
		t.Fatalf("expected argument context, got %q", err)
	}
}

func TestLoadRejectsPositionalArguments(t *testing.T) {
	_, err := Load([]string{"--dev", "extra"})
	if err == nil {
		t.Fatal("expected positional argument to fail")
	}
	if !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("expected positional-argument error, got %q", err)
	}
}
