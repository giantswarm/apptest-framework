package suite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck
	. "github.com/onsi/gomega"    //nolint:staticcheck

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/cluster-standup-teardown/v6/pkg/clusterbuilder"
	"github.com/giantswarm/cluster-standup-teardown/v6/pkg/standup"
	"github.com/giantswarm/cluster-standup-teardown/v6/pkg/teardown"
	"github.com/giantswarm/clustertest/v5"
	"github.com/giantswarm/clustertest/v5/pkg/application"
	clusterclient "github.com/giantswarm/clustertest/v5/pkg/client"
	"github.com/giantswarm/clustertest/v5/pkg/logger"
	"github.com/giantswarm/clustertest/v5/pkg/organization"
	"github.com/giantswarm/clustertest/v5/pkg/wait"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cr "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/apptest-framework/v5/pkg/bundles"
	"github.com/giantswarm/apptest-framework/v5/pkg/client"
	"github.com/giantswarm/apptest-framework/v5/pkg/config"
	"github.com/giantswarm/apptest-framework/v5/pkg/state"
)

// installMode describes who installs the app under test. It determines every
// install, upgrade, pre-check and uninstall step in the suite.
type installMode int

const (
	// installModeApp: the framework creates an App CR itself.
	installModeApp installMode = iota
	// installModeHelmRelease: the framework creates an OCIRepository + HelmRelease itself.
	installModeHelmRelease
	// installModeDefaultApp: the cluster chart owns the resource (an App CR on older
	// cluster charts, a HelmRelease on current ones). The framework never creates,
	// updates or deletes it. The version under test is driven through the Release CR
	// and the suite only asserts on the result, so WithHelmRelease does not apply.
	installModeDefaultApp
)

const (
	// defaultAppInstallTimeout bounds the wait for a default app to be at the version under
	// test after the cluster came up. The cluster is already reported ready at this point, so
	// this only has to absorb the cluster chart's reconcile lag.
	defaultAppInstallTimeout = 5 * time.Minute
	// defaultAppUpgradeTimeout bounds the wait for the cluster chart to reconcile a default
	// app to the version of a newly applied Release.
	defaultAppUpgradeTimeout = 15 * time.Minute
)

// installMode returns the mode this suite runs in.
//
// Being a default app wins over WithHelmRelease: the cluster chart, not the suite,
// decides whether a default app is rendered as an App CR or a HelmRelease, and
// writing to that resource would fight the cluster chart's own reconciliation.
//
// This must only be called from inside a spec or setup node. isDefaultApp is resolved
// in BeforeSuite, so it is not yet known when the spec tree is built.
func (s *suite) installMode() installMode {
	switch {
	case s.isDefaultApp:
		return installModeDefaultApp
	case s.useHelmRelease:
		return installModeHelmRelease
	default:
		return installModeApp
	}
}

type suite struct {
	// Set from TestConfig
	appName     string
	installName string
	repoName    string
	appCatalog  string

	valuesFile       string
	bundleValuesFile string
	isUpgrade        bool
	installNamespace string
	inCluster        bool

	isMCTest bool

	inBundleApp             string
	inBundleAppOverrideType bundles.AppNameOverrideType
	isDefaultApp            bool
	defaultAppName          string
	bundleValuesConfigMap   string

	// HelmRelease mode
	useHelmRelease           bool
	helmSourceKind           client.SourceKind
	helmSourceName           string
	helmSourceNamespace      string
	helmSourceURL            string
	helmChartName            string
	helmTargetNamespace      string
	helmStorageNamespace     string
	helmReleaseName          string
	helmTimeout              time.Duration
	helmRetries              *int
	helmServiceAccountName   string
	helmKubeConfigSecretName string

	afterClusterReady func()
	beforeUpgrade     func()
	tests             func()
	afterSuite        func()
}

// New create a new suite instance that allows configuring an App test suite
func New() *suite {
	testConfig := config.MustLoad()
	return &suite{
		appName:                 testConfig.AppName,
		installName:             testConfig.AppName,
		repoName:                testConfig.RepoName,
		appCatalog:              testConfig.AppCatalog,
		isMCTest:                testConfig.IsMCTest,
		isUpgrade:               false,
		isDefaultApp:            false,
		installNamespace:        "default",
		valuesFile:              "./values.yaml",
		bundleValuesFile:        "./bundle_values.yaml",
		inBundleApp:             "",
		inBundleAppOverrideType: bundles.AppNameOverrideAuto,
		inCluster:               false,
	}
}

// WithIsUpgrade sets if the current test suite is an upgrade test.
// Setting this to true will ensure the latest released version of the App is
// installed first before upgrading it to the test version.
// If not set this defaults to `false`.
func (s *suite) WithIsUpgrade(isUpgrade bool) *suite {
	s.isUpgrade = isUpgrade
	return s
}

// WithInstallNamespace sets the namespace to install the App into.
// If not set this defaults to the `default` namespapce.
func (s *suite) WithInstallNamespace(namespace string) *suite {
	s.installNamespace = namespace
	return s
}

// WithInstallName sets the name to install the App as (prefixed with the cluster name).
// If not set this defaults to the `appName` or `appBundleName` value.
func (s *suite) WithInstallName(name string) *suite {
	s.installName = name
	return s
}

// WithInCluster sets if the App should be installed in the cluster.
// If not set this defaults to `false`.
func (s *suite) WithInCluster(inCluster bool) *suite {
	s.inCluster = inCluster
	return s
}

// WithValuesFile sets a values.yaml file to use for the App values.
// If the file is not found an empty values file is uesd.
// If not set this default to `./values.yaml`
func (s *suite) WithValuesFile(valuesFile string) *suite {
	s.valuesFile, _ = filepath.Abs(valuesFile)
	return s
}

// WithBundleValuesFile sets a bundle_values.yaml file to use for the bundle App values.
// If the file is not found it is ignored.
// If not set this defaults to `./bundle_values.yaml`
func (s *suite) WithBundleValuesFile(valuesFile string) *suite {
	s.bundleValuesFile, _ = filepath.Abs(valuesFile)
	return s
}

