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

	"github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/cluster-standup-teardown/v2/pkg/clusterbuilder"
	"github.com/giantswarm/cluster-standup-teardown/v2/pkg/standup"
	"github.com/giantswarm/cluster-standup-teardown/v2/pkg/teardown"
	"github.com/giantswarm/clustertest/v2"
	"github.com/giantswarm/clustertest/v2/pkg/application"
	clusterclient "github.com/giantswarm/clustertest/v2/pkg/client"
	"github.com/giantswarm/clustertest/v2/pkg/logger"
	"github.com/giantswarm/clustertest/v2/pkg/organization"
	"github.com/giantswarm/clustertest/v2/pkg/wait"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cr "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/apptest-framework/v2/pkg/bundles"
	"github.com/giantswarm/apptest-framework/v2/pkg/client"
	"github.com/giantswarm/apptest-framework/v2/pkg/config"
	"github.com/giantswarm/apptest-framework/v2/pkg/state"
)

type suite struct {
	// Set from TestConfig
	appName     string
	installName string
	repoName    string
	appCatalog  string

	valuesFile       string
	isUpgrade        bool
	installNamespace string

	isMCTest bool

	inBundleApp             string
	inBundleAppOverrideType bundles.AppNameOverrideType
	isDefaultApp            bool

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
		inBundleApp:             "",
		inBundleAppOverrideType: bundles.AppNameOverrideAuto,
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
// If not set this defaults to the `appName` value.
func (s *suite) WithInstallName(name string) *suite {
	s.installName = name
	return s
}

// WithValuesFile sets a values.yaml file to use for the App values.
// If the file is not found an empty values file is uesd.
// If not set this default to `./values.yaml`
func (s *suite) WithValuesFile(valuesFile string) *suite {
	s.valuesFile, _ = filepath.Abs(valuesFile)
	return s
}

// InBundleApp sets this test suite to install the App via the provided bundle App by setting the
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

// BeforeYpgrade allows configuring tests that will run after the App is installed
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
		os.Setenv("E2E_APP_VERSION", latestVersion)

		defer (func() {
			// Set the env back to latest so it doesn't conflict with other suites
			os.Setenv("E2E_APP_VERSION", "latest")
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
			WithInCluster(false)
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
			bundleVersion, err := application.GetLatestAppVersion(s.inBundleApp)
			Expect(err).ToNot(HaveOccurred())
			bundleVersion = strings.TrimPrefix(bundleVersion, "v")

			bundleApp := application.New(fmt.Sprintf("%s-%s", cluster.Name, s.inBundleApp), s.inBundleApp).
				WithCatalog(s.appCatalog).
				WithOrganization(*cluster.Organization).
				WithClusterName(cluster.Name).
				WithVersion(bundleVersion).
				WithInstallNamespace(cluster.Organization.GetNamespace()).
				MustWithValues(fmt.Sprintf("clusterID: %s", cluster.Name), &application.TemplateValues{}).
				WithInCluster(true)

			// Replace app with bundle app that has version of child App set
			bundleApp, err = bundles.OverrideChildApp(bundleApp, app, s.inBundleAppOverrideType)
			Expect(err).NotTo(HaveOccurred())
			state.SetBundleApplication(bundleApp)

			s.isDefaultApp, err = cluster.IsDefaultApp(*bundleApp)
			Expect(err).NotTo(HaveOccurred())
			if s.isDefaultApp && !s.isUpgrade {
				// If we're not an upgrade suite we install the override default app at creation
				cluster = cluster.WithAppOverride(*bundleApp)
			}
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
					skipDefaultAppsApp, err := state.GetCluster().UsesUnifiedClusterApp()
					Expect(err).NotTo(HaveOccurred())

					logger.Log("Waiting for all default apps to be ready")

					defaultAppsSelectorLabels := cr.MatchingLabels{
						"giantswarm.io/cluster":        state.GetCluster().Name,
						"app.kubernetes.io/managed-by": "Helm",
					}

					if !skipDefaultAppsApp {
						defaultAppsAppName := fmt.Sprintf("%s-%s", state.GetCluster().Name, "default-apps")

						Eventually(wait.IsAppDeployed(state.GetContext(), state.GetFramework().MC(), defaultAppsAppName, state.GetCluster().Organization.GetNamespace())).
							WithTimeout(30 * time.Second).
							WithPolling(50 * time.Millisecond).
							Should(BeTrue())

						defaultAppsSelectorLabels = cr.MatchingLabels{
							"giantswarm.io/managed-by": defaultAppsAppName,
						}
					}

					// Wait for all default-apps apps to be deployed
					appList := &v1alpha1.AppList{}
					err = state.GetFramework().MC().List(state.GetContext(), appList, cr.InNamespace(state.GetCluster().Organization.GetNamespace()), defaultAppsSelectorLabels)
					Expect(err).NotTo(HaveOccurred())

					appNamespacedNames := []types.NamespacedName{}
					for _, app := range appList.Items {
						appNamespacedNames = append(appNamespacedNames, types.NamespacedName{Name: app.Name, Namespace: app.Namespace})
					}

					Eventually(wait.IsAllAppDeployed(state.GetContext(), state.GetFramework().MC(), appNamespacedNames)).
						WithTimeout(15 * time.Minute).
						WithPolling(10 * time.Second).
						Should(BeTrue())
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

		By("Uninstalling App", func() {
			if s.isDefaultApp {
				Skip("App is a default app - skipping")
				return
			}

			app := getInstallApp()
			logger.Log("Uninstalling App %s (%s)", app.AppName, app.InstallName)
			err := state.GetFramework().MC().DeleteApp(state.GetContext(), *app)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("", func() {
		if s.afterClusterReady != nil {
			Describe("After Cluster Ready", s.afterClusterReady)
		}

		It("Ensure app isn't already installed", func() {
			if s.isDefaultApp {
				Skip("App is a default app - skipping")
				return
			}

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
		})

		if s.isUpgrade {
			Describe("Install previous version of app", func() {
				It("Install the latest release of the application", func() {
					if s.isDefaultApp {
						Skip("App is a default app - skipping")
						return
					}

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
				})
			})

			if s.beforeUpgrade != nil {
				Describe("Before upgrade", s.beforeUpgrade)
			}
		}

		Describe("Install app", func() {
			It("Install the application with the version to test", func() {
				if s.isDefaultApp && s.isUpgrade {
					// If we're testing the upgrade of a default app we need to do so via a release upgrade
					cluster := state.GetCluster()
					app := state.GetApplication()
					bundleApp := state.GetBundleApplication()
					if bundleApp != nil {
						cluster = cluster.WithAppOverride(*bundleApp)
					} else {
						cluster = cluster.WithAppOverride(*app)
					}

					ctx, cancel := context.WithTimeout(state.GetContext(), 10*time.Minute)
					defer cancel()
					_, err := state.GetFramework().ApplyCluster(ctx, cluster)
					Expect(err).ToNot(HaveOccurred())

				} else if s.isDefaultApp {
					Skip("App is a default app - skipping")
					return
				} else {
					app := getInstallApp()

					ctx, cancel := context.WithTimeout(state.GetContext(), 5*time.Minute)
					defer cancel()
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

// getInstallApp returns the bundle App if it's set, otherwise it returns the App
func getInstallApp() *application.Application {
	bundleApp := state.GetBundleApplication()
	if bundleApp != nil {
		return bundleApp
	}
	return state.GetApplication()
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
