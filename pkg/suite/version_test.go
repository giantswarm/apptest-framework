package suite

import "testing"

// TestResolveAppVersion covers the version the suite tests against. Every install mode reads it
// from here, so it has to be resolved the same way regardless of how the app is installed.
func TestResolveAppVersion(t *testing.T) {
	testCases := []struct {
		name          string
		appVersionEnv string
		expected      string
	}{
		{
			name:          "the environment version is used as given",
			appVersionEnv: "1.2.3",
			expected:      "1.2.3",
		},
		{
			name:          "a leading v is trimmed, as the chart is published without one",
			appVersionEnv: "v1.2.3",
			expected:      "1.2.3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("E2E_APP_VERSION", tc.appVersionEnv)

			s := &suite{appName: "kyverno", repoName: "kyverno-app", appCatalog: "giantswarm"}

			if version := s.resolveAppVersion(); version != tc.expected {
				t.Errorf("version = %q, expected %q", version, tc.expected)
			}
		})
	}
}
