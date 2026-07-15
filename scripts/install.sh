#!/bin/sh
# NetBerth — One-click install script
# Supports: Linux (amd64/arm64), Unraid, Synology DSM, OpenWrt (via Docker)

set -e

RED='\033[0;31m'; GREEN='\033[0;32m'; NC='\033[0m'
echo "${GREEN}=== NetBerth Installer ===${NC}"

# Detect platform
detect_platform() {
  if [ -f /etc/unraid-version ]; then echo "unraid"
  elif [ -f /etc/synoinfo.conf ]; then echo "synology"
  elif [ -f /etc/openwrt_release ]; then echo "openwrt"
  else echo "linux"; fi
}
PLATFORM=$(detect_platform)
echo "Detected: $PLATFORM"

# JWT secret
if [ -z "$NB_JWT_SECRET" ]; then
  NB_JWT_SECRET=$(openssl rand -base64 48 2>/dev/null || cat /dev/urandom | head -c 48 | base64 2>/dev/null || echo "change-me-$(date +%s)")
  echo "Generated JWT secret"
fi

case $PLATFORM in
  unraid|linux)
    echo "Installing via Docker Compose..."
    if ! command -v docker >/dev/null 2>&1; then
      echo "${RED}Docker not found. Install Docker first: https://docs.docker.com/engine/install/${NC}"
      exit 1
    fi
    mkdir -p netberth-data netberth-certs
    chmod 777 netberth-data netberth-certs

    if command -v docker-compose >/dev/null 2>&1; then
      DC="docker-compose"
    elif docker compose version >/dev/null 2>&1; then
      DC="docker compose"
    else
      echo "${RED}Docker Compose not found${NC}"
      exit 1
    fi

    cat > docker-compose.yml << EOF
services:
  netberth:
    image: netberth:latest
    container_name: netberth
    restart: unless-stopped
    network_mode: host
    environment:
      - NB_SERVER_PORT=8443
      - NB_JWT_SECRET=$NB_JWT_SECRET
      - TZ=\${TZ:-Asia/Shanghai}
    volumes:
      - ./netberth-data:/app/data
      - ./netberth-certs:/app/certs
    logging:
      driver: json-file
      options: { max-size: "10m", max-file: "3" }
EOF

    if ! docker image inspect netberth:latest >/dev/null 2>&1; then
      echo "Building image (first time, ~3 minutes)..."
      if [ -f Dockerfile ]; then
        docker build -t netberth:latest .
      else
        echo "${RED}Dockerfile not found. Run from NetBerth project directory.${NC}"
        exit 1
      fi
    fi

    $DC up -d
    sleep 3
    echo ""
    echo "${GREEN}=== NetBerth installed! ===${NC}"
    echo "Admin panel: http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo 'YOUR_IP'):8443"
    echo "Username: admin"
    $DC logs 2>&1 | grep "ADMIN CREDENTIALS" || echo "Password: check logs with: $DC logs | grep 'ADMIN CREDENTIALS'"
    ;;

  synology)
    echo "For Synology DSM 7.x, use Container Manager:"
    echo "1. Open Container Manager > Project > Create"
    echo "2. Project name: netberth"
    echo "3. Paste docker-compose.yml content from above"
    echo "4. Set NB_JWT_SECRET=$NB_JWT_SECRET"
    echo "5. Click Next > Done"
    echo ""
    echo "Or use SSH and run this script on the NAS directly."
    ;;

  openwrt)
    echo "OpenWrt detected. Using Docker if available..."
    if command -v docker >/dev/null 2>&1; then
      PLATFORM=linux;  # Re-run linux path
    else
      echo "${RED}Docker not found on OpenWrt. Install Docker or use a different machine.${NC}"
      exit 1
    fi
    ;;
esac

echo ""
echo "After login, immediately change the password in Settings."
echo "Documentation: open the app and check the Settings > About page."
