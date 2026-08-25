package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	developmentAddress      = "127.0.0.1:8080"
	developmentDatabasePath = "./gopanel.db"
	developmentPublicURL    = "http://127.0.0.1:8080"
	developmentDockerSocket = "/var/run/docker.sock"
)

type Config struct {
	ListenAddress string
	DatabasePath  string
	PublicURL     string
	DockerSocket  string
	Development   bool
}

func Load(arguments []string) (Config, error) {
	parsed, err := parseOptions(arguments)
	if err != nil {
		return Config{}, err
	}
	cfg := buildConfig(parsed)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	listenHost, err := validateListenAddress(c.ListenAddress)
	if err != nil {
		return err
	}
	if err := validateDatabasePath(c.DatabasePath); err != nil {
		return err
	}
	publicURL, err := validatePublicURL(c.PublicURL)
	if err != nil {
		return err
	}
	return validateModeRestrictions(c.Development, listenHost, publicURL)
}

type options struct {
	development   bool
	listenAddress string
	databasePath  string
	publicURL     string
	dockerSocket  string
}

func parseOptions(arguments []string) (options, error) {
	parsed := options{}
	flags := flag.NewFlagSet("gopanel", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.BoolVar(&parsed.development, "dev", false, "run the local development server")
	flags.StringVar(&parsed.listenAddress, "listen-address", "", "HTTP listen address")
	flags.StringVar(&parsed.databasePath, "database-path", "", "SQLite database path")
	flags.StringVar(&parsed.publicURL, "public-url", "", "public GoPanel URL")
	flags.StringVar(&parsed.dockerSocket, "docker-socket", "", "allowed local Docker socket")
	if err := flags.Parse(arguments); err != nil {
		return options{}, fmt.Errorf("parse process arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("unexpected positional arguments")
	}
	return parsed, nil
}

func buildConfig(parsed options) Config {
	cfg := Config{
		ListenAddress: parsed.listenAddress,
		DatabasePath:  parsed.databasePath,
		PublicURL:     parsed.publicURL,
		DockerSocket:  parsed.dockerSocket,
		Development:   parsed.development,
	}
	if cfg.Development {
		applyDevelopmentDefaults(&cfg)
	}
	return cfg
}

func applyDevelopmentDefaults(cfg *Config) {
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = developmentAddress
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = developmentDatabasePath
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = developmentPublicURL
	}
	if cfg.DockerSocket == "" {
		cfg.DockerSocket = developmentDockerSocket
	}
}

func validateListenAddress(address string) (string, error) {
	if address == "" {
		return "", errors.New("listen address is required; use --listen-address host:port")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return "", errors.New("listen address must use host:port format")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("listen address port must be between 1 and 65535")
	}
	if host == "" {
		return "", errors.New("listen address must include a host")
	}
	return host, nil
}

func validateDatabasePath(databasePath string) error {
	if databasePath == "" {
		return errors.New("database path is required; use --database-path")
	}
	if databasePath == ":memory:" || strings.HasPrefix(databasePath, "file:") || strings.Contains(databasePath, "?") {
		return errors.New("database path must be a filesystem path, not a SQLite connection string")
	}
	cleanPath := filepath.Clean(databasePath)
	if cleanPath == "." {
		return errors.New("database path must name a file")
	}
	if err := validateDatabaseParent(filepath.Dir(cleanPath)); err != nil {
		return err
	}
	return validateExistingDatabaseFile(cleanPath)
}

func validateDatabaseParent(parentPath string) error {
	info, err := os.Stat(parentPath)
	if err != nil {
		return fmt.Errorf("database parent directory is unavailable: %w", err)
	}
	if !info.IsDir() {
		return errors.New("database parent path must be a directory")
	}
	return nil
}

func validateExistingDatabaseFile(databasePath string) error {
	info, err := os.Stat(databasePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect database path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("database path must name a regular file")
	}
	return nil
}

func validatePublicURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, errors.New("public URL is required; use --public-url")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("public URL must be a valid absolute URL")
	}
	if !isSupportedPublicURL(parsed) {
		return nil, errors.New("public URL must be an http or https origin without credentials, query, or fragment")
	}
	return parsed, nil
}

func isSupportedPublicURL(parsed *url.URL) bool {
	hasSupportedScheme := parsed.Scheme == "http" || parsed.Scheme == "https"
	hasOrigin := parsed.Host != "" && (parsed.Path == "" || parsed.Path == "/")
	hasNoUnsafeParts := parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
	return hasSupportedScheme && hasOrigin && hasNoUnsafeParts
}

func validateModeRestrictions(development bool, listenHost string, publicURL *url.URL) error {
	if development {
		return validateDevelopmentMode(listenHost, publicURL)
	}
	if publicURL.Scheme != "https" {
		return errors.New("production public URL must use https")
	}
	return nil
}

func validateDevelopmentMode(listenHost string, publicURL *url.URL) error {
	if !isLoopbackHost(listenHost) {
		return errors.New("development mode must listen on loopback")
	}
	if publicURL.Scheme != "http" || !isLoopbackHost(publicURL.Hostname()) {
		return errors.New("development public URL must use http on loopback")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
