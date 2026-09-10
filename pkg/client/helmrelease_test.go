package client

import (
	"testing"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHelmReleaseAtVersion(t *testing.T) {
	tests := []struct {
		name     string
		hr       *helmv2.HelmRelease
		version  string
		expected bool
	}{
		{
			name: "deployed at the expected version",
			hr: helmReleaseWithHistory(
				&helmv2.Snapshot{Version: 1, Status: "deployed", ChartVersion: "1.2.3"},
			),
			version:  "1.2.3",
			expected: true,
		},
		{
			name: "deployed version carries an OCI digest and a v prefix",
			hr: helmReleaseWithHistory(
				&helmv2.Snapshot{Version: 1, Status: "deployed", ChartVersion: "0.0.1-abc123+4ef3415e2070"},
			),
			version:  "v0.0.1-abc123",
			expected: true,
		},
		{
			name: "upgrade only attempted, previous version still deployed",
			hr: helmReleaseWithLastAttemptedRevision(
				"1.2.4",
				&helmv2.Snapshot{Version: 1, Status: "deployed", ChartVersion: "1.2.3"},
			),
			version:  "1.2.4",
			expected: false,
		},
		{
			name: "upgrade failed and rolled back",
			hr: helmReleaseWithLastAttemptedRevision(
				"1.2.4",
				&helmv2.Snapshot{Version: 1, Status: "superseded", ChartVersion: "1.2.3"},
				&helmv2.Snapshot{Version: 2, Status: "failed", ChartVersion: "1.2.4"},
				&helmv2.Snapshot{Version: 3, Status: "deployed", ChartVersion: "1.2.3"},
			),
			version:  "1.2.4",
			expected: false,
		},
		{
			name: "latest release is still pending",
			hr: helmReleaseWithHistory(
				&helmv2.Snapshot{Version: 1, Status: "deployed", ChartVersion: "1.2.3"},
				&helmv2.Snapshot{Version: 2, Status: "pending-upgrade", ChartVersion: "1.2.4"},
			),
			version:  "1.2.4",
			expected: false,
		},
		{
			name: "history not out of order",
			hr: helmReleaseWithHistory(
				&helmv2.Snapshot{Version: 2, Status: "deployed", ChartVersion: "1.2.4"},
				&helmv2.Snapshot{Version: 1, Status: "superseded", ChartVersion: "1.2.3"},
			),
			version:  "1.2.4",
			expected: true,
		},
		{
			name:     "no history yet",
			hr:       helmReleaseWithHistory(),
			version:  "1.2.3",
			expected: false,
		},
		{
			name: "no history, falls back to the HelmRepository chart version",
			hr: &helmv2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{Name: "test-app"},
				Spec: helmv2.HelmReleaseSpec{
					Chart: &helmv2.HelmChartTemplate{
						Spec: helmv2.HelmChartTemplateSpec{Chart: "test-app", Version: "1.2.3"},
					},
				},
			},
			version:  "1.2.3",
			expected: true,
		},
		{
			name:     "no history, falls back to the last attempted revision",
			hr:       helmReleaseWithLastAttemptedRevision("1.2.3+4ef3415e2070"),
			version:  "1.2.3",
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if actual := helmReleaseAtVersion(tc.hr, tc.version); actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

func helmReleaseWithHistory(history ...*helmv2.Snapshot) *helmv2.HelmRelease {
	return &helmv2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "test-app"},
		Status:     helmv2.HelmReleaseStatus{History: history},
	}
}

func helmReleaseWithLastAttemptedRevision(revision string, history ...*helmv2.Snapshot) *helmv2.HelmRelease {
	hr := helmReleaseWithHistory(history...)
	hr.Status.LastAttemptedRevision = revision
	return hr
}
