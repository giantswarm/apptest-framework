package basic

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/apptest-framework/v5/pkg/suite"

	"github.com/giantswarm/clustertest/v5/pkg/logger"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	isUpgrade = false
)

func TestMCApp(t *testing.T) {
	installNamespace := "default"
	// hello-world is installed via an OCIRepository + Flux HelmRelease (not an App CR)
	// so that the platform's cluster-values aren't injected into the chart. hello-world
	// v3.x sets `additionalProperties: false` at the schema root and would otherwise fail
	// to install with a values-schema-violation. An explicit release name keeps the
	// rendered Deployment name deterministic for the lookup. The framework installs it
	// the way any app of a cluster is installed: a HelmRelease in the cluster's org
	// namespace that reaches the cluster, here the MC itself, through its kubeconfig
	// secret. values.yaml turns the chart's Ingress off because it defaults to the
	// `nginx` class, which an MC has no IngressClass for.
	releaseName := "hello-world"

	suite.New().
		WithInstallNamespace(installNamespace).
		WithIsUpgrade(isUpgrade).
		WithValuesFile("./values.yaml").
		WithHelmRelease(true).
		WithHelmReleaseName(releaseName).
		WithHelmTargetNamespace(installNamespace).
		AfterClusterReady(func() {

			It("should connect to the management cluster", func() {
				err := state.GetFramework().MC().CheckConnection()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should connect to the workload cluster", func() {
				wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
				Expect(err).NotTo(HaveOccurred())

				err = wcClient.CheckConnection()
				Expect(err).NotTo(HaveOccurred())
			})

		}).
		BeforeUpgrade(func() {

			It("should not have run the before upgrade", func() {
				logger.Log("This isn't an upgrade test so this test case shouldn't have happened")
				Fail("Shouldn't perform pre-upgrade tests if not an upgrade test suite")
			})

		}).
		Tests(func() {

			It("has the app running in the cluster", func() {
				wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() error {
					logger.Log("Checking if deployment exists in the workload cluster")
					var dp appsv1.Deployment
					err := wcClient.Get(state.GetContext(), types.NamespacedName{Namespace: installNamespace, Name: releaseName}, &dp)
					if err != nil {
						logger.Log("Failed to get deployment: %v", err)
					}
					return err
				}).
					WithPolling(5 * time.Second).
					WithTimeout(5 * time.Minute).
					ShouldNot(HaveOccurred())
			})

		}).
		AfterSuite(func() {

			logger.Log("Cleaning up after tests have completed")

		}).
		Run(t, "MC App Test")
}
