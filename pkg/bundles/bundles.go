package bundles

import (
	"fmt"
	"strings"

	"github.com/giantswarm/cluster-standup-teardown/v2/pkg/values"
	"github.com/giantswarm/clustertest/v3/pkg/application"
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
func OverrideChildApp(bundleApp *application.Application, childApp *application.Application, overrideType AppNameOverrideType) (*application.Application, error) {
	appName := childApp.AppName

	switch overrideType {
	case AppNameOverrideNone:
		// No override values, return bundle app unchanged
		return bundleApp, nil
	case AppNameOverrideCamelCase:
		appName = toCamelCase(appName)
	case AppNameOverrideHyphen:
		// Keep as-is (hyphenated)
	case AppNameOverrideAuto:
		fallthrough
	default:
		// Auto-detect based on bundle app name
		if isCamelCaseName(bundleApp.AppName) {
			appName = toCamelCase(appName)
		} else if !isHyphenName(bundleApp.AppName) {
			return nil, fmt.Errorf("provided bundle is unsupported, child version override format is unknown")
		}
	}

	overrideValues := bundleValues{
		Apps: map[string]appValues{
			appName: {
				Enabled:   true,
				Catalog:   childApp.Catalog,
				Version:   childApp.Version,
				AppName:   childApp.AppName,
				ChartName: childApp.AppName,
				Namespace: childApp.InstallNamespace,
			},
		},
	}

	valuesLayer, err := yaml.Marshal(overrideValues)
	if err != nil {
		return nil, err
	}

	finalValues, err := values.Merge(bundleApp.Values, string(valuesLayer))
	if err != nil {
		return nil, err
	}

	return bundleApp.WithValues(finalValues, &application.TemplateValues{})
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
