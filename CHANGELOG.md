# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).



## [Unreleased]

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

[Unreleased]: https://github.com/giantswarm/apptest-framework/compare/v0.0.6...HEAD
[0.0.6]: https://github.com/giantswarm/apptest-framework/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/giantswarm/apptest-framework/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/giantswarm/apptest-framework/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/giantswarm/apptest-framework/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/giantswarm/apptest-framework/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/giantswarm/apptest-framework/releases/tag/v0.0.1
