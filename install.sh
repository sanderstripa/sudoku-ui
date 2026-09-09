#!/usr/bin/env bash
set -Eeuo pipefail

# Override with SUDOKU_UI_REPO only when installing from a fork.
REPO="${SUDOKU_UI_REPO:-sanderstripa/sudoku-ui}"
BRANCH="${SUDOKU_UI_BRANCH:-main}"
INSTALL_DIR="/opt/sudoku-ui-src"
BIN="/usr/local/bin/sudoku-ui"

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "Run as root: sudo bash install.sh"
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ca-certificates curl jq qrencode unzip tar git golang-go iproute2 >/dev/null

mkdir -p "$INSTALL_DIR"
rm -rf "$INSTALL_DIR"/*
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "https://github.com/${REPO}/archive/refs/heads/${BRANCH}.tar.gz" -o "$TMP/src.tar.gz"
tar -xzf "$TMP/src.tar.gz" -C "$TMP"
SRC="$(find "$TMP" -mindepth 1 -maxdepth 1 -type d | head -1)"
cp -a "$SRC"/. "$INSTALL_DIR"/

cd "$INSTALL_DIR"
VERSION="${SUDOKU_UI_VERSION:-v0.1.3}"
go build -trimpath -ldflags "-s -w -X main.Version=${VERSION} -X main.PanelRepo=${REPO}" -o "$BIN" .
chmod 755 "$BIN"

cat >/etc/systemd/system/sudoku-ui.service <<'UNIT'
[Unit]
Description=Sudoku UI management panel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sudoku-ui
Restart=on-failure
RestartSec=2
User=root
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true

[Install]
WantedBy=multi-user.target
UNIT

cat >/etc/systemd/system/sudoku@.service <<'UNIT'
[Unit]
Description=Sudoku Proxy Instance %i
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sudoku -c /etc/sudoku-ui/connections/%i.json
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload

echo "Installing latest official SUDOKU-ASCII core..."
"$BIN" --install-core

PORT=""
for _ in $(seq 1 100); do
  CANDIDATE=$((20000 + RANDOM % 30000))
  if ! ss -lnt "sport = :$CANDIDATE" 2>/dev/null | grep -q LISTEN; then PORT="$CANDIDATE"; break; fi
done
[[ -n "$PORT" ]] || PORT=2095

USER_NAME="admin-$(od -An -N3 -tx1 /dev/urandom | tr -d ' \n')"
PASSWORD="$(od -An -N12 -tx1 /dev/urandom | tr -d ' \n')"
"$BIN" --init --username "$USER_NAME" --password "$PASSWORD" --listen ":$PORT" --repo "$REPO"

systemctl enable --now sudoku-ui >/dev/null

if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
  ufw allow "$PORT/tcp" >/dev/null
fi

IP="$(curl -4fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"
if [[ -z "$IP" ]]; then IP="$(hostname -I | awk '{print $1}')"; fi

echo
echo "══════════════════════════════════════════════════════"
echo "  Sudoku UI installed"
echo "══════════════════════════════════════════════════════"
echo
echo "Panel:    http://${IP}:${PORT}"
echo "Username: ${USER_NAME}"
echo "Password: ${PASSWORD}"
echo
echo "Save these credentials."
echo "Service: systemctl status sudoku-ui"
echo "══════════════════════════════════════════════════════"
