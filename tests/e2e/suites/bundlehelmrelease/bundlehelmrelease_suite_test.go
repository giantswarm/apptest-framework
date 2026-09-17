package bundlehelmrelease

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

// TestBundleHelmRelease covers `InAppBundle` combined with `WithHelmRelease`: the framework
// installs the parent bundle chart as a HelmRelease the management cluster owns, and the bundle
// installs the app under test as one of its children.
//
// gateway-api-bundle renders its children as App CRs, so this also covers the framework
// asserting on a child whose kind it did not choose.
func TestBundleHelmRelease(t *testing.T) {
	installNamespace := "envoy-gateway-system"

	suite.New().
		WithInstallNamespace(installNamespace).
		WithIsUpgrade(isUpgrade).
		WithBundleValuesFile("./bundle_values.yaml").
		WithHelmRelease(true).
		InAppBundle("gateway-api-bundle").
		AfterClusterReady(func() {

			It("should connect to the management cluster", func() {
				err := state.GetFramework().MC().CheckConnection()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should connect to the workload cluster", func() {
				Eventually(func() error {
					wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
					if err != nil {
						return err
					}
					return wcClient.CheckConnection()
				}).
					WithPolling(10 * time.Second).
					WithTimeout(5 * time.Minute).
					ShouldNot(HaveOccurred())
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
					err := wcClient.Get(state.GetContext(), types.NamespacedName{Namespace: installNamespace, Name: "envoy-gateway"}, &dp)
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
		Run(t, "Bundle HelmRelease Test")
}
