package bundles

import (
	"testing"

	"github.com/giantswarm/clustertest/v3/pkg/application"
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
