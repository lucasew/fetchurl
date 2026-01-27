FROM jdxcode/mise:latest AS builder

WORKDIR /app
COPY go.mod go.sum ./
# Install go using mise
RUN mise use -g go@1.24.0
RUN mise install
# Ensure go is in path or run with mise exec
RUN mise exec -- go mod download
COPY . .
# Disable CGO for static build compatible with Alpine
RUN mise exec -- env CGO_ENABLED=0 go build -o fetchurl ./cmd/fetchurl

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/fetchurl /app/fetchurl
ENTRYPOINT ["/app/fetchurl"]
