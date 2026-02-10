FROM golang:1.26.0

RUN apt-get update \
  && apt-get install --no-install-recommends --no-install-suggests -y ca-certificates jq yq \
  && rm -rf /var/lib/apt/lists/*

RUN go install github.com/onsi/ginkgo/v2/ginkgo@latest

ADD entrypoint.sh /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
