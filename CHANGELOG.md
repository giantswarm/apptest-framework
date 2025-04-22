# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Fix linting issues.

### Added

- Added `WithInstallName` function to override the name the App is installed as, prefixed with the cluster name. (Defaults to app name)

## [1.12.0] - 2025-02-20

### Added

- Add support for gateway-api-bundle.

### Changed

- Enable JSON output for test results.
- Update to Go v1.24

## [1.11.0] - 2025-01-21

### Changed

- Updated `cluster-standup-teardown` to stop using spot instances for CAPA WCs

## [1.10.2] - 2024-10-08

### Fixed

- Update `clustertest` and `cluster-standup-teardown` with fix for loading existing workload cluster

## [1.10.1] - 2024-09-19

### Fixed

- Update `clustertest` with GitHub latest release fix

## [1.10.0] - 2024-09-16

### Changed

- Upgraded Go to v1.23.1 and updated all modules to match

## [1.9.0] - 2024-09-16

## [1.8.0] - 2024-09-06

### Changed

- Updated all dependencies to latest version

## [1.7.1] - 2024-09-02

### Fixed

- Updated clustertest with fix for version prefix on releases

## [1.7.0] - 2024-08-15

### Documentation

- Added more docs on how to write App tests

### Changed

- Updated all modules to latest (including support for Kubernetes v1.31)

### Fixed

- Replace `containerdVolumeSizeGB` and `kubeletVolumeSizeGB` with `libVolumeSizeGB` (from cluster-standup-teardown upgrade)

## [1.6.0] - 2024-07-22

### Added

- Support for testing default apps (and those within default bundles) as long as testing against a provider that supports Releases
- Added example test suites for bundles and default apps

## [1.5.0] - 2024-06-25

### Changed

- Updated all dependencies to latest version

## [1.4.0] - 2024-06-24

### Changed

- Updated all depenedncies to latest version

## [1.3.0] - 2024-06-10

### Changed

- Update `clustertest` to v1.0.0 to support Releases with cluster Apps
- Update `cluster-standup-teardown` to v1.5.0 to support Releases with cluster Apps

## [1.2.1] - 2024-06-10

### Changed

- Bump Go version to v1.22 in Dockerfile

## [1.2.0] - 2024-05-28

### Added

- Added example E2E test suite that uses the hello-world app to self-test the framework
- Added `AfterSuite` hook to allow performing cleanup tasks after tests have finished.

## [1.1.4] - 2024-05-27

### Fixed

- Allow overriding the working directory in entrypoint.sh

## [1.1.3] - 2024-05-21

### Fixed

- Ensure WC is always deleted during AfterSuite even if App uninstall fails
- Handle providers with consolidated default-apps

## [1.1.2] - 2024-05-20

### Changed

- Bumped clustertest and cluster-standup-teardown to latest

## [1.1.1] - 2024-05-12

### Changed

- Added pipefail to entrypoint.sh
- Added log output to entrypoint.sh to indicate current progress

## [1.1.0] - 2024-05-09

### Added

- Added support for running tests against an App running as a child app of a Bundle App by using the new `InAppBundle` function.

### Fixed

- Check for defined number of control plane replicas instead of hardcoded to 3.

## [1.0.0] - 2024-04-30

### Added

- Workload Cluster creation and deletion is now handled in code using `cluster-standup-teardown`

### Changed

- Change `BeforeInstall` hook to be `AfterClusterReady` as wouldn't make sense for default apps that are installed as part of the cluster creation

## [0.0.7] - 2024-04-11

### Changed

- Make clustertest logger use the GinkgoWriter

## [0.0.6] - 2024-03-15

### Added

- Added `Providers` property to config

## [0.0.5] - 2024-03-04

### Removed

- Removed pre-build as it still ended up re-building on run.

## [0.0.4] - 2024-03-04

### Added

- Build the test suites first so the build output can be suppressed from the test logs

## [0.0.3] - 2024-03-01

## [0.0.2] - 2024-02-29

### Added

- Dockerfile for running the tests within

## [0.0.1] - 2024-02-27

### Added

- Suite package to handle setup and running of an App test suite
- State package to share data between test cases
- Config package to provide standard app configuration
- Client package to abstract some test functionality

[Unreleased]: https://github.com/giantswarm/apptest-framework/compare/v1.12.0...HEAD
[1.12.0]: https://github.com/giantswarm/apptest-framework/compare/v1.11.0...v1.12.0
[1.11.0]: https://github.com/giantswarm/apptest-framework/compare/v1.10.2...v1.11.0
[1.10.2]: https://github.com/giantswarm/apptest-framework/compare/v1.10.1...v1.10.2
[1.10.1]: https://github.com/giantswarm/apptest-framework/compare/v1.10.0...v1.10.1
[1.10.0]: https://github.com/giantswarm/apptest-framework/compare/v1.9.0...v1.10.0
[1.9.0]: https://github.com/giantswarm/apptest-framework/compare/v1.8.0...v1.9.0
[1.8.0]: https://github.com/giantswarm/apptest-framework/compare/v1.7.1...v1.8.0
[1.7.1]: https://github.com/giantswarm/apptest-framework/compare/v1.7.0...v1.7.1
[1.7.0]: https://github.com/giantswarm/apptest-framework/compare/v1.6.0...v1.7.0
[1.6.0]: https://github.com/giantswarm/apptest-framework/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/giantswarm/apptest-framework/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/giantswarm/apptest-framework/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/giantswarm/apptest-framework/compare/v1.2.1...v1.3.0
[1.2.1]: https://github.com/giantswarm/apptest-framework/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/giantswarm/apptest-framework/compare/v1.1.4...v1.2.0
[1.1.4]: https://github.com/giantswarm/apptest-framework/compare/v1.1.3...v1.1.4
[1.1.3]: https://github.com/giantswarm/apptest-framework/compare/v1.1.2...v1.1.3
[1.1.2]: https://github.com/giantswarm/apptest-framework/compare/v1.1.1...v1.1.2
[1.1.1]: https://github.com/giantswarm/apptest-framework/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/giantswarm/apptest-framework/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/giantswarm/apptest-framework/compare/v0.0.7...v1.0.0
[0.0.7]: https://github.com/giantswarm/apptest-framework/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/giantswarm/apptest-framework/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/giantswarm/apptest-framework/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/giantswarm/apptest-framework/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/giantswarm/apptest-framework/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/giantswarm/apptest-framework/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/giantswarm/apptest-framework/releases/tag/v0.0.1
