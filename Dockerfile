FROM golang:1.24-alpine

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build leader & member
RUN go build -o leader ./cmd/leader \
 && go build -o member ./cmd/member

EXPOSE 5000

CMD ["./leader"]