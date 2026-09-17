package client

import (
	"context"
	"fmt"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/giantswarm/clustertest/v5/pkg/helmrelease"
	"github.com/giantswarm/clustertest/v5/pkg/logger"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cr "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck
	. "github.com/onsi/gomega"    //nolint:staticcheck
)

// SourceKind represents the kind of source reference used by a HelmRelease.
type SourceKind = helmrelease.SourceKind

const (
	// SourceKindHelmRepository uses spec.chart with a HelmRepository sourceRef.
	SourceKindHelmRepository = helmrelease.SourceKindHelmRepository
	// SourceKindOCIRepository uses spec.chartRef with an OCIRepository reference.
	SourceKindOCIRepository = helmrelease.SourceKindOCIRepository

	// DefaultGiantSwarmHelmRepositoryURL is the default OCI registry for Giant Swarm Helm charts.
	DefaultGiantSwarmHelmRepositoryURL = helmrelease.DefaultRegistryURL
)

// HelmReleaseConfig holds the configuration needed to create a HelmRelease CR.
type HelmReleaseConfig struct {
	// Name is the name of the HelmRelease resource.
	Name string
	// Namespace is the namespace where the HelmRelease CR will be created.
	Namespace string
	// TargetNamespace is the namespace where the Helm chart will be installed.
	TargetNamespace string
	// StorageNamespace is the namespace used for Helm storage.
	// If empty, defaults to the HelmRelease namespace.
	StorageNamespace string
	// ReleaseName is the Helm release name. If empty, defaults to the HelmRelease name.
	ReleaseName string
	// ChartName is the name of the chart in the source.
	ChartName string
	// ChartVersion is the version of the chart to install.
	ChartVersion string
	// SourceKind specifies the kind of source reference.
	// Defaults to SourceKindOCIRepository if not set.
	SourceKind SourceKind
	// SourceName is the name of the source reference (HelmRepository or OCIRepository).
	SourceName string
	// SourceNamespace is the namespace of the source reference.
	// If empty, defaults to the HelmRelease namespace.
	SourceNamespace string
	// SourceURL is the URL of the source to create.
	// For SourceKindHelmRepository: an OCI URL ("oci://registry/path") or HTTPS URL.
	// For SourceKindOCIRepository: an OCI URL ("oci://registry/path/chart").
	// If empty, the Giant Swarm registry is used. The framework creates the source CR
	// before installing the HelmRelease either way.
	SourceURL string
	// Values is the raw values YAML to pass to the chart.
	Values string
	// Interval is the reconciliation interval. Defaults to 5m.
	Interval time.Duration
	// Timeout is the time to wait for Helm operations. Defaults to 5m.
	Timeout time.Duration
	// Retries is the number of retries for install/upgrade remediation. Defaults to 10.
	Retries *int
	// ServiceAccountName is the Kubernetes service account to impersonate when reconciling.
	// Required by clusters with the flux-multi-tenancy Kyverno policy.
	ServiceAccountName string
	// KubeConfigSecretName is the name of the secret containing kubeconfig for remote cluster access.
	// Required when deploying to a workload cluster from the management cluster.
	KubeConfigSecretName string
	// InCluster installs the chart into the cluster the HelmRelease itself lives in, under the
	// impersonated ServiceAccountName and with no kubeConfig. The target namespace is not
	// created by Helm in this case: an impersonated service account is scoped to a namespace
	// that already exists, and Helm's --create-namespace only tolerates AlreadyExists, not the
	// Forbidden it would get back.
	InCluster bool
	// InlineValues writes Values to spec.values instead of to a referenced Secret.
	// helm-controller does not watch valuesFrom sources, so only an inline change reconciles
	// immediately. Use it whenever the values themselves are what a test step changes.
	InlineValues bool
}

// source describes the Flux source CR the HelmRelease pulls its chart from. The kind,
// name and namespace are defaulted here so call-sites can read them back; everything
// else is left for clustertest to default.
func (cfg HelmReleaseConfig) source() helmrelease.Source {
	kind := cfg.SourceKind
	if kind == "" {
		kind = SourceKindOCIRepository
	}

	name := cfg.SourceName
	if name == "" {
		name = cfg.ChartName
	}

	namespace := cfg.SourceNamespace
	if namespace == "" {
		namespace = cfg.Namespace
	}

	return helmrelease.Source{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
		ChartName: cfg.ChartName,
		URL:       cfg.SourceURL,
		Tag:       cfg.ChartVersion,
	}
}

