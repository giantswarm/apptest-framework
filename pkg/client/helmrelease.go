package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/giantswarm/clustertest/v4/pkg/logger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	cr "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/apptest-framework/v4/pkg/state"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck
	. "github.com/onsi/gomega"    //nolint:staticcheck
)

// SourceKind represents the kind of source reference used by a HelmRelease.
type SourceKind string

const (
	// SourceKindHelmRepository uses spec.chart with a HelmRepository sourceRef.
	SourceKindHelmRepository SourceKind = "HelmRepository"
	// SourceKindOCIRepository uses spec.chartRef with an OCIRepository reference.
	SourceKindOCIRepository SourceKind = "OCIRepository"

	// DefaultGiantSwarmHelmRepositoryURL is the default OCI registry for Giant Swarm Helm charts.
	DefaultGiantSwarmHelmRepositoryURL = "oci://gsoci.azurecr.io/charts/giantswarm"
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
	// SourceURL is the URL of the source to create automatically.
	// For SourceKindHelmRepository: an OCI URL ("oci://registry/path") or HTTPS URL.
	// For SourceKindOCIRepository: an OCI URL ("oci://registry/path/chart").
	// When set, the framework creates the source CR before installing the HelmRelease.
	// If empty, the source CR must already exist in the cluster.
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
}

// InstallHelmRelease creates a HelmRelease CR and waits for it to become ready.
// It also ensures that the target and storage namespaces exist before creating the HelmRelease.
// Timeout can be controlled via the provided context.
func InstallHelmRelease(ctx context.Context, cfg HelmReleaseConfig) {
	GinkgoHelper()

	if cfg.Interval == 0 {
		cfg.Interval = 5 * time.Minute
	}

	// Default SourceName to ChartName if not set
	if cfg.SourceName == "" {
		cfg.SourceName = cfg.ChartName
	}

	// Ensure the source CR exists if a URL was provided
	ensureHelmSource(ctx, cfg)

	// Ensure required namespaces exist
	ensureNamespace(ctx, cfg.Namespace)
	if cfg.TargetNamespace != "" {
		ensureNamespace(ctx, cfg.TargetNamespace)
	}
	if cfg.StorageNamespace != "" {
		ensureNamespace(ctx, cfg.StorageNamespace)
	}

	// Ensure the service account exists
	if cfg.ServiceAccountName != "" {
		ensureServiceAccount(ctx, cfg.ServiceAccountName, cfg.Namespace)
	}

	if cfg.Values != "" {
		createValuesSecret(ctx, cfg.Name, cfg.Namespace, cfg.Values)
	}

	hr := buildHelmRelease(cfg)
	logger.Log("Installing HelmRelease %s/%s (chart: %s, version: %s, source: %s/%s)",
		hr.Namespace, hr.Name, cfg.ChartName, cfg.ChartVersion, cfg.SourceKind, cfg.SourceName)

	err := state.GetFramework().MC().CreateOrUpdate(state.GetContext(), hr)
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
func IsHelmReleaseReady(ctx context.Context, name, namespace string) (bool, error) {
	hr := &helmv2.HelmRelease{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hr)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	for _, condition := range hr.Status.Conditions {
		if condition.Type == "Ready" {
			return condition.Status == metav1.ConditionTrue, nil
		}
	}

	return false, nil
}

// IsHelmReleaseVersion checks if a HelmRelease has the expected chart version in its status history.
func IsHelmReleaseVersion(ctx context.Context, name, namespace, version string) (bool, error) {
	hr := &helmv2.HelmRelease{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hr)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check spec.chart if using HelmRepository source
	if hr.Spec.Chart != nil {
		return hr.Spec.Chart.Spec.Version == version, nil
	}

	// For OCIRepository sources, check the last attempted revision in status
	if hr.Status.LastAttemptedRevision != "" {
		return hr.Status.LastAttemptedRevision == version, nil
	}

	return false, nil
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

// DeleteHelmSource deletes the source CR (HelmRepository or OCIRepository) created by ensureHelmSource.
// It is a no-op if SourceURL is empty (i.e. the source was pre-existing and not created by the framework).
func DeleteHelmSource(ctx context.Context, cfg HelmReleaseConfig) error {
	if cfg.SourceURL == "" {
		return nil
	}

	sourceName := cfg.SourceName
	if sourceName == "" {
		sourceName = cfg.ChartName
	}
	sourceNamespace := cfg.SourceNamespace
	if sourceNamespace == "" {
		sourceNamespace = cfg.Namespace
	}

	sourceKind := cfg.SourceKind
	if sourceKind == "" {
		sourceKind = SourceKindOCIRepository
	}

	var obj *unstructured.Unstructured
	switch sourceKind {
	case SourceKindHelmRepository:
		obj = &unstructured.Unstructured{}
		obj.SetAPIVersion("source.toolkit.fluxcd.io/v1")
		obj.SetKind("HelmRepository")
	case SourceKindOCIRepository:
		obj = &unstructured.Unstructured{}
		obj.SetAPIVersion("source.toolkit.fluxcd.io/v1beta2")
		obj.SetKind("OCIRepository")
	default:
		return nil
	}

	obj.SetName(sourceName)
	obj.SetNamespace(sourceNamespace)

	logger.Log("Deleting %s %s/%s", sourceKind, sourceNamespace, sourceName)
	err := state.GetFramework().MC().Delete(ctx, obj)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting %s %s/%s: %w", sourceKind, sourceNamespace, sourceName, err)
	}
	return nil
}

// ensureHelmSource creates the source CR (HelmRepository or OCIRepository) if SourceURL is set.
// For SourceKindHelmRepository with no SourceURL, defaults to DefaultGiantSwarmHelmRepositoryURL.
// If the source already exists it is left unchanged.
func ensureHelmSource(ctx context.Context, cfg HelmReleaseConfig) {
	GinkgoHelper()

	sourceKind := cfg.SourceKind
	if sourceKind == "" {
		sourceKind = SourceKindOCIRepository
	}

	sourceURL := cfg.SourceURL
	if sourceURL == "" {
		switch sourceKind {
		case SourceKindHelmRepository:
			sourceURL = DefaultGiantSwarmHelmRepositoryURL
		case SourceKindOCIRepository:
			chartName := cfg.ChartName
			if chartName != "" {
				sourceURL = DefaultGiantSwarmHelmRepositoryURL + "/" + chartName
			}
		}
	}

	if sourceURL == "" {
		return
	}

	sourceName := cfg.SourceName
	if sourceName == "" {
		sourceName = cfg.ChartName
	}
	sourceNamespace := cfg.SourceNamespace
	if sourceNamespace == "" {
		sourceNamespace = cfg.Namespace
	}

	switch sourceKind {
	case SourceKindHelmRepository:
		ensureHelmRepository(ctx, sourceName, sourceNamespace, sourceURL)
	case SourceKindOCIRepository:
		ensureOCIRepository(ctx, sourceName, sourceNamespace, sourceURL, cfg.ChartVersion)
	}
}

// ensureHelmRepository creates a HelmRepository if it doesn't already exist.
// For OCI-hosted Helm charts, pass an "oci://" URL; for HTTP/HTTPS catalogs pass an https URL.
func ensureHelmRepository(ctx context.Context, name, namespace, url string) {
	GinkgoHelper()

	repoType := "default"
	if strings.HasPrefix(url, "oci://") {
		repoType = "oci"
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "source.toolkit.fluxcd.io/v1",
			"kind":       "HelmRepository",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"type":     repoType,
				"url":      url,
				"interval": "5m",
			},
		},
	}

	logger.Log("Ensuring HelmRepository %s/%s (url: %s)", namespace, name, url)
	err := state.GetFramework().MC().Create(ctx, obj)
	if err != nil && !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

