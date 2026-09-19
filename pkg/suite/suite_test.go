package suite

import (
	"slices"
	"testing"

	"github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/clustertest/v5/pkg/application"
)

func TestInstallMode(t *testing.T) {
	testCases := []struct {
		name           string
		isDefaultApp   bool
		useHelmRelease bool
		inBundleApp    string
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
		{
			name:        "InAppBundle on its own installs App CRs",
			inBundleApp: "security-bundle",
			expected:    installModeApp,
		},
		{
			name:           "InAppBundle with WithHelmRelease installs the bundle as a HelmRelease",
			useHelmRelease: true,
			inBundleApp:    "security-bundle",
			expected:       installModeBundleHelmRelease,
		},
		{
			// A bundle the Release ships is the cluster chart's to install, whatever the
			// suite asked for.
			name:           "a default app bundle stays a default app",
			isDefaultApp:   true,
			useHelmRelease: true,
			inBundleApp:    "security-bundle",
			expected:       installModeDefaultApp,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := &suite{
				isDefaultApp:   tc.isDefaultApp,
				useHelmRelease: tc.useHelmRelease,
				inBundleApp:    tc.inBundleApp,
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

func TestValidate(t *testing.T) {
	testCases := []struct {
		name           string
		isMCTest       bool
		useHelmRelease bool
		inBundleApp    string
		expectErr      bool
	}{
		{
			name: "a plain suite is valid",
		},
		{
			name:        "a bundle suite on a workload cluster is valid",
			inBundleApp: "security-bundle",
		},
		{
			name:     "an MC suite without a bundle is valid",
			isMCTest: true,
		},
		{
			// Bundle children hardcode `<clusterID>-kubeconfig`, which does not exist when
			// the MC itself is the target. Unsupported on both install paths.
			name:        "an MC bundle suite is rejected in App CR mode",
			isMCTest:    true,
			inBundleApp: "security-bundle",
			expectErr:   true,
		},
		{
			name:           "an MC bundle suite is rejected in HelmRelease mode",
			isMCTest:       true,
			useHelmRelease: true,
			inBundleApp:    "security-bundle",
			expectErr:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := &suite{
				isMCTest:       tc.isMCTest,
				useHelmRelease: tc.useHelmRelease,
				inBundleApp:    tc.inBundleApp,
			}
			err := s.validate()
			if tc.expectErr && err == nil {
				t.Errorf("validate() = nil, expected an error")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("validate() = %v, expected no error", err)
			}
		})
	}
}

// TestCloneApplication covers the copy the upgrade pre-install runs off: the builder methods
// mutate in place, so anything reconfigured on the copy has to leave the app under test alone.
func TestCloneApplication(t *testing.T) {
	app := application.New("test-cluster-test-app", "test-app").
		WithVersion("1.2.3").
		WithCatalog("default").
		WithAppLabels(map[string]string{"app": "test-app"}).
		WithExtraConfigs([]v1alpha1.AppExtraConfig{{Kind: "configMap", Name: "test-app-bundle-values"}})

	clone := cloneApplication(app).WithVersion("latest").WithCatalog("other")
	clone.AppLabels["app"] = "other-app"
	clone.ExtraConfigs[0].Name = "other-values"

	if app.Version != "1.2.3" {
		t.Errorf("version = %q, expected the original to keep '1.2.3'", app.Version)
	}
	if app.Catalog != "default" {
		t.Errorf("catalog = %q, expected the original to keep 'default'", app.Catalog)
	}
	if app.AppLabels["app"] != "test-app" {
		t.Errorf("appLabels = %v, expected the original's labels to be untouched", app.AppLabels)
	}
	if app.ExtraConfigs[0].Name != "test-app-bundle-values" {
		t.Errorf("extraConfigs = %v, expected the original's extra configs to be untouched", app.ExtraConfigs)
	}
}

// TestCloneApplicationNil covers the nil case, so a clone of an app that was never set does not
// panic before the suite can report what is actually missing.
func TestCloneApplicationNil(t *testing.T) {
	if clone := cloneApplication(nil); clone != nil {
		t.Errorf("clone = %v, expected nil", clone)
	}
}
