package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/giantswarm/clustertest/v5/pkg/helmrelease"
	"github.com/giantswarm/clustertest/v5/pkg/logger"
	"github.com/giantswarm/clustertest/v5/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cr "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/apptest-framework/v5/pkg/state"

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
	// If empty, defaults to the HelmRelease name, so that every suite owns its own source
	// CR and never shares one with the cluster or another tenant. Set this only to point at
	// a source CR that already exists in the cluster; note that a pre-existing OCIRepository
	// cannot be used, because for that kind the chart version lives in the source's
	// spec.ref and the framework must be able to set it.
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

// resolvedSource is the fully defaulted identity of the Flux source CR backing a
// HelmRelease. It is the single source of truth for the source kind, name, namespace and
// URL, so that installing, updating and deleting a source can never disagree about which
// object they are talking about.
//
// An empty URL means no source CR is to be created, and one is expected to already exist
// in the cluster.
type resolvedSource struct {
	Kind      SourceKind
	Name      string
	Namespace string
	URL       string
}

// resolveSource applies the defaulting rules for the source CR backing the given
// HelmRelease config. It is deliberately free of assertions so it stays a pure function;
// callers assert on the result.
func resolveSource(cfg HelmReleaseConfig) resolvedSource {
	src := resolvedSource{
		Kind:      cfg.SourceKind,
		Name:      cfg.SourceName,
		Namespace: cfg.SourceNamespace,
		URL:       cfg.SourceURL,
	}

	if src.Kind == "" {
		src.Kind = SourceKindOCIRepository
	}
	// Default to the HelmRelease name rather than the chart name: the HelmRelease name is
	// already unique to this test run, so the framework owns the source it creates instead
	// of colliding with a shared one under the chart's name.
	if src.Name == "" {
		src.Name = cfg.Name
	}
	if src.Namespace == "" {
		src.Namespace = cfg.Namespace
	}

	if src.URL == "" {
		switch src.Kind {
		case SourceKindHelmRepository:
			src.URL = DefaultGiantSwarmHelmRepositoryURL
		case SourceKindOCIRepository:
			if cfg.ChartName != "" {
				src.URL = DefaultGiantSwarmHelmRepositoryURL + "/" + cfg.ChartName
			}
		}
	}

	return src
}

// frameworkOwnsSource reports whether a source CR may be modified or deleted by this
// framework. Sources the framework creates carry utils.DeleteAnnotation; anything without
// it belongs to the cluster or another tenant and must be left alone.
func frameworkOwnsSource(obj cr.Object) bool {
	return utils.SafeToDelete(obj.GetAnnotations())
}

// registerSourceScheme registers the source-controller v1 types on the MC client scheme.
//
// clustertest registers only source.toolkit.fluxcd.io/v1beta2, so without this the first
// Get or Create of a v1 source fails with "no kind is registered for the type".
//
// TODO: remove once a clustertest release including
// https://github.com/giantswarm/clustertest/pull/804 is picked up.
func registerSourceScheme() {
	_ = sourcev1.AddToScheme(state.GetFramework().MC().Scheme())
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

	// Ensure the source CR exists
	ensureHelmSource(ctx, cfg)

	// Ensure the HelmRelease namespace exists on the MC.
	// Target and storage namespaces are created by Flux via spec.install.createNamespace.
	ensureNamespace(ctx, cfg.Namespace)

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
// The current status is logged on each call, mirroring the App CR wait conditions.
func IsHelmReleaseReady(ctx context.Context, name, namespace string) (bool, error) {
	ready, err := helmrelease.IsHelmReleaseReady(ctx, state.GetFramework().MC(), name, namespace)()
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Log("HelmRelease '%s/%s' not found yet", namespace, name)
			return false, nil
		}
		return false, err
	}
	return ready, nil
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
		return logHelmReleaseVersion(name, version, hr.Spec.Chart.Spec.Version), nil
	}

	// For OCIRepository sources, check the last attempted revision in status.
	// Flux appends a +<oci-digest> suffix (e.g. 0.0.1-abc123+4ef3415e2070) and
	// version may carry a v prefix, so normalise both sides before comparing.
	if hr.Status.LastAttemptedRevision != "" {
		rev := strings.SplitN(hr.Status.LastAttemptedRevision, "+", 2)[0]
		return logHelmReleaseVersion(name, strings.TrimPrefix(version, "v"), rev), nil
	}

	logger.Log("HelmRelease version for '%s' is not yet known: expectedVersion='%s'", name, version)
	return false, nil
}