// InAppBundle sets this test suite to install the App via the provided bundle App by setting the
// appropriate chart values
func (s *suite) InAppBundle(appBundleName string) *suite {
	s.inBundleApp = strings.ToLower(appBundleName)
	return s
}

// WithBundleOverrideType sets the naming convention for the child app in the bundle values.
// If not set, it defaults to AppNameOverrideAuto which auto-detects based on the bundle app name.
func (s *suite) WithBundleOverrideType(overrideType bundles.AppNameOverrideType) *suite {
	s.inBundleAppOverrideType = overrideType
	return s
}

// WithDefaultAppName sets the name, without the cluster name prefix, of the App CR or
// HelmRelease the cluster chart owns for the app under test.
//
// This only applies when the app under test is a default app of the Release. The cluster
// chart names that resource `<cluster>-<appName>`, and the framework also tries
// `<cluster>-<chartName>` for the few apps rendered from a hand-written template that uses
// the published chart name instead (`<cluster>-aws-ebs-csi-driver-bundle` for the app named
// `aws-ebs-csi-driver`). Set this only for an app named after neither.
func (s *suite) WithDefaultAppName(name string) *suite {
	s.defaultAppName = name
	return s
}

// WithHelmRelease configures the suite to use a Flux HelmRelease CR instead of a Giant Swarm App CR.
// When enabled, the framework will create a HelmRelease resource referencing the configured
// source and chart, and wait for the Ready condition instead of the App deployed status.
// Defaults to using an OCIRepository source — use WithHelmSourceKind to change.
//
// This only applies when the framework is the one installing the app. If the app under
// test is a default app of the Release being tested, the cluster chart installs it (as a
// HelmRelease on current cluster charts) and this setting is ignored.
func (s *suite) WithHelmRelease(useHelmRelease bool) *suite {
	s.useHelmRelease = useHelmRelease
	return s
}

// WithHelmSourceKind sets the kind of source reference for HelmRelease mode.
// Use client.SourceKindOCIRepository (default) or client.SourceKindHelmRepository.
func (s *suite) WithHelmSourceKind(kind client.SourceKind) *suite {
	s.helmSourceKind = kind
	return s
}

// WithHelmSourceName sets the name of the source reference (OCIRepository or HelmRepository)
// for HelmRelease mode. This must match an existing source CR in the cluster.
func (s *suite) WithHelmSourceName(name string) *suite {
	s.helmSourceName = name
	return s
}

// WithHelmSourceNamespace sets the namespace of the source reference.
// If not set, defaults to the HelmRelease namespace.
func (s *suite) WithHelmSourceNamespace(namespace string) *suite {
	s.helmSourceNamespace = namespace
	return s
}

// WithHelmChartName sets the chart name to use in the HelmRelease spec.
// This is the name of the chart as it appears in the source (OCIRepository or HelmRepository).
// Defaults to appName. Set this when the chart name in the registry differs from the app install name.
func (s *suite) WithHelmChartName(name string) *suite {
	s.helmChartName = name
	return s
}

// WithHelmSourceURL sets the URL of the Helm source to create automatically.
// For SourceKindHelmRepository, use an OCI URL like "oci://registry/path" or an HTTPS URL.
// For SourceKindOCIRepository, use an OCI URL like "oci://registry/path/chart".
// When set, the framework will create the source CR before installing the HelmRelease.
func (s *suite) WithHelmSourceURL(url string) *suite {
	s.helmSourceURL = url
	return s
}

// WithHelmTargetNamespace sets the target namespace where the Helm chart will be installed.
// This maps to the HelmRelease spec.targetNamespace field.
// If not set, the chart is installed in the HelmRelease's own namespace.
func (s *suite) WithHelmTargetNamespace(namespace string) *suite {
	s.helmTargetNamespace = namespace
	return s
}

// WithHelmStorageNamespace sets the namespace used for Helm storage.
// This maps to the HelmRelease spec.storageNamespace field.
func (s *suite) WithHelmStorageNamespace(namespace string) *suite {
	s.helmStorageNamespace = namespace
	return s
}

// WithHelmReleaseName sets the Helm release name.
// This maps to the HelmRelease spec.releaseName field.
// If not set, defaults to the HelmRelease resource name.
func (s *suite) WithHelmReleaseName(name string) *suite {
	s.helmReleaseName = name
	return s
}

// WithHelmTimeout sets the timeout for Helm operations (install/upgrade).
// If not set, defaults to the Flux default (5m).
func (s *suite) WithHelmTimeout(timeout time.Duration) *suite {
	s.helmTimeout = timeout
	return s
}

// WithHelmRetries sets the number of retries for install/upgrade remediation.
// If not set, defaults to 10.
func (s *suite) WithHelmRetries(retries int) *suite {
	s.helmRetries = &retries
	return s
}

// WithHelmServiceAccountName sets the Kubernetes service account to impersonate
// when reconciling the HelmRelease, which installs the chart into the cluster the
// HelmRelease lives in rather than through the cluster's kubeconfig secret. Only needed
// for resources the management cluster itself must own, such as app bundles.
// The service account is auto-created if it doesn't exist, so it must be one that already
// holds the permissions to install the chart.
func (s *suite) WithHelmServiceAccountName(name string) *suite {
	s.helmServiceAccountName = name
	return s
}

// WithHelmKubeConfigSecretName sets the kubeconfig secret name used to reach the cluster.
// If not set, defaults to the cluster's own "{clusterName}-kubeconfig" secret, unless a
// service account to impersonate was set instead.
func (s *suite) WithHelmKubeConfigSecretName(name string) *suite {
	s.helmKubeConfigSecretName = name
	return s
}

// AfterClusterReady allows configuring tests that will run as soon as the cluster is up and ready.
// This allows for running tests to check the current state of the cluster and
// assert that any pre-requisites are met.
func (s *suite) AfterClusterReady(fn func()) *suite {
	s.afterClusterReady = fn
	return s
}

