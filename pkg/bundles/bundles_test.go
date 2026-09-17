package bundles

import (
	"strings"
	"testing"

	"github.com/giantswarm/clustertest/v5/pkg/application"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestOverrideChildApp(t *testing.T) {
	tests := []struct {
		name         string
		bundleApp    *application.Application
		childApp     *application.Application
		expectedName string
		expectsError bool
	}{
		{
			name:         "security-bundle - existing child app",
			bundleApp:    application.New("test-security-bundle", "security-bundle"),
			childApp:     application.New("test-kyverno-policies", "kyverno-policies").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectedName: "kyvernoPolicies",
		},
		{
			name:         "security-bundle - new child app",
			bundleApp:    application.New("test-security-bundle", "security-bundle"),
			childApp:     application.New("test-new-app", "new-app").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectedName: "newApp",
		},
		{
			name:         "observability-bundle - existing child app",
			bundleApp:    application.New("test-observability-bundle", "observability-bundle"),
			childApp:     application.New("test-prometheus-operator-crd", "prometheus-operator-crd").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectedName: "prometheusOperatorCrd",
		},
		{
			name:         "observability-bundle - new child app",
			bundleApp:    application.New("test-observability-bundle", "observability-bundle"),
			childApp:     application.New("test-new-app", "new-app").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectedName: "newApp",
		},
		{
			name:         "service-mesh-bundle - existing child app",
			bundleApp:    application.New("test-service-mesh-bundle", "service-mesh-bundle"),
			childApp:     application.New("test-linkerd-control-plane", "linkerd-control-plane").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectedName: "linkerd-control-plane",
		},
		{
			name:         "service-mesh-bundle - new child app",
			bundleApp:    application.New("test-service-mesh-bundle", "service-mesh-bundle"),
			childApp:     application.New("test-new-app", "new-app").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectedName: "new-app",
		},
		{
			name:         "auth-bundle - existing child app",
			bundleApp:    application.New("test-auth-bundle", "auth-bundle"),
			childApp:     application.New("test-ingress-nginx", "ingress-nginx").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectedName: "ingress-nginx",
		},
		{
			name:         "auth-bundle - new child app",
			bundleApp:    application.New("test-auth-bundle", "auth-bundle"),
			childApp:     application.New("test-new-app", "new-app").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectedName: "new-app",
		},
		{
			name:         "unknown-bundle",
			bundleApp:    application.New("test-unknown-bundle", "unknown-bundle"),
			childApp:     application.New("test-new-app", "new-app").WithCatalog("test-catalog").WithVersion("1.2.3"),
			expectsError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := OverrideChildApp(tc.bundleApp, tc.childApp, AppNameOverrideAuto)

			if err != nil && !tc.expectsError {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.expectsError {
				// Nothing more to do
				return
			}

			var values bundleValues
			_ = yaml.Unmarshal([]byte(result.Values), &values)

			childAppValues, ok := values.Apps[tc.expectedName]
			if !ok {
				t.Fatalf("Didn't find expected child app values")
			}

			if !childAppValues.Enabled {
				t.Fatalf("Child app not marked as enabled")
			}

			if childAppValues.Catalog != tc.childApp.Catalog {
				t.Fatalf("Catalog didn't match expected. Expected '%s', Actual: '%s'", tc.childApp.Catalog, childAppValues.Catalog)
			}

			if childAppValues.Version != tc.childApp.Version {
				t.Fatalf("Version didn't match expected. Expected '%s', Actual: '%s'", tc.childApp.Version, childAppValues.Version)
			}
		})
	}
}

// TestOverrideChildChartName covers the chart name a bundle child is pulled under: it
// defaults to the app name, and an explicitly published chart name takes over.
func TestOverrideChildChartName(t *testing.T) {
	tests := []struct {
		name          string
		child         ChildOverride
		expectedChart string
	}{
		{
			name:          "chart name defaults to the app name",
			child:         ChildOverride{AppName: "kyverno-policies", Version: "1.2.3"},
			expectedChart: "kyverno-policies",
		},
		{
			name:          "published chart name differs from the app name",
			child:         ChildOverride{AppName: "cluster-autoscaler", ChartName: "cluster-autoscaler-app", Version: "1.2.3"},
			expectedChart: "cluster-autoscaler-app",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bundleApp := application.New("test-security-bundle", "security-bundle")

			result, err := OverrideChild(bundleApp, tc.child, AppNameOverrideAuto)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var values bundleValues
			_ = yaml.Unmarshal([]byte(result.Values), &values)

			childValues, ok := values.Apps[toCamelCase(tc.child.AppName)]
			if !ok {
				t.Fatalf("Didn't find expected child app values")
			}

			if childValues.ChartName != tc.expectedChart {
				t.Fatalf("ChartName didn't match expected. Expected '%s', Actual: '%s'", tc.expectedChart, childValues.ChartName)
			}
		})
	}
}

// TestOverrideChildOmitsEmptyValues checks that a field the suite didn't set is left out
// of the values layer entirely, so the bundle chart's own default applies instead of
// being overridden with an empty string.
func TestOverrideChildOmitsEmptyValues(t *testing.T) {
	bundleApp := application.New("test-security-bundle", "security-bundle")

	result, err := OverrideChild(bundleApp, ChildOverride{AppName: "kyverno-policies", Version: "1.2.3"}, AppNameOverrideAuto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, key := range []string{"catalog:", "namespace:"} {
		if strings.Contains(result.Values, key) {
			t.Fatalf("Expected '%s' to be omitted from the values layer, got:\n%s", key, result.Values)
		}
	}

	if !strings.Contains(result.Values, "version: 1.2.3") {
		t.Fatalf("Expected the version to be set in the values layer, got:\n%s", result.Values)
	}
}
