package client

import (
	"testing"
)

func TestResolveSource(t *testing.T) {
	testCases := []struct {
		name     string
		cfg      HelmReleaseConfig
		expected resolvedSource
	}{
		{
			name: "everything set explicitly is passed through untouched",
			cfg: HelmReleaseConfig{
				Name:            "test-cluster-my-app",
				Namespace:       "org-test",
				ChartName:       "my-app",
				SourceKind:      SourceKindHelmRepository,
				SourceName:      "shared-repo",
				SourceNamespace: "giantswarm",
				SourceURL:       "https://example.com/charts",
			},
			expected: resolvedSource{
				Kind:      SourceKindHelmRepository,
				Name:      "shared-repo",
				Namespace: "giantswarm",
				URL:       "https://example.com/charts",
			},
		},
		{
			name: "empty kind defaults to OCIRepository",
			cfg: HelmReleaseConfig{
				Name:      "test-cluster-my-app",
				Namespace: "org-test",
				ChartName: "my-app",
			},
			expected: resolvedSource{
				Kind:      SourceKindOCIRepository,
				Name:      "test-cluster-my-app",
				Namespace: "org-test",
				URL:       DefaultGiantSwarmHelmRepositoryURL + "/my-app",
			},
		},
		{
			// The source is named after the HelmRelease, not the chart, so each test run
			// owns its own source instead of sharing one under the chart's name.
			name: "empty source name defaults to the HelmRelease name, not the chart name",
			cfg: HelmReleaseConfig{
				Name:      "test-cluster-cluster-autoscaler",
				Namespace: "org-test",
				ChartName: "cluster-autoscaler-app",
			},
			expected: resolvedSource{
				Kind:      SourceKindOCIRepository,
				Name:      "test-cluster-cluster-autoscaler",
				Namespace: "org-test",
				URL:       DefaultGiantSwarmHelmRepositoryURL + "/cluster-autoscaler-app",
			},
		},
		{
			name: "empty source namespace defaults to the HelmRelease namespace",
			cfg: HelmReleaseConfig{
				Name:       "test-cluster-my-app",
				Namespace:  "org-test",
				ChartName:  "my-app",
				SourceName: "explicit-source",
			},
			expected: resolvedSource{
				Kind:      SourceKindOCIRepository,
				Name:      "explicit-source",
				Namespace: "org-test",
				URL:       DefaultGiantSwarmHelmRepositoryURL + "/my-app",
			},
		},
		{
			name: "empty URL for an OCIRepository defaults to the chart's OCI path",
			cfg: HelmReleaseConfig{
				Name:       "test-cluster-my-app",
				Namespace:  "org-test",
				ChartName:  "my-app",
				SourceKind: SourceKindOCIRepository,
			},
			expected: resolvedSource{
				Kind:      SourceKindOCIRepository,
				Name:      "test-cluster-my-app",
				Namespace: "org-test",
				URL:       DefaultGiantSwarmHelmRepositoryURL + "/my-app",
			},
		},
		{
			name: "empty URL for a HelmRepository defaults to the registry root",
			cfg: HelmReleaseConfig{
				Name:       "test-cluster-my-app",
				Namespace:  "org-test",
				ChartName:  "my-app",
				SourceKind: SourceKindHelmRepository,
			},
			expected: resolvedSource{
				Kind:      SourceKindHelmRepository,
				Name:      "test-cluster-my-app",
				Namespace: "org-test",
				URL:       DefaultGiantSwarmHelmRepositoryURL,
			},
		},
		{
			// No chart name means no URL can be built, which ensureHelmSource treats as
			// "the source CR is expected to already exist".
			name: "empty chart name yields an empty URL for an OCIRepository",
			cfg: HelmReleaseConfig{
				Name:       "test-cluster-my-app",
				Namespace:  "org-test",
				SourceKind: SourceKindOCIRepository,
			},
			expected: resolvedSource{
				Kind:      SourceKindOCIRepository,
				Name:      "test-cluster-my-app",
				Namespace: "org-test",
				URL:       "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := resolveSource(tc.cfg)
			if actual != tc.expected {
				t.Errorf("resolveSource() = %+v, expected %+v", actual, tc.expected)
			}
		})
	}
}

// TestResolveSourceIsStableAcrossInstallAndCleanup guards the bug this change fixes: the
// suite resolves the source at install time with a chart version and again during cleanup
// without one, and both must name the same object or the source is leaked.
func TestResolveSourceIsStableAcrossInstallAndCleanup(t *testing.T) {
	install := HelmReleaseConfig{
		Name:         "test-cluster-my-app",
		Namespace:    "org-test",
		ChartName:    "my-app",
		ChartVersion: "1.2.3",
	}
	cleanup := install
	cleanup.ChartVersion = ""

	if got, want := resolveSource(cleanup), resolveSource(install); got != want {
		t.Errorf("cleanup resolved %+v but install resolved %+v", got, want)
	}
}
