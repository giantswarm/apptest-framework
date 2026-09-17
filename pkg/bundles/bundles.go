package bundles

import (
	"fmt"
	"strings"

	"github.com/giantswarm/cluster-standup-teardown/v6/pkg/values"
	"github.com/giantswarm/clustertest/v5/pkg/application"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

// AppNameOverrideType specifies how the child app name should be formatted in bundle values
type AppNameOverrideType int

const (
	// AppNameOverrideAuto automatically detects the naming convention based on the bundle app name
	AppNameOverrideAuto AppNameOverrideType = iota
	// AppNameOverrideCamelCase converts the app name to camelCase (e.g., "my-app" -> "myApp")
	AppNameOverrideCamelCase
	// AppNameOverrideHyphen keeps the app name with hyphens (e.g., "my-app" stays "my-app")
	AppNameOverrideHyphen
	// AppNameOverrideNone skips setting any override values, returning the bundle app unchanged
	AppNameOverrideNone
)

// OverrideChildApp takes two apps, a bundle app and a child app, and attempts to correctly set the values of the bundle app
// to have it install the desired version of the child app.
// The overrideType specifies the naming convention for the child app.
// If set to AppNameOverrideAuto, it will attempt to auto-detect based on the bundle app name.
//
// The child's chart name is assumed to match its app name. Use [OverrideChild] for a child
// whose chart is published under a different name.
func OverrideChildApp(bundleApp *application.Application, childApp *application.Application, overrideType AppNameOverrideType) (*application.Application, error) {
	return OverrideChild(bundleApp, ChildOverride{
		AppName:   childApp.AppName,
		Catalog:   childApp.Catalog,
		Version:   childApp.Version,
		Namespace: childApp.InstallNamespace,
	}, overrideType)
}

// OverrideChild sets the values of the bundle app so that it installs the described child.
// The overrideType specifies the naming convention for the child app.
// If set to AppNameOverrideAuto, it will attempt to auto-detect based on the bundle app name.
func OverrideChild(bundleApp *application.Application, child ChildOverride, overrideType AppNameOverrideType) (*application.Application, error) {
	valuesLayer, err := ChildValues(bundleApp.AppName, child, overrideType)
	if err != nil {
		return nil, err
	}
	if valuesLayer == "" {
		// No override values, return bundle app unchanged
		return bundleApp, nil
	}

	finalValues, err := values.Merge(bundleApp.Values, valuesLayer)
	if err != nil {
		return nil, err
	}

	return bundleApp.WithValues(finalValues, &application.TemplateValues{})
}

// ChildValues returns the values layer that makes a bundle install the described child.
//
// The layer is returned rather than applied so that both install paths can share it: the App
// CR path merges it into the bundle App's values, the Flux path into the parent HelmRelease's
// values. bundleAppName is only used to pick the naming convention, and only when overrideType
// is AppNameOverrideAuto. An empty layer is returned for AppNameOverrideNone.
func ChildValues(bundleAppName string, child ChildOverride, overrideType AppNameOverrideType) (string, error) {
	appName := child.AppName

	switch overrideType {
	case AppNameOverrideNone:
		return "", nil
	case AppNameOverrideCamelCase:
		appName = toCamelCase(appName)
	case AppNameOverrideHyphen:
		// Keep as-is (hyphenated)
	case AppNameOverrideAuto:
		fallthrough
	default:
		// Auto-detect based on bundle app name
		if isCamelCaseName(bundleAppName) {
			appName = toCamelCase(appName)
		} else if !isHyphenName(bundleAppName) {
			return "", fmt.Errorf("provided bundle is unsupported, child version override format is unknown")
		}
	}

	chartName := child.ChartName
	if chartName == "" {
		chartName = child.AppName
	}

	overrideValues := bundleValues{
		Apps: map[string]appValues{
			appName: {
				Enabled:   true,
				Catalog:   child.Catalog,
				Version:   child.Version,
				AppName:   child.AppName,
				ChartName: chartName,
				Namespace: child.Namespace,
			},
		},
	}

	valuesLayer, err := yaml.Marshal(overrideValues)
	if err != nil {
		return "", err
	}

	return string(valuesLayer), nil
}

// toCamelCase converts a hyphenated app name to camelCase
func toCamelCase(appName string) string {
	appName = strings.ReplaceAll(appName, "-", " ")
	appName = cases.Title(language.English).String(appName)
	appName = strings.ReplaceAll(appName, " ", "")
	return strings.ToLower(appName[:1]) + appName[1:]
}

func isCamelCaseName(appName string) bool {
	return strings.EqualFold(appName, "security-bundle") ||
		strings.EqualFold(appName, "observability-bundle") ||
		strings.EqualFold(appName, "gateway-api-bundle")
}

func isHyphenName(appName string) bool {
	return strings.EqualFold(appName, "service-mesh-bundle") ||
		strings.EqualFold(appName, "auth-bundle")
}
