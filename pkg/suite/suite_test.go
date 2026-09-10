package suite

import (
	"testing"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestIsHelmReleaseForApp(t *testing.T) {
	testCases := []struct {
		name     string
		hr       helmv2.HelmRelease
		expected bool
	}{
		{
			name:     "bare app name",
			hr:       helmv2.HelmRelease{ObjectMeta: metav1.ObjectMeta{Name: "cert-manager"}},
			expected: true,
		},
		{
			name:     "cluster prefixed name",
			hr:       helmv2.HelmRelease{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-cert-manager"}},
			expected: true,
		},
		{
			name: "matching chart name",
			hr: helmv2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{Name: "some-other-name"},
				Spec: helmv2.HelmReleaseSpec{
					Chart: &helmv2.HelmChartTemplate{
						Spec: helmv2.HelmChartTemplateSpec{Chart: "cert-manager"},
					},
				},
			},
			expected: true,
		},
		{
			name: "matching chart ref",
			hr: helmv2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{Name: "some-other-name"},
				Spec: helmv2.HelmReleaseSpec{
					ChartRef: &helmv2.CrossNamespaceSourceReference{Name: "cert-manager"},
				},
			},
			expected: true,
		},
		{
			name:     "unrelated release",
			hr:       helmv2.HelmRelease{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster-coredns"}},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHelmReleaseForApp(tc.hr, "test-cluster", "cert-manager"); got != tc.expected {
				t.Errorf("isHelmReleaseForApp() = %t, expected %t", got, tc.expected)
			}
		})
	}
}