// AfterSuite allows configuring tests that will run during the cleanup / teardown stage after
// all tests have completed. This is performed before the App is uninstalled and before the
// workload cluster is deleted.
func (s *suite) AfterSuite(fn func()) *suite {
	s.afterSuite = fn
	return s
}

// BeforeUpgrade allows configuring tests that will run after the App is installed
// but before it is upgraded to the test version.
// This only runs if `WithIsUpgrade` has been called with `true`.
// This allows for running tests to check the App has finished installing / setting up
// and the upgrade is safe to be applied.
func (s *suite) BeforeUpgrade(fn func()) *suite {
	s.beforeUpgrade = fn
	return s
}

// Tests allows specifying all the tests to run against the App after it has finished
// installing (and upgrading if an upgrade test suite).
func (s *suite) Tests(fn func()) *suite {
	s.tests = fn
	return s
}

// Run setups up and runs the test suite and all provided tests.
//
// Note: Any test specs found within the calling module that aren't provided to the suite
// via `BeforeInstall`, `BeforeUpgrade` or `Tests` will still be run but their order is
// unpredictable and is not recommended.
func (s *suite) Run(t *testing.T, suiteName string) {
	RegisterFailHandler(Fail)

	// Ensure we use an actual semver version instead of "latest"
	if os.Getenv("E2E_APP_VERSION") == "latest" {
		latestVersion, err := application.GetLatestAppVersion(s.repoName)
		if err != nil {
			panic(err)
		}
		latestVersion = strings.TrimPrefix(latestVersion, "v")
		logger.Log("Overriding 'latest' version to '%s'", latestVersion)
		os.Setenv("E2E_APP_VERSION", latestVersion) // #nosec G104

		defer (func() {
			// Set the env back to latest so it doesn't conflict with other suites
			os.Setenv("E2E_APP_VERSION", "latest") // #nosec G104
		})()
	}

	BeforeSuite(func() {
		logger.LogWriter = GinkgoWriter

		mcKubeconfig := os.Getenv("E2E_KUBECONFIG")
		mcContext := os.Getenv("E2E_KUBECONFIG_CONTEXT")
		appVersion := os.Getenv("E2E_APP_VERSION")

		// Ensure all require env vars are set
		Expect(mcKubeconfig).ToNot(BeEmpty(), "`E2E_KUBECONFIG` must be set to the kubeconfig of the test MC")
		Expect(mcContext).ToNot(BeEmpty(), "`E2E_KUBECONFIG_CONTEXT` must be set to the context to use in the kubeconfig")
		Expect(appVersion).ToNot(BeEmpty(), "`E2E_APP_VERSION` must be set to version of the app to test against")

		state.SetContext(context.Background())

		// Setup client for conntecting to MC
		framework, err := clustertest.New(mcContext)
		Expect(err).NotTo(HaveOccurred())
		state.SetFramework(framework)

		var cluster *application.Cluster
		if s.isMCTest {
			cluster = &application.Cluster{
				Name:         state.GetFramework().MC().GetClusterName(),
				Organization: organization.New("giantswarm"),
			}
		} else {
			cb, err := clusterbuilder.GetClusterBuilderForContext(mcContext)
			Expect(err).NotTo(HaveOccurred())
			Expect(cb).NotTo(BeNil())

			// Load an existing cluster is env vars are set, otherwise create a new cluster
			cluster = clusterbuilder.LoadOrBuildCluster(state.GetFramework(), cb)
		}
		Expect(cluster).NotTo(BeNil())
		state.SetCluster(cluster)

		// Create app
		installName := s.installName
		if installName == "" {
			installName = s.appName
		}
		if !s.isMCTest {
			installName = fmt.Sprintf("%s-%s", cluster.Name, installName)
		}
		app := application.New(installName, s.appName).
			WithRepoName(s.repoName).
			WithCatalog(s.appCatalog).
			WithOrganization(*cluster.Organization).
			WithClusterName(cluster.Name).
			WithVersion(appVersion).
			WithInstallNamespace(s.installNamespace).
			MustWithValuesFile(s.valuesFile, &application.TemplateValues{}).
			WithInCluster(s.inCluster)
		state.SetApplication(app)

		if !s.isMCTest {
			s.isDefaultApp, err = cluster.IsDefaultApp(*app)
			Expect(err).NotTo(HaveOccurred())
			if s.isDefaultApp && !s.isUpgrade {
				// If we're not an upgrade suite we install the override default app at creation
				cluster = cluster.WithAppOverride(*app)
			}
		}

		if s.inBundleApp != "" {
			bundleVersion, bundleCatalog := s.resolveBundleVersion(cluster)

			bundleAppName := fmt.Sprintf("%s-%s", cluster.Name, s.inBundleApp)
			bundleApp := application.New(bundleAppName, s.inBundleApp).
				WithCatalog(bundleCatalog).
				WithOrganization(*cluster.Organization).
				WithClusterName(cluster.Name).
				WithVersion(bundleVersion).
				WithInstallNamespace(cluster.Organization.GetNamespace()).
				MustWithValues(fmt.Sprintf("clusterID: %s", cluster.Name), &application.TemplateValues{}).
				WithInCluster(true)

			// Replace app with bundle app that has version of child App set
			bundleApp, err := bundles.OverrideChildApp(bundleApp, app, s.inBundleAppOverrideType)
			Expect(err).NotTo(HaveOccurred())
			state.SetBundleApplication(bundleApp)

			s.isDefaultApp, err = cluster.IsDefaultApp(*bundleApp)
			Expect(err).NotTo(HaveOccurred())
			if s.isDefaultApp && !s.isUpgrade {
				// If we're not an upgrade suite we install the override default app at creation
				cluster = cluster.WithAppOverride(*bundleApp)
			}
		}

		if s.isDefaultApp && s.useHelmRelease {
			logger.Log("'%s' is a default app of the Release under test: WithHelmRelease is ignored, the cluster chart owns the App CR / HelmRelease and the version under test is driven through the Release", s.appName)
		}

		if s.isMCTest {
			logger.Log("Confirming that we're working with an ephemeral MC for this MC App test suite")

			logger.Log("MC Name: '%s', Test Cluster Name: '%s'", cleanClusterName(state.GetFramework().MC().GetClusterName()), cleanClusterName(state.GetCluster().Name))
			Expect(cleanClusterName(state.GetFramework().MC().GetClusterName()) == cleanClusterName(state.GetCluster().Name)).To(BeTrue(), "We're not pointing to the MC cluster but instead trying to use a WC")

			isEphemeral := isEphemeralTestMC()
			logger.Log("MC is ephemeral: '%t'", isEphemeral)
			Expect(isEphemeral).To(BeTrue(), "The MC being used for testing is not an ephemeral MC. Tests could cause side-effects so we block running on non-ephemeral")
		} else {
			// Create new workload cluster
			logger.Log("Creating new workload cluster")

			// We want to make sure the cluster is ready enough for us to install a new App
			// so we wait for all control plane nodes and at least 2 workers to be ready
			clusterReadyFns := []func(wcClient *clusterclient.Client){
				func(wcClient *clusterclient.Client) {
					replicas, err := state.GetFramework().GetExpectedControlPlaneReplicas(state.GetContext(), state.GetCluster().Name, state.GetCluster().GetNamespace())
					Expect(err).NotTo(HaveOccurred())

					// Only check for control plane if not a managed cluster (e.g. EKS)
					if replicas != 0 {
						logger.Log("Waiting for %d control plane nodes to be ready", replicas)
						_ = wait.For(
							wait.AreNumNodesReady(context.Background(), wcClient, int(replicas), &cr.MatchingLabels{"node-role.kubernetes.io/control-plane": ""}),
							wait.WithTimeout(20*time.Minute),
							wait.WithInterval(15*time.Second),
						)
					}
				},
				func(wcClient *clusterclient.Client) {
					logger.Log("Waiting for worker nodes to be ready")
					_ = wait.For(
						wait.AreNumNodesReady(context.Background(), wcClient, 2, clusterclient.DoesNotHaveLabels{"node-role.kubernetes.io/control-plane"}),
						wait.WithTimeout(20*time.Minute),
						wait.WithInterval(15*time.Second),
					)
				},
				func(wcClient *clusterclient.Client) {
					logger.Log("Waiting for all default apps to be ready")

					orgNamespace := state.GetCluster().Organization.GetNamespace()

					// Newer cluster charts deploy default apps as Flux HelmReleases instead of
					// App CRs. We support both by listing each kind and waiting for whatever
					// is present. Presence-based detection avoids hard-coding a version.
					defaultAppsSelectorLabels := cr.MatchingLabels{
						"giantswarm.io/cluster":        state.GetCluster().Name,
						"app.kubernetes.io/managed-by": "Helm",
					}

					appList := &v1alpha1.AppList{}
					err = state.GetFramework().MC().List(state.GetContext(), appList, cr.InNamespace(orgNamespace), defaultAppsSelectorLabels)
					Expect(err).NotTo(HaveOccurred())

					appNamespacedNames := []types.NamespacedName{}
					for _, app := range appList.Items {
						appNamespacedNames = append(appNamespacedNames, types.NamespacedName{Name: app.Name, Namespace: app.Namespace})
					}

					// Top-level default-app HelmReleases are not labelled the same way as App
					// CRs by the cluster chart, so list everything in the org namespace and
					// wait for all of them — matches the cluster-test-suites convention.
					hrList := &helmv2.HelmReleaseList{}
					err = state.GetFramework().MC().List(state.GetContext(), hrList, cr.InNamespace(orgNamespace))
					Expect(err).NotTo(HaveOccurred())

					hrNamespacedNames := []types.NamespacedName{}
					for _, hr := range hrList.Items {
						hrNamespacedNames = append(hrNamespacedNames, types.NamespacedName{Name: hr.Name, Namespace: hr.Namespace})
					}

					logger.Log("Found %d default App CR(s) and %d HelmRelease(s) to wait for in %s", len(appNamespacedNames), len(hrNamespacedNames), orgNamespace)

					if len(appNamespacedNames) > 0 {
						Eventually(wait.IsAllAppDeployed(state.GetContext(), state.GetFramework().MC(), appNamespacedNames)).
							WithTimeout(15 * time.Minute).
							WithPolling(10 * time.Second).
							Should(BeTrue())
					}

					if len(hrNamespacedNames) > 0 {
						Eventually(client.IsAllHelmReleasesReady(state.GetContext(), state.GetFramework().MC(), hrNamespacedNames)).
							WithTimeout(15 * time.Minute).
							WithPolling(10 * time.Second).
							Should(BeTrue())
					}
				},
			}

			cluster, err = standup.New(state.GetFramework(), false, clusterReadyFns...).Standup(cluster)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster).NotTo(BeNil())
			state.SetCluster(cluster)

			logger.Log("Workload cluster ready to use")
		}
	})

	AfterSuite(func() {
		defer func() {
			if !s.isMCTest {
				By("Deleting workload cluster", func() {
					// We defer this to ensure it happens even if uninstalling the app fails
					logger.Log("Deleting workload cluster")
					err := teardown.New(state.GetFramework()).Teardown(state.GetCluster())
					Expect(err).NotTo(HaveOccurred())
				})
			}
		}()

		if s.afterSuite != nil {
			By("User-provided After Suite", s.afterSuite)
		}

		if s.bundleValuesConfigMap != "" {
			By("Deleting bundle values ConfigMap", func() {
				app := getInstallApp()
				configMap := &corev1.ConfigMap{
					ObjectMeta: v1.ObjectMeta{
						Name:      s.bundleValuesConfigMap,
						Namespace: app.GetNamespace(),
					},
				}
				err := state.GetFramework().MC().Delete(state.GetContext(), configMap)
				if err != nil && !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())
				}
			})
		}

		By("Uninstalling App", func() {
			switch s.installMode() {
			case installModeDefaultApp:
				// Owned by the cluster chart, so it goes away with the cluster.
				logger.Log("App is a default app - nothing to uninstall")

			case installModeHelmRelease:
				installName := s.getHelmReleaseName()
				cfg := s.buildHelmReleaseConfig(installName, "")
				logger.Log("Uninstalling HelmRelease %s/%s", cfg.Namespace, installName)
				err := client.DeleteHelmRelease(state.GetContext(), installName, cfg.Namespace)
				Expect(err).NotTo(HaveOccurred())
				if cfg.SourceURL != "" {
					err = client.DeleteHelmSource(state.GetContext(), cfg)
					Expect(err).NotTo(HaveOccurred())
				}

			case installModeApp:
				app := getInstallApp()
				logger.Log("Uninstalling App %s (%s)", app.AppName, app.InstallName)
				err := state.GetFramework().MC().DeleteApp(state.GetContext(), *app)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})

	Describe("", func() {
		if s.afterClusterReady != nil {
			Describe("After Cluster Ready", s.afterClusterReady)
		}

		It("Ensure app isn't already installed", func() {
			switch s.installMode() {
			case installModeDefaultApp:
				Skip("App is a default app - installed by the cluster chart")

			case installModeHelmRelease:
				installName := s.getHelmReleaseName()
				cfg := s.buildHelmReleaseConfig(installName, "")
				logger.Log("Checking that HelmRelease %s isn't already installed", installName)

				hr := &helmv2.HelmRelease{
					ObjectMeta: v1.ObjectMeta{
						Name:      installName,
						Namespace: cfg.Namespace,
					},
				}
				err := state.GetFramework().MC().Get(state.GetContext(), cr.ObjectKeyFromObject(hr), hr)
				Expect(err).ToNot(BeNil())
				Expect(errors.IsNotFound(err)).To(BeTrue())

			case installModeApp:
				appCR := getInstallApp()

				logger.Log("Checking that App %s isn't already installed", appCR.AppName)

				app := &v1alpha1.App{
					ObjectMeta: v1.ObjectMeta{
						Name:      appCR.InstallName,
						Namespace: appCR.GetNamespace(),
					},
				}
				err := state.GetFramework().MC().Get(state.GetContext(), cr.ObjectKeyFromObject(app), app)
				Expect(err).ToNot(BeNil())
				Expect(errors.IsNotFound(err)).To(BeTrue())
			}
		})

		if s.isUpgrade {
			Describe("Install previous version of app", func() {
				It("Install the latest release of the application", func() {
					switch s.installMode() {
					case installModeDefaultApp:
						// The cluster was created without the app override, so it already came
						// up with the version the Release pins.
						Skip("App is a default app - the previous version was installed with the cluster")

					case installModeHelmRelease:
						latestVersion, err := application.GetLatestAppVersion(s.repoName)
						Expect(err).NotTo(HaveOccurred())
						latestVersion = strings.TrimPrefix(latestVersion, "v")

						installName := s.getHelmReleaseName()

						ctx, cancel := context.WithTimeout(state.GetContext(), s.getHelmInstallTimeout())
						defer cancel()

						cfg := s.buildHelmReleaseConfig(installName, latestVersion)
						client.InstallHelmRelease(ctx, cfg)

					case installModeApp:
						var app *application.Application
						if s.inBundleApp != "" {
							cluster := state.GetCluster()
							app = application.New(fmt.Sprintf("%s-%s", cluster.Name, s.inBundleApp), s.inBundleApp).
								WithCatalog(s.appCatalog).
								WithOrganization(*cluster.Organization).
								WithClusterName(cluster.Name).
								WithVersion("latest").
								WithInstallNamespace(cluster.Organization.GetNamespace()).
								MustWithValues(fmt.Sprintf("clusterID: %s", cluster.Name), &application.TemplateValues{}).
								WithInCluster(true)
						} else {
							app = state.GetApplication().WithVersion("latest")
						}

						ctx, cancel := context.WithTimeout(state.GetContext(), 5*time.Minute)
						defer cancel()
						client.InstallApp(ctx, app)
					}
				})
			})

			if s.beforeUpgrade != nil {
				Describe("Before upgrade", s.beforeUpgrade)
			}
		}

		Describe("Install app", func() {
			It("Install the application with the version to test", func() {
				switch s.installMode() {
				case installModeDefaultApp:
					if !s.isUpgrade {
						// The version under test was applied as a Release app override when the
						// cluster was created, so there is nothing left to install here. Assert
						// the override took effect instead of assuming it did: nothing else
						// checks the version a default app came up at.
						ctx, cancel := context.WithTimeout(state.GetContext(), defaultAppInstallTimeout)
						defer cancel()
						s.waitForDefaultApp(ctx)
						return
					}

					// Upgrading a default app is a Release upgrade. The app's own App CR /
					// HelmRelease is owned by the cluster chart and must not be written to.
					cluster := state.GetCluster()
					app := getInstallApp()

					ctx, cancel := context.WithTimeout(state.GetContext(), 10*time.Minute)
					defer cancel()
					_, err := state.GetFramework().ApplyCluster(ctx, cluster.WithAppOverride(*app))
					Expect(err).ToNot(HaveOccurred())

					// ApplyCluster returns once the updated Release is applied and the cluster
					// is still ready, which says nothing about the app itself. Wait for the
					// cluster chart to reconcile the app to the version under test, otherwise
					// the tests run against the pre-upgrade version.
					waitCtx, waitCancel := context.WithTimeout(state.GetContext(), defaultAppUpgradeTimeout)
					defer waitCancel()
					s.waitForDefaultApp(waitCtx)

				case installModeHelmRelease:
					appVersion := os.Getenv("E2E_APP_VERSION")
					Expect(appVersion).NotTo(BeEmpty(), "E2E_APP_VERSION must be set for HelmRelease tests")
					installName := s.getHelmReleaseName()

					ctx, cancel := context.WithTimeout(state.GetContext(), s.getHelmInstallTimeout())
					defer cancel()

					cfg := s.buildHelmReleaseConfig(installName, appVersion)

					if s.isUpgrade {
						// Upgrade: update the existing HelmRelease version
						client.UpdateHelmReleaseVersion(ctx, cfg, appVersion)
					} else {
						client.InstallHelmRelease(ctx, cfg)
					}

					// Wait for the HelmRelease to be ready at the expected version
					Eventually(func() (bool, error) {
						ready, err := client.IsHelmReleaseReady(state.GetContext(), installName, cfg.Namespace)
						if !ready || err != nil {
							return false, err
						}
						return client.IsHelmReleaseVersion(state.GetContext(), installName, cfg.Namespace, appVersion)
					}).
						WithContext(ctx).
						WithPolling(5 * time.Second).
						Should(BeTrue())

				case installModeApp:
					app := getInstallApp()

					ctx, cancel := context.WithTimeout(state.GetContext(), 5*time.Minute)
					defer cancel()

					if state.GetBundleApplication() != nil {
						if _, err := os.Stat(s.bundleValuesFile); err == nil {
							bundleValuesContent, err := os.ReadFile(s.bundleValuesFile)
							Expect(err).NotTo(HaveOccurred())

							configMapName := fmt.Sprintf("%s-bundle-values", app.InstallName)
							configMap := &corev1.ConfigMap{
								TypeMeta: v1.TypeMeta{
									Kind:       "ConfigMap",
									APIVersion: "v1",
								},
								ObjectMeta: v1.ObjectMeta{
									Name:      configMapName,
									Namespace: app.GetNamespace(),
								},
								Data: map[string]string{
									"values": string(bundleValuesContent),
								},
							}
							err = state.GetFramework().MC().CreateOrUpdate(ctx, configMap)
							Expect(err).NotTo(HaveOccurred())
							s.bundleValuesConfigMap = configMapName

							app = app.WithExtraConfigs([]v1alpha1.AppExtraConfig{
								{
									Kind:      "configMap",
									Name:      configMapName,
									Namespace: app.GetNamespace(),
									Priority:  25,
								},
							})
						}
					}

					client.InstallApp(ctx, app)
				}
			})
		})

		if s.tests != nil {
			Describe("App Tests", s.tests)
		}
	})

	RunSpecs(t, suiteName)
}

