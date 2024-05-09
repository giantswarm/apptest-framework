#!/usr/bin/env bash

set -e
set -o pipefail

cd /app

SUITES_TO_RUN=$(find $1 -name '*_suite_test.go' | xargs dirname | uniq | xargs)
shift

echo "About to run the following test suites: ${SUITES_TO_RUN}"

REPORT_DIR=${REPORT_DIR:-/tmp/reports}
mkdir -p ${REPORT_DIR}
echo "Test results will be saved to: ${REPORT_DIR}"

echo "🛠️ Building test suites... (this may take a short while with no log output)"
ginkgo --output-dir=${REPORT_DIR} --junit-report=test-results.xml --timeout 4h --keep-going -v -r $@ ${SUITES_TO_RUN}
