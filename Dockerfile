FROM golang:1.23 AS builder
WORKDIR /workspace

# Copy everything and build the app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build binaries for each component
RUN go build -o bin/api ./cmd/api
RUN go build -o bin/controller ./cmd/controller
RUN go build -o bin/worker ./cmd/worker

# Stage 2: Production images
FROM ubuntu:22.04 AS api
WORKDIR /app
COPY --from=builder /workspace/bin/api ./api
ENTRYPOINT ["./api"]

FROM ubuntu:22.04 AS controller
WORKDIR /app
COPY --from=builder /workspace/bin/controller ./controller
ENTRYPOINT ["./controller"]

FROM ubuntu:22.04 AS worker
WORKDIR /app
COPY --from=builder /workspace/bin/worker ./worker
ENTRYPOINT ["./worker"]
