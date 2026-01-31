# hadolint ignore=DL3007
FROM jdxcode/mise:latest AS builder

WORKDIR /app
COPY go.mod go.sum mise.toml ./
RUN mise trust && mise install go
RUN mise exec go -- go mod download
COPY . .
# Disable CGO for static build compatible with Alpine
RUN mise exec go -- env CGO_ENABLED=0 go build -o dist/fetchurl ./cmd/fetchurl

FROM alpine:3.21
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/dist/fetchurl /app/fetchurl
ENTRYPOINT ["/app/fetchurl"]