// defaultAppRef identifies the App CR or HelmRelease the cluster chart owns for the default
// app under test.
type defaultAppRef struct {
	// AppName is the Giant Swarm app name, as it appears in the Release CR.
	AppName string
	// Namespace is the cluster's organization namespace, where the cluster chart renders
	// the resource.
	Namespace string
	// Names are the candidate resource names, most likely first.
	Names []string
}

// resolveDefaultAppRef builds the reference to the resource the cluster chart owns for the
// app under test.
//
// The cluster chart names it `<cluster>-<appName>`, but a handful of apps are rendered from
// a hand-written template that uses the published chart name instead
// (`<cluster>-aws-ebs-csi-driver-bundle` for the app named `aws-ebs-csi-driver`), so the
// chart name is tried as a second candidate. WithDefaultAppName overrides both.
//
// For an app installed through a bundle it is the bundle that is the default app, and the
// chart name belongs to the child, so it is not a candidate: `<cluster>-<childApp>` would
// match the bundle's own child resource and be compared against the bundle's version.
func (s *suite) resolveDefaultAppRef() defaultAppRef {
	cluster := state.GetCluster()
	app := getInstallApp()

	chartName := s.getHelmChartName()
	if state.GetBundleApplication() != nil {
		// The chart name belongs to the child, not to the bundle that is the default app.
		chartName = ""
	}

	return defaultAppRef{
		AppName:   app.AppName,
		Namespace: cluster.GetNamespace(),
		Names:     defaultAppResourceNames(cluster.Name, app.AppName, chartName, s.defaultAppName),
	}
}

