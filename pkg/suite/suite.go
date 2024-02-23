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

	applicationv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/apptest-framework/pkg/client"
	"github.com/giantswarm/apptest-framework/pkg/config"
	"github.com/giantswarm/apptest-framework/pkg/state"
	"github.com/giantswarm/clustertest"
	"github.com/giantswarm/clustertest/pkg/application"
	"github.com/giantswarm/clustertest/pkg/logger"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	beforeInstall func()
	beforeUpgrade func()
	tests         func()
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

// BeforeInstall allows configuring tests that will run before the App is installed.
// This allows for running tests to check the current state of the cluster and
// assert that any pre-requisites are met.
func (s *suite) BeforeInstall(fn func()) *suite {
	s.beforeInstall = fn
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
		mcKubeconfig := os.Getenv("E2E_KUBECONFIG")
		mcContext := os.Getenv("E2E_KUBECONFIG_CONTEXT")
		wcName := os.Getenv("E2E_WC_NAME")
		wcNamespace := os.Getenv("E2E_WC_NAMESPACE")
		appVersion := os.Getenv("E2E_APP_VERSION")

		// Ensure all require env vars are set
		Expect(mcKubeconfig).ToNot(BeEmpty(), "`E2E_KUBECONFIG` must be set to the kubeconfig of the test MC")
		Expect(mcContext).ToNot(BeEmpty(), "`E2E_KUBECONFIG_CONTEXT` must be set to the context to use in the kubeconfig")
		Expect(wcName).ToNot(BeEmpty(), "`E2E_WC_NAME` must be set to the name of the test WC")
		Expect(wcNamespace).ToNot(BeEmpty(), "`E2E_WC_NAMESPACE` must be set to namespace the test WC is in")
		Expect(appVersion).ToNot(BeEmpty(), "`E2E_APP_VERSION` must be set to version of the app to test against")

		state.SetContext(context.Background())

		// Setup client for conntecting to MC
		framework, err := clustertest.New(mcContext)
		Expect(err).NotTo(HaveOccurred())
		state.SetFramework(framework)

		// Setup client for connecting to WC
		cluster, err := framework.LoadCluster()
		Expect(err).NotTo(HaveOccurred())
		Expect(cluster).NotTo(BeNil())
		state.SetCluster(cluster)

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
	})

	Describe("", func() {
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

		if s.beforeInstall != nil {
			Describe("Before install", s.beforeInstall)
		}

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
