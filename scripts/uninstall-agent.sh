#!/usr/bin/env bash
set -euo pipefail

systemctl stop nova@.service || true
systemctl disable nova@.service || true
rm -f /etc/systemd/system/nova@.service
systemctl daemon-reload || true
rm -f /usr/local/bin/nova-agent
echo "nova-agent removed"