// defaultAppResourceNames returns the candidate names of the cluster chart's resource for the
// app, most likely first. An override, when given, is the only candidate. chartName may be
// empty when the app has no chart name distinct from its app name, or when it would name
// something other than the resource under test.
func defaultAppResourceNames(clusterName, appName, chartName, override string) []string {
	prefixed := func(name string) string {
		return fmt.Sprintf("%s-%s", clusterName, name)
	}

	if override != "" {
		return []string{prefixed(override)}
	}

	names := []string{prefixed(appName)}
	if chartName != "" && chartName != appName {
		names = append(names, prefixed(chartName))
	}
	return names
}

// waitForDefaultApp waits until the resource the cluster chart owns for the default app under
// test is deployed at the version under test.
//
// Both the install and the upgrade of a default app happen through the Release CR, which
// nothing else asserts on: the standup wait only checks that every default app is ready, at
// whatever version, and ApplyCluster returns once the cluster is still ready. Without this
// wait a Release app override that silently did not take effect leaves the suite testing the
// version the Release pins while believing it tested the override.
func (s *suite) waitForDefaultApp(ctx context.Context) {
	GinkgoHelper()

	app := getInstallApp()
	builtApp, _, err := app.Build()
	Expect(err).NotTo(HaveOccurred())
	version := strings.TrimPrefix(builtApp.Spec.Version, "v")

	ref := s.resolveDefaultAppRef()
	logger.Log("Waiting for default app '%s' to be deployed at version '%s' (looking for %v in namespace '%s')", ref.AppName, version, ref.Names, ref.Namespace)

	Eventually(func() (bool, error) {
		return isDefaultAppAtVersion(ctx, ref, version)
	}).
		WithContext(ctx).
		WithPolling(10*time.Second).
		Should(BeTrue(), "default app '%s' was not deployed at version '%s'", ref.AppName, version)
}

