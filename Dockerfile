# ================= BUILD =================
FROM golang:1.24-alpine AS builder

WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o leader ./cmd/leader && \
    go build -o member ./cmd/member

# ================= RUNTIME =================
FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=builder /app/leader /app/leader
COPY --from=builder /app/member /app/member

VOLUME ["/app/config", "/app/logs"]

EXPOSE 5555 9090
ENTRYPOINT ["/app/leader"]
