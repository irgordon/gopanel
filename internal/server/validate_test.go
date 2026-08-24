package server

import (
	"strings"
	"testing"
)

func TestValidateInputReturnsFieldErrors(t *testing.T) {
	valid := Input{Name: "prod-caddy", Address: "10.0.0.12", ConnectionType: "docker"}
	if errs := ValidateInput(valid); len(errs) != 0 {
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
			errs := ValidateInput(input)
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
	errs := ValidateInput(input)
	msg := errs["address"]
	if strings.Contains(strings.ToLower(msg), "docker") || strings.Contains(strings.ToLower(msg), "reachable") || strings.Contains(strings.ToLower(msg), "socket") {
		t.Fatalf("address validation must not imply Docker reachability, got %q", msg)
	}
	// Credential reference errors must not imply semantic meaning
	input2 := Input{Name: "valid-name", Address: "10.0.0.1", ConnectionType: "docker", CredentialReference: strings.Repeat("x", 257)}
	errs2 := ValidateInput(input2)
	msg2 := errs2["credential_reference"]
	if strings.Contains(strings.ToLower(msg2), "docker") || strings.Contains(strings.ToLower(msg2), "vault") || strings.Contains(strings.ToLower(msg2), "kubernetes") {
		t.Fatalf("credential_reference validation must not imply semantic meaning, got %q", msg2)
	}
	// Ensure generic messages
	if !strings.Contains(msg2, "at most 256") {
		t.Fatalf("expected generic length message, got %q", msg2)
	}
}

func TestValidateInputCredentialReferenceNullable(t *testing.T) {
	input := Input{Name: "valid-name", Address: "caddy.internal", ConnectionType: "caddy", CredentialReference: ""}
	if errs := ValidateInput(input); len(errs) != 0 {
		t.Fatalf("expected credential_reference nullable, got %v", errs)
	}
	input.CredentialReference = "prod-context"
	if errs := ValidateInput(input); len(errs) != 0 {
		t.Fatalf("expected opaque credential_reference to pass Phase 3, got %v", errs)
	}
}
