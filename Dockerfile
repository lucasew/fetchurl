FROM golang:1.24.0-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o fetchurl ./cmd/fetchurl

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/fetchurl /app/fetchurl
ENTRYPOINT ["/app/fetchurl"]
