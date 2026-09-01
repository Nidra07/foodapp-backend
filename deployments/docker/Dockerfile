# --- build stage ---
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# Generate sqlc code required by the Go build
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0
RUN sqlc generate

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/api ./cmd/api

# --- runtime stage ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY db/migrations /app/db/migrations

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/api"]
