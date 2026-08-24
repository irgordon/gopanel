package server

import (
	"strings"
	"testing"
)

func TestValidateInputReturnsFieldErrors(t *testing.T) {
	valid := Input{Name: "prod-caddy", Address: "10.0.0.12", ConnectionType: "docker"}
	if _, errs := ValidateInput(valid); len(errs) != 0 {
		t.Fatalf("expected valid input, got %v", errs)
	}
	tests := []struct {
		name   string
		change func(*Input)
		field  string
		want   string
	}{
		{name: "name empty", change: func(i *Input) { i.Name = "" }, field: "name", want: "server name"},
		{name: "address scheme", change: func(i *Input) { i.Address = "https://10.0.0.12" }, field: "address", want: "without scheme"},
		{name: "address with slash", change: func(i *Input) { i.Address = "10.0.0.12/api" }, field: "address", want: "without scheme"},
		{name: "connection type", change: func(i *Input) { i.ConnectionType = "unknown" }, field: "connection_type", want: "must be one of"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.change(&input)
			_, errs := ValidateInput(input)
			msg, ok := errs[test.field]
			if !ok {
				t.Fatalf("expected error for %s", test.field)
			}
			if !strings.Contains(msg, test.want) {
				t.Fatalf("expected %q to contain %q", msg, test.want)
			}
		})
	}
}

func TestValidateInputMessagesAreIntegrationSafe(t *testing.T) {
	// Address errors must not imply Docker reachability
	input := Input{Name: "valid-name", Address: "not a host!", ConnectionType: "docker"}
	_, errs := ValidateInput(input)
	msg := errs["address"]
	if strings.Contains(strings.ToLower(msg), "docker") || strings.Contains(strings.ToLower(msg), "reachable") || strings.Contains(strings.ToLower(msg), "socket") {
		t.Fatalf("address validation must not imply Docker reachability, got %q", msg)
	}
}

func TestValidateInputReturnsNormalizedValue(t *testing.T) {
	input := Input{Name: "  valid-name  ", Address: "  caddy.internal  ", ConnectionType: "  caddy  "}
	validated, errs := ValidateInput(input)
	if len(errs) != 0 {
		t.Fatalf("expected valid input, got %v", errs)
	}
	if validated.Name != "valid-name" || validated.Address != "caddy.internal" || validated.ConnectionType != "caddy" {
		t.Fatalf("expected normalized input, got %#v", validated)
	}
}