// isDefaultAppAtVersion reports whether the resource the cluster chart owns for the app under
// test is deployed at the given version.
//
// Which kind that resource is depends on the cluster chart generation, so both are looked for
// and whichever exists is used. A HelmRelease wins: while a cluster chart migrates an app from
// an App CR to a HelmRelease both exist for a while, and the HelmRelease is the one that
// survives.
func isDefaultAppAtVersion(ctx context.Context, ref defaultAppRef, version string) (bool, error) {
	hr, err := findDefaultAppHelmRelease(ctx, ref)
	if err != nil {
		return false, err
	}
	if hr != nil {
		atVersion, err := client.IsHelmReleaseVersion(ctx, hr.Name, hr.Namespace, version)
		if err != nil || !atVersion {
			return false, err
		}
		return client.IsHelmReleaseReady(ctx, hr.Name, hr.Namespace)
	}

	appCR, err := findDefaultAppCR(ctx, ref)
	if err != nil {
		return false, err
	}
	if appCR != nil {
		if requested := strings.TrimPrefix(appCR.Spec.Version, "v"); requested != version {
			logger.Log("App '%s/%s' doesn't request version '%s' yet: spec.version='%s'", appCR.Namespace, appCR.Name, version, requested)
			return false, nil
		}
		if deployed := strings.TrimPrefix(appCR.Status.Version, "v"); deployed != version {
			logger.Log("App '%s/%s' is not yet at version '%s': status.version='%s'", appCR.Namespace, appCR.Name, version, deployed)
			return false, nil
		}
		return wait.IsAppDeployed(ctx, state.GetFramework().MC(), appCR.Name, appCR.Namespace)()
	}

	logger.Log("No HelmRelease or App CR for default app '%s' found in namespace '%s' yet", ref.AppName, ref.Namespace)
	return false, nil
}

