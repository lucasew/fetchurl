#!/bin/bash
set -e
mkdir -p dist
export CGO_ENABLED=0
echo "Building artifacts..."
go tool dist list | grep -vE 'wasm|aix|plan9|android|ios|illumos|solaris|dragonfly' | while IFS=/ read -r GOOS GOARCH; do
  echo "Building for $GOOS/$GOARCH"
  env GOOS=$GOOS GOARCH=$GOARCH go build -v -o dist/fetchurl-$GOOS-$GOARCH ./cmd/fetchurl || echo "Failed to build for $GOOS/$GOARCH"
done
