package suite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/apiextensions-application/api/v1alpha1"
	applicationv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/apptest-framework/pkg/client"
	"github.com/giantswarm/apptest-framework/pkg/config"
	"github.com/giantswarm/apptest-framework/pkg/state"
	"github.com/giantswarm/cluster-standup-teardown/pkg/clusterbuilder"
	"github.com/giantswarm/cluster-standup-teardown/pkg/standup"
	"github.com/giantswarm/cluster-standup-teardown/pkg/teardown"
	"github.com/giantswarm/clustertest"
	"github.com/giantswarm/clustertest/pkg/application"
	clusterclient "github.com/giantswarm/clustertest/pkg/client"
	"github.com/giantswarm/clustertest/pkg/logger"
	"github.com/giantswarm/clustertest/pkg/wait"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cr "sigs.k8s.io/controller-runtime/pkg/client"
)

type suite struct {
	// Set from TestConfig
	appName    string
	repoName   string
	appCatalog string

	valuesFile       string
	isUpgrade        bool
	installNamespace string

	afterClusterReady func()
	beforeUpgrade     func()
	tests             func()
}

// New create a new suite instance that allows configuring an App test suite
func New(testConfig config.TestConfig) *suite {
	return &suite{
		appName:          testConfig.AppName,
		repoName:         testConfig.RepoName,
		appCatalog:       testConfig.AppCatalog,
		isUpgrade:        false,
		installNamespace: "default",
		valuesFile:       "./values.yaml",
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

// WithValuesFile sets a values.yaml file to use for the App values.
// If the file is not found an empty values file is uesd.
// If not set this default to `./values.yaml`
func (s *suite) WithValuesFile(valuesFile string) *suite {
	s.valuesFile, _ = filepath.Abs(valuesFile)
	return s
}

// AfterClusterReady allows configuring tests that will run as soon as the cluster is up and ready.
// This allows for running tests to check the current state of the cluster and
// assert that any pre-requisites are met.
func (s *suite) AfterClusterReady(fn func()) *suite {
	s.afterClusterReady = fn
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

		cb, err := clusterbuilder.GetClusterBuilderForContext(mcContext)
		Expect(err).NotTo(HaveOccurred())
		Expect(cb).NotTo(BeNil())

		// Load an existing cluster is env vars are set, otherwise create a new cluster
		cluster := clusterbuilder.LoadOrBuildCluster(state.GetFramework(), cb)
		Expect(cluster).NotTo(BeNil())
		state.SetCluster(cluster)

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
					logger.Log("Waiting for %q control plane nodes to be ready", replicas)
					_ = wait.For(
						wait.AreNumNodesReady(context.Background(), wcClient, 3, &cr.MatchingLabels{"node-role.kubernetes.io/control-plane": ""}),
						wait.WithTimeout(20*time.Minute),
						wait.WithInterval(15*time.Second),
					)
				}
			},
			func(wcClient *clusterclient.Client) {
				logger.Log("Waiting for worker nodes to be ready")
				_ = wait.For(
					wait.AreNumNodesReady(context.Background(), wcClient, 2, &cr.MatchingLabels{"node-role.kubernetes.io/worker": ""}),
					wait.WithTimeout(20*time.Minute),
					wait.WithInterval(15*time.Second),
				)
			},
			func(wcClient *clusterclient.Client) {
				logger.Log("Waiting for all default apps to be ready")
				defaultAppsAppName := fmt.Sprintf("%s-%s", state.GetCluster().Name, "default-apps")

				Eventually(wait.IsAppDeployed(state.GetContext(), state.GetFramework().MC(), defaultAppsAppName, state.GetCluster().Organization.GetNamespace())).
					WithTimeout(30 * time.Second).
					WithPolling(50 * time.Millisecond).
					Should(BeTrue())

				// Wait for all default-apps apps to be deployed
				appList := &v1alpha1.AppList{}
				err := state.GetFramework().MC().List(state.GetContext(), appList, cr.InNamespace(state.GetCluster().Organization.GetNamespace()), cr.MatchingLabels{"giantswarm.io/managed-by": defaultAppsAppName})
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

		// Create app
		app := application.New(fmt.Sprintf("%s-%s", cluster.Name, s.appName), s.appName).
			WithRepoName(s.repoName).
			WithCatalog(s.appCatalog).
			WithOrganization(*cluster.Organization).
			WithClusterName(cluster.Name).
			WithVersion(appVersion).
			WithInstallNamespace(s.installNamespace).
			MustWithValuesFile(s.valuesFile, &application.TemplateValues{}).
			WithInCluster(false)

		state.SetApplication(app)
	})

	AfterSuite(func() {
		app := state.GetApplication()
		logger.Log("Uninstalling App %s", app.AppName)
		err := state.GetFramework().MC().DeleteApp(state.GetContext(), *app)
		Expect(err).NotTo(HaveOccurred())

		logger.Log("Deleting workload cluster")
		err = teardown.New(state.GetFramework()).Teardown(state.GetCluster())
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("", func() {
		if s.afterClusterReady != nil {
			Describe("After Cluster Ready", s.afterClusterReady)
		}

		// TODO: Exclude this if default app
		It("Ensure app isn't already installed", func() {
			appCR := state.GetApplication()

			logger.Log("Checking that App %s isn't already installed", appCR.AppName)

			app := &applicationv1alpha1.App{
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

					app := state.GetApplication().WithVersion("latest")

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
				app := state.GetApplication()

				ctx, cancel := context.WithTimeout(state.GetContext(), 5*time.Minute)
				defer cancel()
				client.InstallApp(ctx, app)
			})
		})

		if s.tests != nil {
			Describe("App Tests", s.tests)
		}
	})

	RunSpecs(t, suiteName)
}
