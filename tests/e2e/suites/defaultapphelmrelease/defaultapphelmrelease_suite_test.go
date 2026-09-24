package defaultapphelmrelease

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"
	"github.com/giantswarm/apptest-framework/v5/pkg/suite"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	applicationv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/clustertest/v5/pkg/logger"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	isUpgrade = false
)

// TestDefaultAppHelmRelease covers a default app suite that also sets `WithHelmRelease(true)`.
//
// The setting has to be ignored: the cluster chart owns the app's HelmRelease and drives its
// version through the Release, so a framework install would overwrite a resource somebody else
// reconciles. What the suite must still do is assert that the version under test is the one
// that actually got deployed.
func TestDefaultAppHelmRelease(t *testing.T) {
	installNamespace := "kube-system"

	suite.New().
		WithInstallNamespace(installNamespace).
		WithIsUpgrade(isUpgrade).
		WithValuesFile("./values.yaml").
		WithHelmRelease(true).
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
		Tests(func() {

			It("has the app running in the cluster", func() {
				wcClient, err := state.GetFramework().WC(state.GetCluster().Name)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() error {
					logger.Log("Checking if deployment exists in the workload cluster")
					var dp appsv1.Deployment
					err := wcClient.Get(state.GetContext(), types.NamespacedName{Namespace: installNamespace, Name: state.GetApplication().AppName}, &dp)
					if err != nil {
						logger.Log("Failed to get deployment: %v", err)
					}
					return err
				}).
					WithPolling(5 * time.Second).
					WithTimeout(5 * time.Minute).
					ShouldNot(HaveOccurred())
			})

			It("left the resource the cluster chart owns alone", func() {
				cluster := state.GetCluster()
				name := types.NamespacedName{
					Name:      cluster.Name + "-" + state.GetApplication().AppName,
					Namespace: cluster.GetNamespace(),
				}

				// Which kind the app is rendered as belongs to the cluster chart: current
				// charts install their default apps as HelmReleases, Releases still pinned to
				// an older one as App CRs. The framework follows whichever it finds, so the
				// assertion has to as well.
				var labels map[string]string

				hr := &helmv2.HelmRelease{}
				err := state.GetFramework().MC().Get(state.GetContext(), name, hr)
				switch {
				case err == nil:
					logger.Log("The cluster chart rendered '%s' as a HelmRelease", name.Name)
					labels = hr.Labels
				case apierrors.IsNotFound(err):
					appCR := &applicationv1alpha1.App{}
					err := state.GetFramework().MC().Get(state.GetContext(), name, appCR)
					Expect(err).NotTo(HaveOccurred(), "the cluster chart rendered neither a HelmRelease nor an App CR named '"+name.Name+"'")

					logger.Log("The cluster chart rendered '%s' as an App CR", name.Name)
					labels = appCR.Labels
				default:
					Expect(err).NotTo(HaveOccurred())
				}

				// The cluster chart labels everything it renders. The framework creates its
				// resources with no labels of its own, and installs over an existing one with
				// `CreateOrUpdate`, so a hijacked resource loses these.
				Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "Helm"), "the resource the cluster chart owns was overwritten by the framework")
				Expect(labels).To(HaveKeyWithValue("giantswarm.io/cluster", cluster.Name), "the resource the cluster chart owns was overwritten by the framework")
			})

		}).
		AfterSuite(func() {

			logger.Log("Cleaning up after tests have completed")

		}).
		Run(t, "Default App HelmRelease Test")
}
