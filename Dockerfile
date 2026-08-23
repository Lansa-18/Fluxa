# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/worker ./cmd/worker

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget

RUN adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /app/bin/api .
COPY --from=builder /app/bin/worker .
COPY --from=builder /app/db/migrations ./db/migrations

RUN chown -R appuser:appuser /app

USER appuser

EXPOSE 3000

CMD ["/app/api"]