// InstallHelmRelease creates a HelmRelease CR and waits for it to become ready.
// It ensures the HelmRelease namespace exists on the MC. Target and storage namespaces
// are created by Flux via spec.install.createNamespace.
// Timeout can be controlled via the provided context.
func InstallHelmRelease(ctx context.Context, cfg HelmReleaseConfig) {
	GinkgoHelper()

	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}

	// Ensure the HelmRelease namespace exists on the MC.
	// Target and storage namespaces are created by Flux via spec.install.createNamespace.
	ensureNamespace(ctx, cfg.Namespace)

	// Ensure the source CR the HelmRelease pulls the chart from exists
	source := cfg.source()
	err := helmrelease.EnsureSource(ctx, state.GetFramework().MC(), source)
	Expect(err).NotTo(HaveOccurred())

	// Ensure the service account exists. An in-cluster install impersonates a service account
	// that must already hold the permissions to install the chart, so it is required rather
	// than created.
	if cfg.ServiceAccountName != "" {
		if cfg.InCluster {
			requireServiceAccount(ctx, cfg.ServiceAccountName, cfg.Namespace)
		} else {
			ensureServiceAccount(ctx, cfg.ServiceAccountName, cfg.Namespace)
		}
	}

	if cfg.Values != "" && !cfg.InlineValues {
		createValuesSecret(ctx, cfg.Name, cfg.Namespace, cfg.Values)
	}

	hr := buildHelmRelease(cfg)
	logger.Log("Installing HelmRelease %s/%s (chart: %s, version: %s, source: %s/%s)",
		hr.Namespace, hr.Name, cfg.ChartName, cfg.ChartVersion, source.Kind, source.Name)

	err = state.GetFramework().MC().CreateOrUpdate(state.GetContext(), hr)
	Expect(err).NotTo(HaveOccurred())

	state.SetHelmRelease(hr)

	Eventually(func() (bool, error) {
		return IsHelmReleaseReady(state.GetContext(), hr.Name, hr.Namespace)
	}).
		WithContext(ctx).
		WithPolling(5 * time.Second).
		Should(BeTrue())
}

// IsHelmReleaseReady checks if a HelmRelease has the Ready condition set to True.
// The current status is logged on each call, mirroring the App CR wait conditions.
// A HelmRelease that doesn't exist yet is not an error, it is simply not ready.
func IsHelmReleaseReady(ctx context.Context, name, namespace string) (bool, error) {
	return helmrelease.IsHelmReleaseReady(ctx, state.GetFramework().MC(), name, namespace)()
}

// IsAllHelmReleasesReady returns a check function for use with Gomega's Eventually
// that polls the given list of HelmReleases and returns true once all of them
// have a Ready=True condition. Its signature mirrors wait.IsAllAppDeployed so
// call-sites can use either one interchangeably.
// Every HelmRelease is checked and logged on each poll, so a stuck release is visible.
func IsAllHelmReleasesReady(ctx context.Context, c cr.Client, helmReleases []types.NamespacedName) func() (bool, error) {
	areAllReady := helmrelease.AreAllReady(ctx, c, helmReleases)
	return func() (bool, error) {
		return areAllReady() == nil, nil
	}
}

// IsHelmReleaseVersion checks whether the chart version a HelmRelease has actually
// deployed matches the expected one. A HelmRelease that doesn't exist yet is not an
// error, it is simply not at the expected version.
func IsHelmReleaseVersion(ctx context.Context, name, namespace, version string) (bool, error) {
	return helmrelease.IsHelmReleaseVersion(ctx, state.GetFramework().MC(), name, namespace, version)()
}

// DeleteHelmRelease deletes a HelmRelease CR and its associated values Secret if present.
func DeleteHelmRelease(ctx context.Context, name, namespace string) error {
	hr := &helmv2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := state.GetFramework().MC().Delete(ctx, hr)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting HelmRelease %s/%s: %w", namespace, name, err)
	}

	// Clean up the values secret if it was created
	secretName := fmt.Sprintf("%s-values", name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}
	err = state.GetFramework().MC().Delete(ctx, secret)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting values Secret %s/%s: %w", namespace, secretName, err)
	}

	return nil
}

