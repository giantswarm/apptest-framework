#!/usr/bin/env bash

set -e

cd /app

SUITES_TO_RUN=$(find $1 -name '*_suite_test.go' | xargs dirname | uniq | xargs)
shift

REPORT_DIR=${REPORT_DIR:-/tmp/reports}
mkdir -p ${REPORT_DIR}

echo "Building test suites"
ginkgo build ${SUITES_TO_RUN} &1>/dev/null

ginkgo --output-dir=${REPORT_DIR} --junit-report=test-results.xml --timeout 4h --keep-going -v -r $@ ${SUITES_TO_RUN}
