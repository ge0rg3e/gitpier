#!/usr/bin/env bash

set -e

echo "Building server..."
go build -o ./main ./cmd/server/main.go

echo "Starting server..."
./main