// DeleteHelmSource deletes the source CR (HelmRepository or OCIRepository) backing the
// HelmRelease. InstallHelmRelease always ensures the source, also when SourceURL is empty
// and the default registry URL is used, so it is always cleaned up here. Leaving it behind
// pins the next run to this run's chart version, as an existing source is never updated.
func DeleteHelmSource(ctx context.Context, cfg HelmReleaseConfig) error {
	return helmrelease.DeleteSource(ctx, state.GetFramework().MC(), cfg.source())
}

func buildHelmRelease(cfg HelmReleaseConfig) *helmv2.HelmRelease {
	source := cfg.source()

	retries := 10
	if cfg.Retries != nil {
		retries = *cfg.Retries
	}

	hr := &helmv2.HelmRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "helm.toolkit.fluxcd.io/v2",
			Kind:       "HelmRelease",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
		},
		Spec: helmv2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: cfg.Interval},
			Install: &helmv2.Install{
				Remediation: &helmv2.InstallRemediation{
					Retries: retries,
				},
				CreateNamespace: !cfg.InCluster,
			},
			Upgrade: &helmv2.Upgrade{
				Remediation: &helmv2.UpgradeRemediation{
					Retries:              retries,
					Strategy:             &[]helmv2.RemediationStrategy{helmv2.RollbackRemediationStrategy}[0],
					RemediateLastFailure: boolPtr(true),
				},
			},
		},
	}

	if cfg.Timeout > 0 {
		hr.Spec.Timeout = &metav1.Duration{Duration: cfg.Timeout}
	}

	switch source.Kind {
	case SourceKindOCIRepository:
		hr.Spec.ChartRef = &helmv2.CrossNamespaceSourceReference{
			Kind:      string(source.Kind),
			Name:      source.Name,
			Namespace: source.Namespace,
		}
	case SourceKindHelmRepository:
		hr.Spec.Chart = &helmv2.HelmChartTemplate{
			Spec: helmv2.HelmChartTemplateSpec{
				Chart:   cfg.ChartName,
				Version: cfg.ChartVersion,
				SourceRef: helmv2.CrossNamespaceObjectReference{
					Kind:      string(source.Kind),
					Name:      source.Name,
					Namespace: source.Namespace,
				},
			},
		}
	}

	if cfg.TargetNamespace != "" {
		hr.Spec.TargetNamespace = cfg.TargetNamespace
	}

	if cfg.StorageNamespace != "" {
		hr.Spec.StorageNamespace = cfg.StorageNamespace
	}

	if cfg.ReleaseName != "" {
		hr.Spec.ReleaseName = cfg.ReleaseName
	}

	if cfg.ServiceAccountName != "" {
		hr.Spec.ServiceAccountName = cfg.ServiceAccountName
	}

	if cfg.Values != "" {
		if cfg.InlineValues {
			raw, err := yaml.YAMLToJSON([]byte(cfg.Values))
			Expect(err).NotTo(HaveOccurred())
			hr.Spec.Values = &apiextensionsv1.JSON{Raw: raw}
		} else {
			hr.Spec.ValuesFrom = append(hr.Spec.ValuesFrom, helmv2.ValuesReference{
				Kind: "Secret",
				Name: fmt.Sprintf("%s-values", cfg.Name),
			})
		}
	}

	if cfg.KubeConfigSecretName != "" {
		hr.Spec.KubeConfig = &meta.KubeConfigReference{
			SecretRef: &meta.SecretKeyReference{
				Name: cfg.KubeConfigSecretName,
			},
		}
	}

	return hr
}

// createValuesSecret creates a Secret containing chart values for a HelmRelease.
// It ensures the target namespace exists before creating the Secret.
func createValuesSecret(ctx context.Context, name, namespace, values string) {
	GinkgoHelper()

	ensureNamespace(ctx, namespace)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-values", name),
			Namespace: namespace,
		},
		StringData: map[string]string{
			"values.yaml": values,
		},
	}

	err := state.GetFramework().MC().CreateOrUpdate(ctx, secret)
	Expect(err).NotTo(HaveOccurred())
}

