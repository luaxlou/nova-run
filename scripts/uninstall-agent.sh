#!/usr/bin/env bash
set -euo pipefail

systemctl stop nova-agent.service || true
systemctl disable nova-agent.service || true
rm -f /etc/systemd/system/nova-agent.service
systemctl daemon-reload || true
rm -f /usr/local/bin/nova-agent
echo "nova-agent removed"
