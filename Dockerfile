FROM node:22-alpine AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev sqlite-dev
WORKDIR /src
COPY . .
RUN mkdir -p internal/api/handler/webroot
COPY --from=web-builder /dist/web/ internal/api/handler/webroot/
RUN CGO_ENABLED=1 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o netberth ./cmd/netberth

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata sqlite-libs curl openssl
WORKDIR /app
COPY --from=go-builder /src/netberth .
COPY scripts/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && mkdir -p /app/data /app/config /app/certs
EXPOSE 8443
VOLUME ["/app/data", "/app/config", "/app/certs"]
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -sf http://localhost:8443/api/v1/system/status || exit 1
ENTRYPOINT ["/app/entrypoint.sh"]