// logHelmReleaseVersion logs the version comparison and reports whether it matches.
func logHelmReleaseVersion(name, expectedVersion, actualVersion string) bool {
	if expectedVersion == actualVersion {
		logger.Log("HelmRelease version for '%s' is as expected: expectedVersion='%s' actualVersion='%s'", name, expectedVersion, actualVersion)
		return true
	}
	logger.Log("HelmRelease version for '%s' is not yet as expected: expectedVersion='%s' actualVersion='%s'", name, expectedVersion, actualVersion)
	return false
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
// HelmRelease, but only the one the framework created.
//
// Ownership is decided by the annotation ensureHelmSource sets, not by the config: a source
// that already existed in the cluster is left in place, since it may be shared with the
// cluster itself or with other tenants.
func DeleteHelmSource(ctx context.Context, cfg HelmReleaseConfig) error {
	src := resolveSource(cfg)

	registerSourceScheme()

	var obj cr.Object
	switch src.Kind {
	case SourceKindHelmRepository:
		obj = &sourcev1.HelmRepository{}
	case SourceKindOCIRepository:
		obj = &sourcev1.OCIRepository{}
	default:
		return nil
	}

	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: src.Name, Namespace: src.Namespace}, obj)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting %s %s/%s: %w", src.Kind, src.Namespace, src.Name, err)
	}

	if !frameworkOwnsSource(obj) {
		logger.Log("Not deleting %s %s/%s: it was not created by this framework", src.Kind, src.Namespace, src.Name)
		return nil
	}

	logger.Log("Deleting %s %s/%s", src.Kind, src.Namespace, src.Name)
	if err := state.GetFramework().MC().Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting %s %s/%s: %w", src.Kind, src.Namespace, src.Name, err)
	}
	return nil
}

// ensureHelmSource makes sure the source CR (HelmRepository or OCIRepository) backing the
// HelmRelease exists and points at the chart under test.
//
// A source the framework creates is annotated so it can be recognised later: it is updated
// if a previous run left one behind, and deleted during cleanup. A source that already
// exists without that annotation belongs to somebody else and is never modified.
//
// It is a no-op when the resolved source has no URL, which means the source CR is expected
// to already exist in the cluster.
func ensureHelmSource(ctx context.Context, cfg HelmReleaseConfig) {
	GinkgoHelper()

	src := resolveSource(cfg)
	if src.URL == "" {
		return
	}

	registerSourceScheme()

	switch src.Kind {
	case SourceKindHelmRepository:
		ensureHelmRepository(ctx, src)
	case SourceKindOCIRepository:
		ensureOCIRepository(ctx, src, cfg.ChartVersion)
	}
}

// ensureHelmRepository creates the HelmRepository, or updates it if the framework owns it.
// For OCI-hosted Helm charts the URL is an "oci://" URL; for HTTP/HTTPS catalogs an https one.
//
// Reusing a HelmRepository the framework does not own is safe, because for this source kind
// the chart version is set on the HelmRelease itself rather than on the source.
func ensureHelmRepository(ctx context.Context, src resolvedSource) {
	GinkgoHelper()

	repoType := sourcev1.HelmRepositoryTypeDefault
	if strings.HasPrefix(src.URL, "oci://") {
		repoType = sourcev1.HelmRepositoryTypeOCI
	}

	desired := &sourcev1.HelmRepositorySpec{
		Type:     repoType,
		URL:      src.URL,
		Interval: metav1.Duration{Duration: 5 * time.Minute},
	}

	existing := &sourcev1.HelmRepository{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: src.Name, Namespace: src.Namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Log("Creating HelmRepository %s/%s (url: %s)", src.Namespace, src.Name, src.URL)
		obj := &sourcev1.HelmRepository{ObjectMeta: sourceObjectMeta(src), Spec: *desired}
		err = state.GetFramework().MC().Create(ctx, obj)
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		return
	}
	Expect(err).NotTo(HaveOccurred())

	if !frameworkOwnsSource(existing) {
		logger.Log("Reusing pre-existing HelmRepository %s/%s, which is not managed by this framework", src.Namespace, src.Name)
		return
	}

	logger.Log("Updating HelmRepository %s/%s left over from an earlier run (url: %s)", src.Namespace, src.Name, src.URL)
	existing.Spec = *desired
	Expect(state.GetFramework().MC().Update(ctx, existing)).To(Succeed())
}

