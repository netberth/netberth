FROM alpine:3.20
COPY netberth-linux /app/netberth
COPY entrypoint.sh /app/entrypoint.sh
COPY healthcheck.sh /app/healthcheck.sh
RUN chmod +x /app/netberth /app/entrypoint.sh /app/healthcheck.sh && mkdir -p /app/data /app/config /app/certs /storage
EXPOSE 8443
VOLUME ["/app/data", "/app/config", "/app/certs", "/storage"]
ENTRYPOINT ["/app/entrypoint.sh"]
