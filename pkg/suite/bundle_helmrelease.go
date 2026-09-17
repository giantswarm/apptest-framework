package suite

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck
	. "github.com/onsi/gomega"    //nolint:staticcheck

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/giantswarm/cluster-standup-teardown/v6/pkg/values"
	"github.com/giantswarm/clustertest/v5/pkg/logger"
	"k8s.io/apimachinery/pkg/types"

	"github.com/giantswarm/apptest-framework/v5/pkg/bundles"
	"github.com/giantswarm/apptest-framework/v5/pkg/client"
	"github.com/giantswarm/apptest-framework/v5/pkg/state"
)

const (
	// bundleServiceAccountName is the service account the parent bundle HelmRelease
	// impersonates. It already exists in every organization namespace and is what the cluster
	// chart and cluster-test-suites use for the same purpose.
	bundleServiceAccountName = "automation"

	// bundleUninstallTimeout bounds the wait for the parent bundle's uninstall to finish. It
	// has to complete before the workload cluster is torn down: once the cluster's kubeconfig
	// secret is gone, a child HelmRelease's finalizer can never run and the organization
	// namespace is stuck terminating.
	bundleUninstallTimeout = 5 * time.Minute
	// bundleChildCleanupTimeout bounds the best-effort wait for the bundle's children to
	// disappear after the parent was uninstalled.
	bundleChildCleanupTimeout = 5 * time.Minute
	// bundleChildInstallTimeout bounds the wait for a bundle child to be deployed at the
	// version under test once the parent bundle itself is installed.
	bundleChildInstallTimeout = 10 * time.Minute
)

// bundleHelmReleaseName returns the name of the parent bundle's HelmRelease.
//
// The parent is named after the bundle, never after the app under test: the bundle renders its
// children as `<cluster>-<childAppName>` in the very same namespace, so a parent named after
// the child would collide with the resource the parent itself creates.
func bundleHelmReleaseName(clusterName, bundleAppName string) string {
	return fmt.Sprintf("%s-%s", clusterName, bundleAppName)
}

// bundleChildResourceName returns the name a bundle chart gives the child it installs into the
// organization namespace. Both bundle generations use the same convention: App CR bundles
// (service-mesh-bundle, gateway-api-bundle) and HelmRelease bundles (security-bundle 2.x) alike
// name it `<clusterID>-<appName>`.
func bundleChildResourceName(clusterName, childAppName string) string {
	return fmt.Sprintf("%s-%s", clusterName, childAppName)
}

func (s *suite) bundleHelmReleaseName() string {
	return bundleHelmReleaseName(state.GetCluster().Name, s.inBundleApp)
}

// bundleChildRef points at the resource the bundle renders for the app under test.
func (s *suite) bundleChildRef() managedAppRef {
	cluster := state.GetCluster()
	appName := state.GetApplication().AppName

	return managedAppRef{
		AppName:   appName,
		Namespace: cluster.GetNamespace(),
		Names:     []string{bundleChildResourceName(cluster.Name, appName)},
	}
}

// bundleParentValues merges the value layers of a parent bundle, lowest precedence first.
//
// This reproduces the precedence of the App CR path, where `bundle_values.yaml` is an
// extraConfigs ConfigMap at priority 25, below the user config layer that carries the cluster
// identity and the child override.
func bundleParentValues(bundleValues, identity, childLayer string) (string, error) {
	return values.Merge(bundleValues, identity, childLayer)
}

// loadBundleValues reads the bundle values file and renders it as a Go template, with the same
// variables as the app's own values file. A missing file yields empty values.
func (s *suite) loadBundleValues() string {
	GinkgoHelper()

	if s.bundleValuesFile == "" {
		return ""
	}

	rendered, err := renderValuesFile(s.bundleValuesFile, appTemplateValues(state.GetCluster()))
	Expect(err).NotTo(HaveOccurred())

	return rendered
}