// ensureOCIRepository creates the OCIRepository, or updates it if the framework owns it.
// The ref is pinned to chartVersion, falling back to a "*" semver range when no version is
// given. There is no "latest" tag in the Giant Swarm registry.
//
// Unlike a HelmRepository, an OCIRepository carries the chart version in its spec.ref, so
// one the framework does not own cannot be used: the version under test could not be
// pinned, and the suite would wait for a version that is never fetched.
func ensureOCIRepository(ctx context.Context, src resolvedSource, chartVersion string) {
	GinkgoHelper()

	desired := &sourcev1.OCIRepositorySpec{
		URL:       src.URL,
		Interval:  metav1.Duration{Duration: 5 * time.Minute},
		Reference: ociRepositoryRef(chartVersion),
	}

	existing := &sourcev1.OCIRepository{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: src.Name, Namespace: src.Namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Log("Creating OCIRepository %s/%s (url: %s, ref: %+v)", src.Namespace, src.Name, src.URL, *desired.Reference)
		obj := &sourcev1.OCIRepository{ObjectMeta: sourceObjectMeta(src), Spec: *desired}
		err = state.GetFramework().MC().Create(ctx, obj)
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		return
	}
	Expect(err).NotTo(HaveOccurred())

	Expect(frameworkOwnsSource(existing)).To(BeTrue(), fmt.Sprintf(
		"OCIRepository %s/%s already exists and is not managed by this framework, so the chart "+
			"version under test cannot be pinned on it. Remove WithHelmSourceName/WithHelmSourceNamespace "+
			"to let the framework create its own source, or point them at a name it can own.",
		src.Namespace, src.Name))

	logger.Log("Updating OCIRepository %s/%s left over from an earlier run (url: %s, ref: %+v)", src.Namespace, src.Name, src.URL, *desired.Reference)
	existing.Spec = *desired
	Expect(state.GetFramework().MC().Update(ctx, existing)).To(Succeed())
}

// sourceObjectMeta builds the metadata for a source CR created by the framework, including
// the annotation that marks it as ours to update and delete.
func sourceObjectMeta(src resolvedSource) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      src.Name,
		Namespace: src.Namespace,
		Annotations: map[string]string{
			utils.DeleteAnnotation: "true",
		},
	}
}

// ociRepositoryRef pins an OCIRepository to the given chart version, or to any version
// when none is given.
func ociRepositoryRef(chartVersion string) *sourcev1.OCIRepositoryRef {
	tag := strings.TrimPrefix(chartVersion, "v")
	if tag == "" {
		return &sourcev1.OCIRepositoryRef{SemVer: "*"}
	}
	return &sourcev1.OCIRepositoryRef{Tag: tag}
}

// updateOCIRepositoryTag patches the spec.ref.tag of an existing OCIRepository.
// Only a source the framework created is patched, so an upgrade test can never repoint a
// source that belongs to the cluster or another tenant.
func updateOCIRepositoryTag(ctx context.Context, name, namespace, tag string) {
	GinkgoHelper()

	registerSourceScheme()

	tag = strings.TrimPrefix(tag, "v")

	obj := &sourcev1.OCIRepository{}
	err := state.GetFramework().MC().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
	Expect(err).NotTo(HaveOccurred())

	Expect(frameworkOwnsSource(obj)).To(BeTrue(), fmt.Sprintf(
		"refusing to change the tag of OCIRepository %s/%s, which is not managed by this framework",
		namespace, name))

	if obj.Spec.Reference == nil {
		obj.Spec.Reference = &sourcev1.OCIRepositoryRef{}
	}
	obj.Spec.Reference.Tag = tag

	logger.Log("Updating OCIRepository %s/%s tag to %s", namespace, name, tag)
	err = state.GetFramework().MC().Update(ctx, obj, &cr.UpdateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func buildHelmRelease(cfg HelmReleaseConfig) *helmv2.HelmRelease {
	src := resolveSource(cfg)

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
				CreateNamespace: true,
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

	switch src.Kind {
	case SourceKindOCIRepository:
		hr.Spec.ChartRef = &helmv2.CrossNamespaceSourceReference{
			Kind:      string(SourceKindOCIRepository),
			Name:      src.Name,
			Namespace: src.Namespace,
		}
	case SourceKindHelmRepository:
		hr.Spec.Chart = &helmv2.HelmChartTemplate{
			Spec: helmv2.HelmChartTemplateSpec{
				Chart:   cfg.ChartName,
				Version: cfg.ChartVersion,
				SourceRef: helmv2.CrossNamespaceObjectReference{
					Kind:      string(SourceKindHelmRepository),
					Name:      src.Name,
					Namespace: src.Namespace,
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

	src := resolveSource(cfg)

	if src.Kind == SourceKindOCIRepository {
		updateOCIRepositoryTag(ctx, src.Name, src.Namespace, version)
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