// ensureOCIRepository creates an OCIRepository if it doesn't already exist.
// The tag is set to chartVersion; pass an empty chartVersion to use "latest".
func ensureOCIRepository(ctx context.Context, name, namespace, url, tag string) {
	GinkgoHelper()

	if tag == "" {
		tag = "latest"
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "source.toolkit.fluxcd.io/v1beta2",
			"kind":       "OCIRepository",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"url":      url,
				"interval": "5m",
				"ref": map[string]any{
					"tag": tag,
				},
			},
		},
	}

	logger.Log("Ensuring OCIRepository %s/%s (url: %s, tag: %s)", namespace, name, url, tag)
	err := state.GetFramework().MC().Create(ctx, obj)
	if err != nil && !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

// updateOCIRepositoryTag patches the spec.ref.tag of an existing OCIRepository.
func updateOCIRepositoryTag(ctx context.Context, name, namespace, tag string) {
	GinkgoHelper()

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("source.toolkit.fluxcd.io/v1beta2")
	obj.SetKind("OCIRepository")

	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
	Expect(err).NotTo(HaveOccurred())

	if err := unstructured.SetNestedField(obj.Object, tag, "spec", "ref", "tag"); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	logger.Log("Updating OCIRepository %s/%s tag to %s", namespace, name, tag)
	err = state.GetFramework().MC().Update(ctx, obj, &cr.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func buildHelmRelease(cfg HelmReleaseConfig) *helmv2.HelmRelease {
	sourceName := cfg.SourceName
	if sourceName == "" {
		sourceName = cfg.ChartName
	}

	sourceNamespace := cfg.SourceNamespace
	if sourceNamespace == "" {
		sourceNamespace = cfg.Namespace
	}

	sourceKind := cfg.SourceKind
	if sourceKind == "" {
		sourceKind = SourceKindOCIRepository
	}

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

	switch sourceKind {
	case SourceKindOCIRepository:
		hr.Spec.ChartRef = &helmv2.CrossNamespaceSourceReference{
			Kind:      string(SourceKindOCIRepository),
			Name:      sourceName,
			Namespace: sourceNamespace,
		}
	case SourceKindHelmRepository:
		hr.Spec.Chart = &helmv2.HelmChartTemplate{
			Spec: helmv2.HelmChartTemplateSpec{
				Chart:   cfg.ChartName,
				Version: cfg.ChartVersion,
				SourceRef: helmv2.CrossNamespaceObjectReference{
					Kind:      string(SourceKindHelmRepository),
					Name:      sourceName,
					Namespace: sourceNamespace,
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
		hr.Spec.ValuesFrom = append(hr.Spec.ValuesFrom, helmv2.ValuesReference{
			Kind: "Secret",
			Name: fmt.Sprintf("%s-values", cfg.Name),
		})
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

	sourceKind := cfg.SourceKind
	if sourceKind == "" {
		sourceKind = SourceKindOCIRepository
	}

	if sourceKind == SourceKindOCIRepository {
		sourceName := cfg.SourceName
		if sourceName == "" {
			sourceName = cfg.ChartName
		}
		sourceNamespace := cfg.SourceNamespace
		if sourceNamespace == "" {
			sourceNamespace = cfg.Namespace
		}
		updateOCIRepositoryTag(ctx, sourceName, sourceNamespace, version)
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