// bundleParentValues builds the full values of the parent bundle with the app under test pinned
// to childVersion.
func (s *suite) bundleParentValues(childVersion string) string {
	GinkgoHelper()

	cluster := state.GetCluster()
	app := state.GetApplication()

	// The bundle chart reads both of these: clusterID names the children and the kubeconfig
	// secret they reach the cluster through, organization goes into their labels. Logged
	// because a bundle chart with `additionalProperties: false` at the root would reject them.
	identity := fmt.Sprintf("clusterID: %s\norganization: %s\n", cluster.Name, cluster.Organization.Name)
	logger.Log("Setting bundle values 'clusterID: %s' and 'organization: %s'", cluster.Name, cluster.Organization.Name)

	childLayer, err := bundles.ChildValues(s.inBundleApp, bundles.ChildOverride{
		AppName:   app.AppName,
		ChartName: s.getHelmChartName(),
		Catalog:   app.Catalog,
		Version:   childVersion,
		Namespace: app.InstallNamespace,
	}, s.inBundleAppOverrideType)
	Expect(err).NotTo(HaveOccurred())

	merged, err := bundleParentValues(s.loadBundleValues(), identity, childLayer)
	Expect(err).NotTo(HaveOccurred())

	return merged
}

// bundleHelmReleaseConfig builds the config for the parent bundle chart.
//
// It is deliberately not buildHelmReleaseConfig: a bundle is installed the other way round. The
// management cluster has to own the bundle's own release, because it is the bundle that hands
// each child the workload cluster's kubeconfig, so the parent runs in-cluster under an
// impersonated service account with no kubeConfig of its own. With kubeConfig unset the
// flux-multi-tenancy Kyverno policy also requires the target and storage namespaces to equal
// the HelmRelease's own namespace.
func (s *suite) bundleHelmReleaseConfig(chartValues string) client.HelmReleaseConfig {
	cluster := state.GetCluster()
	namespace := cluster.Organization.GetNamespace()
	name := s.bundleHelmReleaseName()

	serviceAccountName := s.helmServiceAccountName
	if serviceAccountName == "" {
		serviceAccountName = bundleServiceAccountName
	}

	// A failed upgrade must not be remediated: a rollback would restore the previous release's
	// values, and with them the previous child version, turning an upgrade failure into a
	// confusing child version mismatch.
	noRetries := 0

	return client.HelmReleaseConfig{
		Name:      name,
		Namespace: namespace,
		// Kyverno requires both to match the HelmRelease's namespace while kubeConfig is unset.
		TargetNamespace:  namespace,
		StorageNamespace: namespace,
		ChartName:        s.inBundleApp,
		ChartVersion:     state.GetBundleApplication().Version,
		// Named explicitly: the source name otherwise defaults to the bare chart name, which
		// every suite in this namespace would share, and an existing source is never updated,
		// so a leftover would pin the bundle to another run's version.
		SourceName:         name,
		SourceURL:          s.helmSourceURL,
		Timeout:            s.helmTimeout,
		Retries:            &noRetries,
		ServiceAccountName: serviceAccountName,
		InCluster:          true,
		InlineValues:       true,
		Values:             chartValues,
	}
}

// installBundleHelmRelease installs the parent bundle at the version the Release pins, with the
// app under test pinned to childVersion.
func (s *suite) installBundleHelmRelease(ctx context.Context, childVersion string) {
	GinkgoHelper()

	cfg := s.bundleHelmReleaseConfig(s.bundleParentValues(childVersion))
	logger.Log("Installing bundle '%s' version '%s' as HelmRelease %s/%s, with '%s' pinned to '%s'",
		s.inBundleApp, cfg.ChartVersion, cfg.Namespace, cfg.Name, state.GetApplication().AppName, childVersion)

	client.InstallHelmRelease(ctx, cfg)
}

// updateBundleChildVersion moves the app under test to childVersion by rewriting the parent's
// values, leaving the bundle itself at the version it was installed at.
//
// Only the child moves: upgrading the bundle at the same time would be testing two changes at
// once, and can be a bundle downgrade when the Release pins an older bundle than the one
// published.
func (s *suite) updateBundleChildVersion(ctx context.Context, childVersion string) {
	GinkgoHelper()

	cfg := s.bundleHelmReleaseConfig(s.bundleParentValues(childVersion))
	logger.Log("Moving '%s' to version '%s' in bundle HelmRelease %s/%s",
		state.GetApplication().AppName, childVersion, cfg.Namespace, cfg.Name)

	err := client.UpdateHelmReleaseValues(ctx, cfg.Name, cfg.Namespace, cfg.Values)
	Expect(err).NotTo(HaveOccurred())
}

