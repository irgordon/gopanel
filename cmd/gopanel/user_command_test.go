package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParseUserCommandRejectsUnknownInput(t *testing.T) {
	for _, arguments := range [][]string{
		{},
		{"unknown"},
		{"create-admin", "--unknown"},
		{"create-admin", "--database-path"},
		{"create-admin", "unexpected"},
	} {
		if _, err := parseUserCommand(arguments); err == nil {
			t.Fatalf("expected arguments %q to fail", arguments)
		}
	}
}

func TestReadAdminCredentialsUsesHiddenPasswordReader(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	if _, err := writer.WriteString("admin@example.com\nAdmin User\n"); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	password := "  password with spaces  "

	credentials, err := readAdminCredentials(reader, &output, func(int) bool { return true }, func(int) ([]byte, error) {
		return []byte(password), nil
	})

	if err != nil {
		t.Fatal(err)
	}
	if credentials.email != "admin@example.com" || credentials.name != "Admin User" || credentials.password != password {
		t.Fatalf("unexpected credentials: %#v", credentials)
	}
	if strings.Contains(output.String(), password) {
		t.Fatal("password was written to terminal output")
	}
}
