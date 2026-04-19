FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY internal ./internal
COPY tooling ./tooling
COPY cmd ./cmd
RUN go build -o /out/autodev-stage-runtime ./tooling/runtime
RUN go build -o /out/cognition ./cmd/cognition

FROM alpine:3.21
ARG CONFIG_SOURCE
RUN case "$CONFIG_SOURCE" in PROD|TEST) ;; *) echo "CONFIG_SOURCE must be PROD or TEST" >&2; exit 1;; esac
ARG CONFIG_REPO_URL="https://github.com/tonyatmph/autodev-test-config.git"
ARG CONFIG_COMMIT="master"

ENV CONFIG_SOURCE=${CONFIG_SOURCE}
ENV CONFIG_REPO_URL=${CONFIG_REPO_URL}
ENV CONFIG_COMMIT=${CONFIG_COMMIT}
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/tooling/cognition/bin
ENV TMPDIR=/tmp

WORKDIR /app
RUN apk add --no-cache git jq

RUN addgroup -S autodev &&     adduser -S -G autodev autodev &&     adduser -S -G autodev agent &&     adduser -S -G autodev generator &&     adduser -S -G autodev governed

COPY --from=build /out/autodev-stage-runtime /usr/local/bin/autodev-stage-runtime
COPY --from=build /out/cognition /tooling/cognition/bin/cognition
COPY prompts ./prompts
COPY tooling /tooling
COPY docker/runner/boot.sh /usr/local/bin/boot.sh
COPY docker/runner/prepare-workspace.sh /usr/local/bin/prepare-workspace.sh
RUN chmod +x /usr/local/bin/boot.sh /usr/local/bin/prepare-workspace.sh

ENTRYPOINT ["/usr/local/bin/boot.sh"]
