package view

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestGenericComponentsRenderTheirPublicContract(t *testing.T) {
	tests := []struct {
		name      string
		component templ.Component
		expected  []string
	}{
		{
			name:      "button",
			component: Button("Continue"),
			expected:  []string{`<button type="submit"`, `>Continue</button>`},
		},
		{
			name:      "badge",
			component: Badge("Ready"),
			expected:  []string{`<span`, `>Ready</span>`},
		},
		{
			name:      "card",
			component: Card(),
			expected:  []string{`<section`, `</section>`},
		},
		{
			name:      "alert",
			component: Alert("Action required", "Try again."),
			expected:  []string{`role="status"`, `aria-live="polite"`, `Action required`, `Try again.`},
		},
		{
			name:      "form field",
			component: FormField("server-name", "Server name", "name", "alpha", "Use a recognizable name.", "Name is required."),
			expected:  []string{`label for="server-name"`, `name="name"`, `value="alpha"`, `Use a recognizable name.`, `Name is required.`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			html := renderView(t, test.component)
			for _, expected := range test.expected {
				if !strings.Contains(html, expected) {
					t.Fatalf("expected rendered component to contain %q", expected)
				}
			}
		})
	}
}

func renderView(t *testing.T, component templ.Component) string {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatalf("render component: %v", err)
	}
	return output.String()
}
