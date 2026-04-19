FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/control-plane ./cmd/control-plane

FROM alpine:3.21
ARG CONFIG_SOURCE
RUN case "$CONFIG_SOURCE" in PROD|TEST) ;; *) echo "CONFIG_SOURCE must be PROD or TEST" >&2; exit 1;; esac
ARG CONFIG_REPO_URL="https://github.com/mph-tech/autodev.git"
ARG CONFIG_COMMIT="main"

ENV CONFIG_SOURCE=${CONFIG_SOURCE}
ENV CONFIG_REPO_URL=${CONFIG_REPO_URL}
ENV CONFIG_COMMIT=${CONFIG_COMMIT}

WORKDIR /app
RUN apk add --no-cache git
RUN addgroup -S autodev && \
    adduser -S -G autodev autodev

COPY --from=build /out/control-plane /usr/local/bin/control-plane
COPY prompts ./prompts

COPY docker/control-plane/boot.sh /usr/local/bin/boot.sh
COPY docker/control-plane/prepare-workspace.sh /usr/local/bin/prepare-workspace.sh
RUN chmod +x /usr/local/bin/boot.sh /usr/local/bin/prepare-workspace.sh

EXPOSE 9090
ENTRYPOINT ["/usr/local/bin/boot.sh"]
