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

// TestAppVersionFromEnv covers which values of `E2E_APP_VERSION` name a version to test and
// which ask for the latest published release instead. The fallback itself is not covered here:
// it resolves against the GitHub releases of the app.
func TestAppVersionFromEnv(t *testing.T) {
	testCases := []struct {
		name          string
		appVersionEnv string
		expected      string
		expectedOK    bool
	}{
		{
			name:          "a version is used as given",
			appVersionEnv: "1.2.3",
			expected:      "1.2.3",
			expectedOK:    true,
		},
		{
			name:          "a leading v is trimmed, as the chart is published without one",
			appVersionEnv: "v1.2.3",
			expected:      "1.2.3",
			expectedOK:    true,
		},
		{
			name:          "an unset variable asks for the latest release",
			appVersionEnv: "",
		},
		{
			name:          "`latest` asks for the latest release",
			appVersionEnv: "latest",
		},
		{
			name:          "a bare prefix asks for the latest release, rather than no version",
			appVersionEnv: "v",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("E2E_APP_VERSION", tc.appVersionEnv)

			version, ok := appVersionFromEnv()
			if version != tc.expected || ok != tc.expectedOK {
				t.Errorf("appVersionFromEnv() = (%q, %t), expected (%q, %t)", version, ok, tc.expected, tc.expectedOK)
			}
		})
	}
}