// UpdateHelmReleaseVersion updates the chart version for an existing HelmRelease.
// For HelmRepository sources, it updates spec.chart.spec.version on the HelmRelease.
// For OCIRepository sources, it updates spec.ref.tag on the OCIRepository (sourced from cfg).
func UpdateHelmReleaseVersion(ctx context.Context, cfg HelmReleaseConfig, version string) {
	GinkgoHelper()

	if source := cfg.source(); source.Kind == SourceKindOCIRepository {
		err := helmrelease.UpdateOCIRepositoryTag(ctx, state.GetFramework().MC(), source.Name, source.Namespace, version)
		Expect(err).NotTo(HaveOccurred())
		return
	}

	hr := &helmv2.HelmRelease{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: cfg.Name, Namespace: cfg.Namespace}, hr)
	Expect(err).NotTo(HaveOccurred())

	if hr.Spec.Chart != nil {
		hr.Spec.Chart.Spec.Version = version
		err = state.GetFramework().MC().Update(ctx, hr, &cr.UpdateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

// UpdateHelmReleaseValues rewrites spec.values on an existing HelmRelease.
//
// Writing the spec bumps metadata.generation, so helm-controller picks the change up straight
// away. A values Secret referenced through valuesFrom would not: helm-controller does not watch
// those, so the new values would only land on the next reconcile interval, if at all.
func UpdateHelmReleaseValues(ctx context.Context, name, namespace, values string) error {
	raw, err := yaml.YAMLToJSON([]byte(values))
	if err != nil {
		return fmt.Errorf("converting values for HelmRelease %s/%s: %w", namespace, name, err)
	}

	hr := &helmv2.HelmRelease{}
	if err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hr); err != nil {
		return fmt.Errorf("getting HelmRelease %s/%s: %w", namespace, name, err)
	}

	hr.Spec.Values = &apiextensionsv1.JSON{Raw: raw}
	if err := state.GetFramework().MC().Update(ctx, hr, &cr.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating values of HelmRelease %s/%s: %w", namespace, name, err)
	}

	state.SetHelmRelease(hr)

	return nil
}

// WaitForHelmReleaseDeleted blocks until the named HelmRelease is gone from the API, or the
// context expires.
//
// Deleting a HelmRelease only starts the uninstall: helm-controller holds a finalizer until it
// has run, and anything the release created is still in place until then.
func WaitForHelmReleaseDeleted(ctx context.Context, name, namespace string) error {
	for {
		hr := &helmv2.HelmRelease{}
		err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hr)
		if errors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("getting HelmRelease %s/%s: %w", namespace, name, err)
		}

		logger.Log("Waiting for HelmRelease %s/%s to finish uninstalling", namespace, name)

		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for HelmRelease %s/%s to be deleted: %w", namespace, name, ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
}

// requireServiceAccount fails the suite when the service account to impersonate does not exist.
//
// Creating it, the way ensureServiceAccount does, produces a service account with no
// permissions at all, which turns a missing prerequisite into an opaque RBAC failure minutes
// later and leaves the service account behind.
func requireServiceAccount(ctx context.Context, name, namespace string) {
	GinkgoHelper()

	sa := &corev1.ServiceAccount{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sa)
	Expect(err).NotTo(HaveOccurred(), "ServiceAccount '%s/%s' must already exist and hold the permissions to install the chart", namespace, name)
}

// ensureServiceAccount creates a service account if it doesn't already exist.
func ensureServiceAccount(ctx context.Context, name, namespace string) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sa)
	if errors.IsNotFound(err) {
		logger.Log("Creating ServiceAccount %s/%s", namespace, name)
		sa.TypeMeta = metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		}
		err = state.GetFramework().MC().Create(ctx, sa)
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

// ensureNamespace creates a namespace if it doesn't already exist.
func ensureNamespace(ctx context.Context, name string) {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name}, ns)
	if errors.IsNotFound(err) {
		logger.Log("Creating namespace %s", name)
		err = state.GetFramework().MC().Create(ctx, ns)
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func boolPtr(b bool) *bool {
	return &b
}
