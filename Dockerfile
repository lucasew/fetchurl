# hadolint ignore=DL3007
FROM jdxcode/mise:latest AS builder

WORKDIR /app
COPY go.mod go.sum mise.toml ./
# Remove non-Go tools from mise.toml to prevent mise from trying to install them
RUN sed -i -E '/^(golangci-lint|actionlint|node|yamllint|hadolint|"github:sqlc-dev\/sqlc")/d' mise.toml
RUN mise trust && mise install
RUN mise run install
COPY . .
# Disable CGO for static build compatible with Alpine
RUN mise exec -- env CGO_ENABLED=0 go build -o dist/fetchurl ./cmd/fetchurl

FROM alpine:3.21
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/dist/fetchurl /app/fetchurl
ENTRYPOINT ["/app/fetchurl"]
