FROM golang:1.24-alpine AS builder
WORKDIR /app/report-api
COPY pkg/middleware /app/pkg/middleware
COPY report-api/go.mod report-api/go.sum* ./
RUN go mod download 2>/dev/null || true
COPY report-api/ .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /report-api ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /report-api .
COPY report-api/report-ui ./report-ui
RUN mkdir -p /app/exports && chown -R nobody:nobody /app/exports /app
USER nobody
EXPOSE 8082
CMD ["./report-api"]