// findDefaultAppHelmRelease looks for the cluster chart's HelmRelease for the app under test,
// by name first and then by the chart it pulls, so a cluster chart that names the resource
// after neither the app nor the chart is still found. Returns nil when there is none (yet).
func findDefaultAppHelmRelease(ctx context.Context, ref defaultAppRef) (*helmv2.HelmRelease, error) {
	mcClient := state.GetFramework().MC()

	for _, name := range ref.Names {
		hr := &helmv2.HelmRelease{}
		err := mcClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ref.Namespace}, hr)
		if err == nil {
			return hr, nil
		}
		if !errors.IsNotFound(err) {
			return nil, err
		}
	}

	hrList := &helmv2.HelmReleaseList{}
	if err := mcClient.List(ctx, hrList, cr.InNamespace(ref.Namespace)); err != nil {
		return nil, err
	}
	for i := range hrList.Items {
		hr := &hrList.Items[i]
		if hr.Spec.Chart != nil && hr.Spec.Chart.Spec.Chart == ref.AppName {
			return hr, nil
		}
		if hr.Spec.ChartRef != nil && hr.Spec.ChartRef.Name == ref.AppName {
			return hr, nil
		}
	}
	return nil, nil
}

// findDefaultAppCR looks for the cluster chart's App CR for the app under test, by name first
// and then by spec.name, which is the app name however the CR itself is named. Returns nil
// when there is none (yet).
func findDefaultAppCR(ctx context.Context, ref defaultAppRef) (*v1alpha1.App, error) {
	mcClient := state.GetFramework().MC()

	for _, name := range ref.Names {
		appCR := &v1alpha1.App{}
		err := mcClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ref.Namespace}, appCR)
		if err == nil {
			return appCR, nil
		}
		if !errors.IsNotFound(err) {
			return nil, err
		}
	}

	appList := &v1alpha1.AppList{}
	if err := mcClient.List(ctx, appList, cr.InNamespace(ref.Namespace)); err != nil {
		return nil, err
	}
	for i := range appList.Items {
		if appList.Items[i].Spec.Name == ref.AppName {
			return &appList.Items[i], nil
		}
	}
	return nil, nil
}

// getInstallApp returns the bundle App if it's set, otherwise it returns the App
func getInstallApp() *application.Application {
	bundleApp := state.GetBundleApplication()
	if bundleApp != nil {
		return bundleApp
	}
	return state.GetApplication()
}

// resolveBundleVersion determines the version and catalog of the bundle App to install.
//
// By default the bundle is pinned to the version shipped by the cluster's Release, so suites
// that install an app via a bundle test it as released instead of incidentally bumping the
// bundle to the latest published version (which may be ahead of any release and incompatible).
// An explicit override via E2E_OVERRIDE_VERSIONS for the bundle takes precedence. If the bundle
// is not part of the Release, it falls back to the latest published bundle version.
func (s *suite) resolveBundleVersion(cluster *application.Cluster) (version string, catalog string) {
	// 1. Explicit override via E2E_OVERRIDE_VERSIONS (e.g. when testing a bundle's own build).
	if v, c, ok := overrideVersionFor(s.inBundleApp); ok {
		if c == "" {
			c = s.appCatalog
		}
		logger.Log("Using overridden bundle version for '%s': %s (catalog: %s)", s.inBundleApp, v, c)
		return strings.TrimPrefix(v, "v"), c
	}

	// 2. Version pinned by the cluster's Release.
	if release, err := cluster.GetRelease(); err == nil && release != nil {
		for _, releaseApp := range release.Spec.Apps {
			if releaseApp.Name == s.inBundleApp {
				c := releaseApp.Catalog
				if c == "" {
					c = s.appCatalog
				}
				logger.Log("Using Release-pinned bundle version for '%s': %s (catalog: %s)", s.inBundleApp, releaseApp.Version, c)
				return strings.TrimPrefix(releaseApp.Version, "v"), c
			}
		}
	}

	// 3. Fallback: latest published bundle release.
	latest, err := application.GetLatestAppVersion(s.inBundleApp)
	Expect(err).ToNot(HaveOccurred())
	logger.Log("Bundle '%s' is not pinned in the Release; falling back to latest published version: %s", s.inBundleApp, latest)
	return strings.TrimPrefix(latest, "v"), s.appCatalog
}

