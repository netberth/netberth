FROM node:20-alpine AS web-builder
WORKDIR /web
COPY web/package.json ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.22-alpine AS go-builder
ENV GOPROXY=https://goproxy.cn,direct
RUN apk add --no-cache gcc musl-dev sqlite-dev
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN mkdir -p internal/api/handler/webroot
COPY --from=web-builder /dist/web/ internal/api/handler/webroot/
RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o netberth ./cmd/netberth

FROM alpine:3.20
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
