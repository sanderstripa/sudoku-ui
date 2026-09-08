#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${EUID:-$(id -u)} -eq 0 ]] || { echo "Run as root"; exit 1; }
read -r -p "Remove Sudoku UI and all panel-managed connections? [y/N] " ans
[[ "$ans" =~ ^[Yy]$ ]] || exit 0
systemctl disable --now sudoku-ui 2>/dev/null || true
for unit in $(systemctl list-units --all 'sudoku@*.service' --no-legend 2>/dev/null | awk '{print $1}'); do systemctl disable --now "$unit" || true; done
rm -f /etc/systemd/system/sudoku-ui.service /etc/systemd/system/sudoku@.service
rm -f /usr/local/bin/sudoku-ui
rm -rf /etc/sudoku-ui /opt/sudoku-ui-src
systemctl daemon-reload
echo "Sudoku UI removed. Official Sudoku core binary was left at /usr/local/bin/sudoku."
