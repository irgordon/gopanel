package auth

import (
	"bytes"
	"testing"
	"time"
)

func TestLoginCSRFBindsContextExpiryAndProcessKey(t *testing.T) {
	now := time.Now().UTC()
	csrf := NewCSRF(bytes.Repeat([]byte{1}, 32))
	contextCookie, token, _, err := csrf.NewLoginContext(now)
	if err != nil {
		t.Fatal(err)
	}
	otherContext, _, _, err := csrf.NewLoginContext(now)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewCSRF(bytes.Repeat([]byte{2}, 32))
	tests := []struct {
		name    string
		csrf    *CSRF
		context string
		token   string
		now     time.Time
		valid   bool
	}{
		{name: "valid", csrf: csrf, context: contextCookie, token: token, now: now, valid: true},
		{name: "missing", csrf: csrf, context: contextCookie, token: "", now: now},
		{name: "malformed", csrf: csrf, context: contextCookie, token: "v1.invalid", now: now},
		{name: "other context", csrf: csrf, context: otherContext, token: token, now: now},
		{name: "expired", csrf: csrf, context: contextCookie, token: token, now: now.Add(loginContextLifetime)},
		{name: "restarted process", csrf: restarted, context: contextCookie, token: token, now: now},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := test.csrf.ValidateLogin(test.context, test.token, test.now); actual != test.valid {
				t.Fatalf("expected valid=%v, got %v", test.valid, actual)
			}
		})
	}
}

func TestAuthenticatedCSRFBindsSessionAndProcessKey(t *testing.T) {
	csrf := NewCSRF(bytes.Repeat([]byte{3}, 32))
	token, err := csrf.AuthToken("session-one")
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewCSRF(bytes.Repeat([]byte{4}, 32))
	tests := []struct {
		name    string
		csrf    *CSRF
		session string
		token   string
		valid   bool
	}{
		{name: "valid", csrf: csrf, session: "session-one", token: token, valid: true},
		{name: "missing session", csrf: csrf, token: token},
		{name: "other session", csrf: csrf, session: "session-two", token: token},
		{name: "malformed", csrf: csrf, session: "session-one", token: "v1.invalid"},
		{name: "restarted process", csrf: restarted, session: "session-one", token: token},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := test.csrf.ValidateAuth(test.session, test.token); actual != test.valid {
				t.Fatalf("expected valid=%v, got %v", test.valid, actual)
			}
		})
	}
}
