package server

import (
	"errors"
	"net"
	"regexp"
	"strings"
)

var allowedConnectionTypes = map[string]bool{
	"docker": true, "caddy": true, "vault": true, "kubernetes": true,
}

var (
	namePattern          = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _\-\.]{1,62}[a-zA-Z0-9]$`)
	hostnameLabelPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?$`)
)

func ValidateInput(input Input) (ValidatedInput, map[string]string) {
	errs := make(map[string]string)
	if err := validateName(input.Name); err != nil {
		errs["name"] = err.Error()
	}
	if err := validateAddress(input.Address); err != nil {
		errs["address"] = err.Error()
	}
	if err := validateConnectionType(input.ConnectionType); err != nil {
		errs["connection_type"] = err.Error()
	}
	if len(errs) != 0 {
		return ValidatedInput{}, errs
	}
	return ValidatedInput{
		Name:           strings.TrimSpace(input.Name),
		Address:        strings.TrimSpace(input.Address),
		ConnectionType: strings.TrimSpace(input.ConnectionType),
	}, nil
}

func validateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("server name is required")
	}
	if len(trimmed) < 3 || len(trimmed) > 64 {
		return errors.New("server name must be between 3 and 64 characters")
	}
	if !namePattern.MatchString(trimmed) {
		return errors.New("server name must start and end with alphanumeric and contain only letters, numbers, spaces, hyphens, dots, underscores")
	}
	return nil
}

func validateAddress(address string) error {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return errors.New("server address is required")
	}
	if len(trimmed) > 253 {
		return errors.New("server address must be at most 253 characters")
	}
	// No scheme, no credentials, no URL
	if strings.Contains(trimmed, "://") || strings.Contains(trimmed, "@") || strings.Contains(trimmed, "/") {
		return errors.New("server address must be a hostname or IP without scheme or path")
	}
	// Validate as hostname or IP
	if ip := net.ParseIP(trimmed); ip != nil {
		return nil
	}
	if !isValidHostname(trimmed) {
		return errors.New("server address must be a valid hostname or IP address")
	}
	return nil
}

func isValidHostname(host string) bool {
	if len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if !hostnameLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

func validateConnectionType(connectionType string) error {
	normalized := strings.TrimSpace(connectionType)
	if normalized == "" {
		return errors.New("connection type is required")
	}
	if !allowedConnectionTypes[normalized] {
		return errors.New("connection type must be one of: docker, caddy, vault, kubernetes")
	}
	return nil
}
