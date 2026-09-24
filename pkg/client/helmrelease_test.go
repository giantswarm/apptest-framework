package client

import (
	"testing"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/onsi/gomega"
)

// TestSortedValuesFrom covers the order Flux merges a HelmRelease's values sources in, which is
// what decides which layer wins. The App platform merges all ConfigMaps before all Secrets,
// each by ascending priority, and the cluster and bundle charts render their HelmReleases that
// way, so the framework has to as well.
func TestSortedValuesFrom(t *testing.T) {
	testCases := []struct {
		name     string
		sources  []ValuesSource
		expected []string
	}{
		{
			name:     "no sources means no valuesFrom",
			sources:  nil,
			expected: nil,
		},
		{
			name: "config maps are merged before secrets",
			sources: []ValuesSource{
				{Kind: "Secret", Name: "app-values", Priority: ValuesPriorityUserConfig},
				{Kind: "ConfigMap", Name: "cluster-values", Priority: ValuesPriorityDefault},
			},
			expected: []string{"cluster-values", "app-values"},
		},
		{
			name: "a lower priority is merged first, so a higher one wins",
			sources: []ValuesSource{
				{Kind: "ConfigMap", Name: "user", Priority: ValuesPriorityUserConfig},
				{Kind: "ConfigMap", Name: "cluster", Priority: ValuesPriorityDefault},
			},
			expected: []string{"cluster", "user"},
		},
		{
			name: "an unset priority takes the default slot",
			sources: []ValuesSource{
				{Kind: "ConfigMap", Name: "user", Priority: ValuesPriorityUserConfig},
				{Kind: "ConfigMap", Name: "unset"},
			},
			expected: []string{"unset", "user"},
		},
		{
			name: "sources in the same slot keep the order they were given in",
			sources: []ValuesSource{
				{Kind: "ConfigMap", Name: "first", Priority: ValuesPriorityDefault},
				{Kind: "ConfigMap", Name: "second", Priority: ValuesPriorityDefault},
			},
			expected: []string{"first", "second"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			refs := sortedValuesFrom(tc.sources)

			if len(refs) != len(tc.expected) {
				t.Fatalf("got %d valuesFrom entries, expected %d: %v", len(refs), len(tc.expected), refs)
			}
			for i, name := range tc.expected {
				if refs[i].Name != name {
					t.Errorf("valuesFrom[%d] = %q, expected %q", i, refs[i].Name, name)
				}
			}
		})
	}
}

// TestSortedValuesFromKeepsFields covers the per-source settings that decide how a layer is
// read: an App platform ConfigMap keeps its values under `values`, and an optional source must
// stay optional or a cluster without one fails the release.
func TestSortedValuesFromKeepsFields(t *testing.T) {
	refs := sortedValuesFrom([]ValuesSource{
		{Kind: "ConfigMap", Name: "cluster-values", ValuesKey: "values", Optional: true},
	})

	expected := helmv2.ValuesReference{
		Kind:      "ConfigMap",
		Name:      "cluster-values",
		ValuesKey: "values",
		Optional:  true,
	}
	if len(refs) != 1 || refs[0] != expected {
		t.Errorf("valuesFrom = %v, expected %v", refs, []helmv2.ValuesReference{expected})
	}
}

// TestSortedValuesFromDoesNotMutate covers that sorting leaves the caller's slice alone: the
// config it comes from is reused across the install, upgrade and uninstall steps.
func TestSortedValuesFromDoesNotMutate(t *testing.T) {
	sources := []ValuesSource{
		{Kind: "Secret", Name: "app-values", Priority: ValuesPriorityUserConfig},
		{Kind: "ConfigMap", Name: "cluster-values"},
	}

	_ = sortedValuesFrom(sources)

	if sources[0].Name != "app-values" || sources[1].Name != "cluster-values" {
		t.Errorf("sources were reordered: %v", sources)
	}
}

// TestBuildHelmReleaseValuesFrom covers where the suite's own values file lands among the other
// layers: in the user config slot, so that anything merged for the suite can be overridden by
// it.
func TestBuildHelmReleaseValuesFrom(t *testing.T) {
	hr := buildHelmRelease(HelmReleaseConfig{
		Name:         "test-app",
		Namespace:    "org-giantswarm",
		ChartName:    "test-app",
		ChartVersion: "1.2.3",
		Values:       "replicas: 2\n",
		ValuesFrom: []ValuesSource{
			{Kind: "ConfigMap", Name: "test-cluster-values", ValuesKey: "values", Optional: true},
		},
	})

	expected := []string{"test-cluster-values", "test-app-values"}
	if len(hr.Spec.ValuesFrom) != len(expected) {
		t.Fatalf("got %d valuesFrom entries, expected %d: %v", len(hr.Spec.ValuesFrom), len(expected), hr.Spec.ValuesFrom)
	}
	for i, name := range expected {
		if hr.Spec.ValuesFrom[i].Name != name {
			t.Errorf("valuesFrom[%d] = %q, expected %q", i, hr.Spec.ValuesFrom[i].Name, name)
		}
	}
	if hr.Spec.Values != nil {
		t.Errorf("spec.values = %s, expected the values to be referenced from the Secret", hr.Spec.Values.Raw)
	}
}

// TestBuildHelmReleaseInlineValues covers the inline path: the values go to spec.values, and no
// Secret is referenced for them.
func TestBuildHelmReleaseInlineValues(t *testing.T) {
	// buildHelmRelease asserts on the values it marshals, so it needs a fail handler.
	gomega.RegisterTestingT(t)

	hr := buildHelmRelease(HelmReleaseConfig{
		Name:         "test-app",
		Namespace:    "org-giantswarm",
		ChartName:    "test-app",
		ChartVersion: "1.2.3",
		Values:       "replicas: 2\n",
		InlineValues: true,
	})

	if hr.Spec.Values == nil || string(hr.Spec.Values.Raw) != `{"replicas":2}` {
		t.Errorf("spec.values = %v, expected the values inline", hr.Spec.Values)
	}
	if len(hr.Spec.ValuesFrom) != 0 {
		t.Errorf("valuesFrom = %v, expected none", hr.Spec.ValuesFrom)
	}
}

// TestBuildHelmReleaseInlineValuesWithValuesFrom covers a bundle's config: its own values are
// inline, and anything merged for it (the cluster values) still has to be referenced. Flux
// merges valuesFrom first, so the bundle's values keep winning.
func TestBuildHelmReleaseInlineValuesWithValuesFrom(t *testing.T) {
	gomega.RegisterTestingT(t)

	hr := buildHelmRelease(HelmReleaseConfig{
		Name:         "test-bundle",
		Namespace:    "org-giantswarm",
		ChartName:    "test-bundle",
		ChartVersion: "1.2.3",
		Values:       "replicas: 2\n",
		InlineValues: true,
		ValuesFrom: []ValuesSource{
			{Kind: "ConfigMap", Name: "test-cluster-values", ValuesKey: "values", Optional: true},
		},
	})

	if hr.Spec.Values == nil || string(hr.Spec.Values.Raw) != `{"replicas":2}` {
		t.Errorf("spec.values = %v, expected the values inline", hr.Spec.Values)
	}
	if len(hr.Spec.ValuesFrom) != 1 || hr.Spec.ValuesFrom[0].Name != "test-cluster-values" {
		t.Errorf("valuesFrom = %v, expected the cluster values ConfigMap", hr.Spec.ValuesFrom)
	}
}