// waitForBundleParent waits for the parent bundle's HelmRelease to be ready.
//
// This is a weak signal on its own: the parent's manifests are only more CRs, which Helm does
// not wait on, so the parent reports ready while its children are still reconciling or failing.
// waitForBundleChild is the assertion that matters.
func (s *suite) waitForBundleParent(ctx context.Context) {
	GinkgoHelper()

	name := s.bundleHelmReleaseName()
	namespace := state.GetCluster().Organization.GetNamespace()

	Eventually(func() (bool, error) {
		return client.IsHelmReleaseReady(state.GetContext(), name, namespace)
	}).
		WithContext(ctx).
		WithPolling(5*time.Second).
		Should(BeTrue(), "bundle HelmRelease '%s/%s' did not become ready", namespace, name)
}

// waitForBundleChild waits until the resource the bundle renders for the app under test is
// deployed at the given version.
//
// The app version is never asserted on the parent: the parent's version is the bundle chart's.
func (s *suite) waitForBundleChild(ctx context.Context, version string) {
	GinkgoHelper()

	ref := s.bundleChildRef()
	logger.Log("Waiting for bundle child '%s' to be deployed at version '%s' (looking for %v in namespace '%s')", ref.AppName, version, ref.Names, ref.Namespace)

	Eventually(func() (bool, error) {
		return isManagedAppAtVersion(ctx, ref, version)
	}).
		WithContext(ctx).
		WithPolling(10*time.Second).
		Should(BeTrue(), "bundle child '%s' was not deployed at version '%s'", ref.AppName, version)
}

// uninstallBundleHelmRelease removes the parent bundle and everything it installed.
//
// The children are not deleted directly: they belong to the parent's Helm release, so
// uninstalling the parent removes the child App CRs / HelmReleases and their own controllers
// then uninstall them from the workload cluster. The parent's uninstall does not block on the
// children's finalizers, so both are waited for before the cluster is torn down.
func (s *suite) uninstallBundleHelmRelease(ctx context.Context) {
	GinkgoHelper()

	cfg := s.bundleHelmReleaseConfig("")

	logger.Log("Uninstalling bundle HelmRelease %s/%s", cfg.Namespace, cfg.Name)
	err := client.DeleteHelmRelease(ctx, cfg.Name, cfg.Namespace)
	Expect(err).NotTo(HaveOccurred())

	deleteCtx, cancel := context.WithTimeout(ctx, bundleUninstallTimeout)
	defer cancel()
	Expect(client.WaitForHelmReleaseDeleted(deleteCtx, cfg.Name, cfg.Namespace)).To(Succeed())

	s.waitForBundleChildDeleted(ctx)

	err = client.DeleteHelmSource(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())
}

// waitForBundleChildDeleted waits, best effort, for the bundle's child to disappear.
//
// Best effort because the suite has already passed or failed by this point and there is nothing
// left to assert: the wait only exists to keep the workload cluster alive long enough for the
// child's own uninstall to reach it. Giving up is logged rather than failed.
func (s *suite) waitForBundleChildDeleted(ctx context.Context) {
	ref := s.bundleChildRef()

	waitCtx, cancel := context.WithTimeout(ctx, bundleChildCleanupTimeout)
	defer cancel()

	for {
		hr, err := findManagedHelmRelease(waitCtx, ref)
		if err == nil && hr == nil {
			appCR, err := findManagedAppCR(waitCtx, ref)
			if err == nil && appCR == nil {
				return
			}
		}

		logger.Log("Waiting for bundle child '%s' to be removed from namespace '%s'", ref.AppName, ref.Namespace)

		select {
		case <-waitCtx.Done():
			logger.Log("Gave up waiting for bundle child '%s' to be removed: %v", ref.AppName, waitCtx.Err())
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// bundleParentHelmRelease fetches the parent bundle's HelmRelease.
func (s *suite) bundleParentHelmRelease(ctx context.Context) (*helmv2.HelmRelease, error) {
	hr := &helmv2.HelmRelease{}
	key := types.NamespacedName{
		Name:      s.bundleHelmReleaseName(),
		Namespace: state.GetCluster().Organization.GetNamespace(),
	}
	err := state.GetFramework().MC().Get(ctx, key, hr)
	return hr, err
}