// overrideVersionFor parses the E2E_OVERRIDE_VERSIONS environment variable and returns the
// version (and optional catalog) requested for the given app, if any. The format mirrors the
// one used by clustertest: a comma separated list of `app=version` or `app=version:catalog`.
func overrideVersionFor(appName string) (version string, catalog string, ok bool) {
	overrides := os.Getenv("E2E_OVERRIDE_VERSIONS")
	if overrides == "" {
		return "", "", false
	}
	for _, pair := range strings.Split(overrides, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(parts[0]), appName) {
			continue
		}
		value := strings.TrimSpace(parts[1])
		if idx := strings.LastIndex(value, ":"); idx != -1 {
			return strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx+1:]), true
		}
		return value, "", true
	}
	return "", "", false
}

func isEphemeralTestMC() bool {
	values := &application.ClusterValues{}

	clusterName := cleanClusterName(state.GetFramework().MC().GetClusterName())
	// It's possible that we connect to an MC while it is still being set up and not quite ready yet.
	// If we get an error while trying to get the Cluster values we'll retry for up to 2 minutes
	Eventually(func() error {
		logger.Log("Checking if MC '%s' is ephemeral", clusterName)
		return state.GetFramework().MC().GetHelmValues(clusterName, "org-giantswarm", values)
	}).
		WithTimeout(2 * time.Minute).
		WithPolling(5 * time.Second).
		Should(BeNil())

	return strings.Contains(values.BaseDomain, "ephemeral")
}

func cleanClusterName(clusterName string) string {
	return strings.TrimPrefix(clusterName, "teleport.giantswarm.io-")
}

// getHelmReleaseName returns the name to use for the HelmRelease CR.
func (s *suite) getHelmChartName() string {
	if s.helmChartName != "" {
		return s.helmChartName
	}
	return s.appName
}

func (s *suite) getHelmReleaseName() string {
	name := s.installName
	if name == "" {
		name = s.appName
	}
	cluster := state.GetCluster()
	if cluster != nil && !s.isMCTest {
		name = fmt.Sprintf("%s-%s", cluster.Name, name)
	}
	return name
}

// getHelmInstallTimeout returns the timeout to use for HelmRelease install/upgrade operations.
// Defaults to 10 minutes if not explicitly set via WithHelmTimeout.
func (s *suite) getHelmInstallTimeout() time.Duration {
	if s.helmTimeout > 0 {
		return s.helmTimeout
	}
	return 10 * time.Minute
}

// loadValues reads the values file and returns its content as a string.
// Returns an empty string if the file does not exist.
func (s *suite) loadValues() string {
	valuesPath := s.valuesFile
	if valuesPath == "" {
		return ""
	}
	content, err := os.ReadFile(valuesPath) // #nosec G304
	if err != nil {
		return ""
	}
	return string(content)
}

// buildHelmReleaseConfig constructs a HelmReleaseConfig from suite settings,
// applying the defaults a Giant Swarm cluster expects.
//
// A cluster's apps are installed through a HelmRelease that lives in the cluster's org
// namespace and reaches the cluster with the cluster's own kubeconfig secret. That holds
// for a management cluster too: the MC is a CAPI cluster like any other, self-managed in
// org-giantswarm, so an MC test needs the same shape rather than an in-cluster install.
// Only resources the MC itself must own (app bundles, for example) are installed
// in-cluster with an impersonated service account, which stays opt-in.
func (s *suite) buildHelmReleaseConfig(installName, chartVersion string) client.HelmReleaseConfig {
	cluster := state.GetCluster()
	namespace := s.installNamespace
	sourceNamespace := s.helmSourceNamespace
	kubeConfigSecret := s.helmKubeConfigSecretName
	serviceAccountName := s.helmServiceAccountName
	storageNamespace := s.helmStorageNamespace

	// Use cluster org namespace if default
	if namespace == "default" {
		namespace = cluster.Organization.GetNamespace()
		logger.Log("Auto-setting HelmRelease namespace to cluster org namespace: %s", namespace)
	}

	// Auto-set the cluster's kubeconfig secret unless the suite asked for an in-cluster
	// install by naming a service account to impersonate.
	if kubeConfigSecret == "" && serviceAccountName == "" {
		kubeConfigSecret = fmt.Sprintf("%s-kubeconfig", cleanClusterName(cluster.Name))
		logger.Log("Auto-setting kubeconfig secret: %s", kubeConfigSecret)
	}

	if kubeConfigSecret != "" && serviceAccountName != "" {
		// Setting both causes Flux to impersonate the service account on the MC instead
		// of using the kubeconfig, which fails.
		logger.Log("Ignoring service account '%s': the HelmRelease reaches the cluster through kubeconfig secret '%s'", serviceAccountName, kubeConfigSecret)
		serviceAccountName = ""
	}

	// Default storageNamespace to targetNamespace so Helm stores release secrets in the
	// cluster the chart is installed into (not the MC org namespace).
	if storageNamespace == "" {
		storageNamespace = s.helmTargetNamespace
		if storageNamespace != "" {
			logger.Log("Auto-setting HelmRelease storageNamespace to targetNamespace: %s", storageNamespace)
		}
	}

	return client.HelmReleaseConfig{
		Name:                 installName,
		Namespace:            namespace,
		TargetNamespace:      s.helmTargetNamespace,
		StorageNamespace:     storageNamespace,
		ReleaseName:          s.helmReleaseName,
		ChartName:            s.getHelmChartName(),
		ChartVersion:         chartVersion,
		SourceKind:           s.helmSourceKind,
		SourceName:           s.helmSourceName,
		SourceNamespace:      sourceNamespace,
		SourceURL:            s.helmSourceURL,
		Timeout:              s.helmTimeout,
		Retries:              s.helmRetries,
		ServiceAccountName:   serviceAccountName,
		KubeConfigSecretName: kubeConfigSecret,
		Values:               s.loadValues(),
	}
}
