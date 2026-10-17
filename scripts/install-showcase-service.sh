#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
mkdir -p "$unit_dir"
# systemd does not inherit the interactive shell's PATH.
cat > "$unit_dir/jana-showcase.service" <<EOF
[Unit]
Description=Jana showcase Docker build and deployment watcher
After=default.target

[Service]
Type=simple
WorkingDirectory=$PWD
ExecStart=/bin/bash "$PWD/scripts/showcase.sh" watch
Environment=PATH=/usr/local/bin:/usr/bin:/bin
Restart=always
RestartSec=15
TimeoutStopSec=30
KillMode=control-group
UMask=0077

[Install]
WantedBy=default.target
EOF
systemctl --user daemon-reload
systemctl --user enable --now jana-showcase.service
echo 'Deployment watcher installed. Logs: journalctl --user -u jana-showcase -f'
echo 'For startup without login, enable user lingering: loginctl enable-linger "$USER"'
