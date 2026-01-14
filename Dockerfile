# ==============================================================================
# BUILD STAGE
# ==============================================================================
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install system dependencies (required for some CGO or SSL operations, though we aim for CGO_ENABLED=0)
RUN apk add --no-cache git

# Cache Dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy Source Code
COPY . .

# Build Binaries (Statically Linked)
# We build all three binaries in one go.
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/leader ./cmd/leader
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/member ./cmd/member
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/client ./client/test_client.go

# ==============================================================================
# RUNTIME STAGE
# ==============================================================================
# We use a small alpine image for the final runtime.
# Using 'scratch' is smaller but 'alpine' is better for debugging (sh, ls, etc.) in a lab.
FROM alpine:latest

WORKDIR /root/

# Install certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Copy binaries from builder
COPY --from=builder /bin/leader .
COPY --from=builder /bin/member .
COPY --from=builder /bin/client .

# Copy Configs and TLS Certificates
# Assumes these exist in the context. In a real production setup, these might be secrets.
COPY config ./config

# Expose ports
# Leader: 6666 (TCP), 5555 (gRPC), 9090 (Metrics)
# Member: 5556+ (gRPC), 9092+ (Metrics)
EXPOSE 6666 5555 9090 5556 9092

# Default entrypoint (can be overridden)
CMD ["./leader"]
