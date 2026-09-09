package suite

import "testing"

func TestInstallMode(t *testing.T) {
	testCases := []struct {
		name           string
		isDefaultApp   bool
		useHelmRelease bool
		expected       installMode
	}{
		{
			name:     "plain suite installs an App CR",
			expected: installModeApp,
		},
		{
			name:           "WithHelmRelease installs a HelmRelease",
			useHelmRelease: true,
			expected:       installModeHelmRelease,
		},
		{
			name:         "a default app is installed by the cluster chart",
			isDefaultApp: true,
			expected:     installModeDefaultApp,
		},
		{
			// Regression: the install step used to check useHelmRelease first, which made
			// the framework write to the HelmRelease the cluster chart owns.
			name:           "a default app stays a default app with WithHelmRelease set",
			isDefaultApp:   true,
			useHelmRelease: true,
			expected:       installModeDefaultApp,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := &suite{
				isDefaultApp:   tc.isDefaultApp,
				useHelmRelease: tc.useHelmRelease,
			}
			if got := s.installMode(); got != tc.expected {
				t.Errorf("installMode() = %d, expected %d", got, tc.expected)
			}
		})
	}
}
