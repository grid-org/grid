FROM golang:1.23 AS builder
WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build Controller binary
FROM builder AS build-controller
RUN go build -o bin/controller ./cmd/controller

# Build Worker binary
FROM builder AS build-worker
RUN go build -o bin/worker ./cmd/worker

# Base production image
FROM ubuntu:22.04 AS base
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*

# Controller image
FROM base AS controller
COPY --from=build-controller /workspace/bin/controller /app/controller
ENTRYPOINT ["/app/controller"]

# Worker image
FROM base AS worker
COPY --from=build-worker /workspace/bin/worker /app/worker
ENTRYPOINT ["/app/worker"]
