FROM golang:1.22

RUN apt-get update \
  && apt-get install --no-install-recommends --no-install-suggests -y ca-certificates jq yq \
  && rm -rf /var/lib/apt/lists/*

COPY <<EOF /entrypoint.sh
#!/usr/bin/env bash
set -e
cd /app
SUITES_TO_RUN=$(find \$1 -name '*_suite_test.go' | xargs)
shift
REPORT_DIR=\${REPORT_DIR:-/tmp/reports}
mkdir -p \${REPORT_DIR}
ginkgo --output-dir=\${REPORT_DIR} --junit-report=test-results.xml --timeout 4h --keep-going -v -r $@ \${SUITES_TO_RUN}
EOF

RUN chmod +x /entrypoint.sh

RUN go install github.com/onsi/ginkgo/v2/ginkgo@latest

ENTRYPOINT ["/entrypoint.sh"]
