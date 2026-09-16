package suite

import (
	"slices"
	"testing"
)

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

func TestDefaultAppResourceNames(t *testing.T) {
	testCases := []struct {
		name      string
		appName   string
		chartName string
		override  string
		expected  []string
	}{
		{
			name:      "chart name equal to the app name",
			appName:   "cert-manager",
			chartName: "cert-manager",
			expected:  []string{"test-cluster-cert-manager"},
		},
		{
			// The cluster chart names the resource after the app, not after the chart, so
			// the app name has to come first.
			name:      "chart name differing from the app name",
			appName:   "cluster-autoscaler",
			chartName: "cluster-autoscaler-app",
			expected:  []string{"test-cluster-cluster-autoscaler", "test-cluster-cluster-autoscaler-app"},
		},
		{
			name:     "no chart name known",
			appName:  "security-bundle",
			expected: []string{"test-cluster-security-bundle"},
		},
		{
			// aws-ebs-csi-driver is rendered from a hand-written template that names the
			// resource after neither the app nor a chart name the suite knows.
			name:      "explicit override wins",
			appName:   "aws-ebs-csi-driver",
			chartName: "aws-ebs-csi-driver",
			override:  "aws-ebs-csi-driver-bundle",
			expected:  []string{"test-cluster-aws-ebs-csi-driver-bundle"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultAppResourceNames("test-cluster", tc.appName, tc.chartName, tc.override)
			if !slices.Equal(got, tc.expected) {
				t.Errorf("defaultAppResourceNames() = %v, expected %v", got, tc.expected)
			}
		})
	}
}
