FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY cmd/cognition ./cmd/cognition
COPY tooling/lib/llm ./tooling/lib/llm
RUN go build -o /usr/local/bin/cognition ./cmd/cognition/main.go

FROM alpine:latest
RUN apk add --no-cache jq
COPY --from=builder /usr/local/bin/cognition /usr/local/bin/cognition
# Assuming we have a boot/prepare script...
ENTRYPOINT ["/usr/local/bin/cognition"]
