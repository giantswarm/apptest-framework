// package state provides a singleton that can be used to share data across all test cases within a test suite
// The state can contain the following:
// - Framework - An initialized `clustertest` framework client pointing at the test management cluster
// - Cluster - A Cluster object with details about the test workload cluster
// - Application - An Application object with details abou the App being tested
// - Context - A context instance
package state
