package suite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/giantswarm/clustertest/v5/pkg/application"
)

// TestRenderValuesFile covers the values a suite installs with: the file is a Go template on
// both install paths, so a placeholder has to be substituted rather than passed through.
func TestRenderValuesFile(t *testing.T) {
	templateValues := &application.TemplateValues{
		ClusterName:  "test-cluster",
		Namespace:    "org-giantswarm",
		Organization: "giantswarm",
	}

	testCases := []struct {
		name     string
		contents string
		// missing skips writing the file at all.
		missing  bool
		expected string
	}{
		{
			name:     "placeholders are substituted",
			contents: "clusterName: {{ .ClusterName }}\norganization: {{ .Organization }}\nnamespace: {{ .Namespace }}\n",
			expected: "clusterName: test-cluster\norganization: giantswarm\nnamespace: org-giantswarm\n",
		},
		{
			name:     "a plain values file is passed through",
			contents: "replicas: 2\n",
			expected: "replicas: 2\n",
		},
		{
			name:     "an empty file means no values",
			contents: "",
			expected: "",
		},
		{
			name:     "a whitespace-only file means no values",
			contents: "\n\n",
			expected: "",
		},
		{
			name:     "a missing file means no values",
			missing:  true,
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "values.yaml")
			if !tc.missing {
				if err := os.WriteFile(path, []byte(tc.contents), 0o600); err != nil {
					t.Fatalf("writing values file: %s", err)
				}
			}

			got, err := renderValuesFile(path, templateValues)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != tc.expected {
				t.Errorf("rendered values = %q, expected %q", got, tc.expected)
			}
		})
	}
}

// TestRenderValuesFileInvalidTemplate covers a values file that cannot be rendered: it has to
// fail the suite rather than silently install with whatever the file happened to contain.
func TestRenderValuesFileInvalidTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "values.yaml")
	if err := os.WriteFile(path, []byte("clusterName: {{ .Nope }}\n"), 0o600); err != nil {
		t.Fatalf("writing values file: %s", err)
	}

	if _, err := renderValuesFile(path, &application.TemplateValues{}); err == nil {
		t.Error("expected an error for a template referencing an unknown variable, got none")
	}
}
