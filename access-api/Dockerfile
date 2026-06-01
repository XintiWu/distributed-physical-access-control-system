FROM golang:1.24-alpine AS builder
WORKDIR /app/access-api
COPY pkg/middleware /app/pkg/middleware
COPY access-api/go.mod access-api/go.sum* ./
RUN go mod download 2>/dev/null || true
COPY access-api/ .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /access-api ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /access-api .
RUN mkdir -p /data/outbox && chown -R nobody:nobody /data
USER nobody
EXPOSE 8080
CMD ["./access-api"]
