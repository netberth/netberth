#!/bin/sh
# NetBerth one-command upgrade script
# Usage: ./scripts/upgrade.sh [version]
# Preserves all data, auto-backup, auto-migration

set -e
VERSION="${1:-latest}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.prod.yml}"

echo "=== NetBerth Upgrade to $VERSION ==="

# Backup
echo "[1/4] Backing up database..."
BACKUP_FILE="netberth-backup-$(date +%Y%m%d-%H%M%S).db"
if [ -f data/netberth.db ]; then
  cp data/netberth.db "$BACKUP_FILE"
  echo "  Backup: $BACKUP_FILE ($(du -h "$BACKUP_FILE" | cut -f1))"
fi

# Pull new image (if using registry)
echo "[2/4] Pulling new image..."
if docker compose -f "$COMPOSE_FILE" pull 2>/dev/null; then
  echo "  Image pulled from registry"
else
  echo "  Building from source..."
  docker compose -f "$COMPOSE_FILE" build --no-cache
fi

# Stop old container
echo "[3/4] Restarting with new image..."
docker compose -f "$COMPOSE_FILE" down

# Start new container
docker compose -f "$COMPOSE_FILE" up -d

# Verify
echo "[4/4] Verifying..."
sleep 3
if docker compose -f "$COMPOSE_FILE" logs --tail=5 2>/dev/null | grep -q "NetBerth starting"; then
  echo "  Upgrade successful!"
  docker compose -f "$COMPOSE_FILE" logs --tail=2
else
  echo "  WARNING: Check logs with: docker compose -f $COMPOSE_FILE logs"
fi

echo ""
echo "=== Upgrade complete ==="
echo "Backup saved: $BACKUP_FILE"
