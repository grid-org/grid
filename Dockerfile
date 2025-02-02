FROM golang:1.23 AS builder
WORKDIR /workspace

# Copy go.mod and go.sum and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build API binary
# FROM builder AS build-api
# RUN go build -o bin/api ./cmd/api

# Build Controller binary
FROM builder AS build-controller
RUN go build -o bin/controller ./cmd/controller

# Build Worker binary
FROM builder AS build-worker
RUN go build -o bin/worker ./cmd/worker

# Stage 2: Production images
FROM ubuntu:22.04 AS base
WORKDIR /app
# RUN apk add --no-cache ca-certificates
RUN apt-get update && apt-get install -y ca-certificates

# API image
# FROM base AS api
# COPY --from=build-api /workspace/bin/api /app/api
# ENTRYPOINT ["/app/api"]

# Controller image
FROM base AS controller
COPY --from=build-controller /workspace/bin/controller /app/controller
ENTRYPOINT ["/app/controller"]

# Worker image
FROM base AS worker
COPY --from=build-worker /workspace/bin/worker /app/worker
ENTRYPOINT ["/app/worker"]
