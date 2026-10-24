# Build
FROM golang:1.26-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

# Runtime
FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 1000 app

COPY --from=builder /out/server /app/server
COPY migrations /app/migrations

ENV ADDR=:8080 \
    DB_PATH=/data/anitaqlet.db \
    MIGRATIONS_DIR=/app/migrations \
    FRONTEND_DIR=/app/frontend

RUN mkdir -p /data && chown -R app:app /data /app
USER app

VOLUME ["/data"]
EXPOSE 8080

CMD ["/app/server"]
