package suite

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestBundleHelmReleaseName(t *testing.T) {
	// The parent must be named after the bundle: naming it after the app under test would
	// collide with the child the bundle itself renders into the same namespace.
	if got := bundleHelmReleaseName("test-cluster", "security-bundle"); got != "test-cluster-security-bundle" {
		t.Errorf("bundleHelmReleaseName() = %q, expected %q", got, "test-cluster-security-bundle")
	}
	if got := bundleChildResourceName("test-cluster", "kyverno"); got != "test-cluster-kyverno" {
		t.Errorf("bundleChildResourceName() = %q, expected %q", got, "test-cluster-kyverno")
	}
}

func TestBundleParentValues(t *testing.T) {
	bundleValues := `
clusterValues: false
apps:
  kyverno:
    version: 1.0.0
    namespace: kyverno
  jiralert:
    version: 2.0.0
`
	identity := "clusterID: test-cluster\norganization: giantswarm\n"
	childLayer := "apps:\n  kyverno:\n    enabled: true\n    version: 9.9.9\n    chartName: kyverno\n"

	merged, err := bundleParentValues(bundleValues, identity, childLayer)
	if err != nil {
		t.Fatalf("bundleParentValues() returned an error: %v", err)
	}

	var got map[string]interface{}
	if err := yaml.Unmarshal([]byte(merged), &got); err != nil {
		t.Fatalf("merged values are not valid YAML: %v", err)
	}

	if got["clusterID"] != "test-cluster" {
		t.Errorf("clusterID = %v, expected test-cluster", got["clusterID"])
	}
	if got["organization"] != "giantswarm" {
		t.Errorf("organization = %v, expected giantswarm", got["organization"])
	}
	// A key only bundle_values.yaml sets must survive the merge.
	if got["clusterValues"] != false {
		t.Errorf("clusterValues = %v, expected false", got["clusterValues"])
	}

	apps, ok := got["apps"].(map[string]interface{})
	if !ok {
		t.Fatalf("apps is %T, expected a map", got["apps"])
	}

	kyverno, ok := apps["kyverno"].(map[string]interface{})
	if !ok {
		t.Fatalf("apps.kyverno is %T, expected a map", apps["kyverno"])
	}
	// The child layer is the highest-precedence layer, so it wins on the version.
	if kyverno["version"] != "9.9.9" {
		t.Errorf("apps.kyverno.version = %v, expected 9.9.9", kyverno["version"])
	}
	// but the rest of what bundle_values.yaml said about the child is deep-merged, not replaced.
	if kyverno["namespace"] != "kyverno" {
		t.Errorf("apps.kyverno.namespace = %v, expected kyverno", kyverno["namespace"])
	}
	// Siblings the child layer never mentions are untouched.
	if jiralert, ok := apps["jiralert"].(map[string]interface{}); !ok || jiralert["version"] != "2.0.0" {
		t.Errorf("apps.jiralert = %v, expected version 2.0.0 to survive", apps["jiralert"])
	}
}

func TestBundleParentValuesWithoutBundleValuesFile(t *testing.T) {
	// A suite with no bundle_values.yaml passes an empty layer, which must be skipped rather
	// than blanking the merge.
	merged, err := bundleParentValues("", "clusterID: test-cluster\n", "apps:\n  kyverno:\n    enabled: true\n")
	if err != nil {
		t.Fatalf("bundleParentValues() returned an error: %v", err)
	}

	var got map[string]interface{}
	if err := yaml.Unmarshal([]byte(merged), &got); err != nil {
		t.Fatalf("merged values are not valid YAML: %v", err)
	}

	if got["clusterID"] != "test-cluster" {
		t.Errorf("clusterID = %v, expected test-cluster", got["clusterID"])
	}
	apps, ok := got["apps"].(map[string]interface{})
	if !ok {
		t.Fatalf("apps is %T, expected a map", got["apps"])
	}
	if kyverno, ok := apps["kyverno"].(map[string]interface{}); !ok || kyverno["enabled"] != true {
		t.Errorf("apps.kyverno = %v, expected enabled: true", apps["kyverno"])
	}
}
