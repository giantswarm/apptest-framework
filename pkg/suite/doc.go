// package suite provides a standard way of setting up and triggering an App test suite
//
// The installation, upgrade (if required) and uninstallation of the App is handled within the test suite.
// Hooks are provided for users to add pre-install, pre-upgrade and post-install test cases.
//
// # Example
//
//	func TestBasic(t *testing.T) {
//		suite.New().
//			WithInstallNamespace("kube-system").
//			WithIsUpgrade(isUpgrade).
//			WithValuesFile("./values.yaml").
//			AfterClusterReady(func() {
//				// Do any pre-install checks here (ensure the cluster has needed pre-reqs)
//			}).
//			BeforeUpgrade(func() {
//				// Perform any checks between installing the latest released version
//				// and upgrading it to the version to test
//				// E.g. ensure that the initial install has completed and has settled before upgrading
//			}).
//			Tests(func() {
//				// Include calls to app tests here
//			}).
//			AfterSuite(func() {
//				// Include any post-test cleanup
//			})
//			Run(t, "Basic Test")
//	}
package suite
