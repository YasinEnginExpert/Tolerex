# ==========================================================
# BUILD STAGE
# ==========================================================
FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build static binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o leader ./cmd/leader && \
    go build -o member ./cmd/member

# ==========================================================
# RUNTIME STAGE
# ==========================================================
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/leader /app/leader
COPY --from=builder /app/member /app/member

# Config & logs Docker volume’dan gelecek
VOLUME ["/app/config", "/app/logs"]


EXPOSE 5555 9090 5556 9092

ENTRYPOINT ["/app/leader"]
