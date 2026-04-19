FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -o /echo-service .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /echo-service .
EXPOSE 8080
CMD ["./echo-service"]
