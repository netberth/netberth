#!/bin/sh
# NetBerth pre-flight health check
# Detects host network topology: IPv4, IPv6, gateway, interface list

echo "=== NetBerth Host Topology ==="

# IPv4 detection
echo -n "IPv4: "
if ip -4 addr show 2>/dev/null | grep -q 'inet '; then
  ip -4 addr show | grep 'inet ' | awk '{print $2}' | head -3 | tr '\n' ' '
  echo ""
else
  echo "none"
fi

# IPv6 detection
echo -n "IPv6: "
if ip -6 addr show 2>/dev/null | grep -q 'inet6 .*global'; then
  echo "ENABLED"
  ip -6 addr show | grep 'inet6 .*global' | awk '{print $2}' | head -2
else
  echo "none (or link-local only)"
fi

# Default route
echo -n "Gateway: "
ip route show default 2>/dev/null | awk '{print $3}' || echo "unknown"

# Nameserver check
echo -n "DNS: "
grep '^nameserver' /etc/resolv.conf 2>/dev/null | awk '{print $2}' | head -2 | tr '\n' ' ' || echo "unknown"
echo ""

# WebDAV boundary check
echo -n "WebDAV boundary: "
WEBDAV_ROOT="/storage"
if [ -d "$WEBDAV_ROOT" ]; then
  if touch "$WEBDAV_ROOT/.nh-test" 2>/dev/null; then
    rm -f "$WEBDAV_ROOT/.nh-test"
    echo "OK (writable)"
  else
    echo "OK (read-only)"
  fi
else
  echo "not mounted"
fi

# STUN connectivity test (quick)
echo -n "STUN (Google): "
timeout 2 nc -zu stun.l.google.com 19302 2>/dev/null && echo "reachable" || echo "unreachable"

echo "=== NetBerth Ready ==="
