#!/usr/bin/env bash
set -Eeuo pipefail
REPO="${SUDOKU_UI_REPO:-sanderstripa/sudoku-ui}"; CORE_REPO="SUDOKU-ASCII/sudoku"
BIN=/usr/local/bin/sudoku-ui; CORE_BIN=/usr/local/bin/sudoku
die(){ echo "Ошибка: $*" >&2; exit 1; }
[[ ${EUID:-$(id -u)} -eq 0 ]] || die "запустите установку от root"
[[ -r /etc/os-release ]] || die "поддерживаются Ubuntu и Debian"
. /etc/os-release
[[ ${ID:-} == ubuntu || ${ID:-} == debian ]] || die "поддерживаются Ubuntu и Debian"
case "$(uname -m)" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) die "неподдерживаемая архитектура $(uname -m)";; esac
export DEBIAN_FRONTEND=noninteractive
CADDY_WAS_PRESENT=0; command -v caddy >/dev/null 2>&1 && CADDY_WAS_PRESENT=1
apt-get update -qq
apt-get install -y -qq ca-certificates curl jq qrencode tar iproute2 caddy >/dev/null
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
release_asset(){ curl -fsSL "https://api.github.com/repos/$1/releases/latest" | jq -r --arg n "$2" '.assets[]|select(.name==$n)|.browser_download_url' | head -n1; }

echo "Устанавливаем Sudoku UI…"
PANEL_URL="$(release_asset "$REPO" "sudoku-ui-linux-${ARCH}")"
[[ -n "$PANEL_URL" ]] || die "не найден релиз Sudoku UI для linux/${ARCH}"
PANEL_SHA_URL="$(release_asset "$REPO" "sudoku-ui-linux-${ARCH}.sha256")"
curl -fsSL "$PANEL_URL" -o "$TMP/sudoku-ui"
if [[ -n "$PANEL_SHA_URL" ]]; then
  curl -fsSL "$PANEL_SHA_URL" -o "$TMP/sudoku-ui.sha256"
  (cd "$TMP"; sed -i "s#sudoku-ui-linux-${ARCH}#sudoku-ui#" sudoku-ui.sha256; sha256sum -c sudoku-ui.sha256 >/dev/null) || die "контрольная сумма Sudoku UI не совпала"
fi
chmod 755 "$TMP/sudoku-ui"
PANEL_VERSION="$("$TMP/sudoku-ui" --version)" || die "скачанный Sudoku UI не запускается"
install -m 755 "$TMP/sudoku-ui" "$BIN"

INSTALL_CORE=1
if [[ -e "$CORE_BIN" || -e /etc/sudoku || -e /etc/systemd/system/sudoku.service ]]; then
  echo; echo "Обнаружена существующая установка Sudoku."
  echo "1 — использовать существующий Sudoku Core"
  echo "2 — заменить бинарник, сохранив резервную копию"
  echo "3 — отменить установку"
  read -r -p "Выберите [1/2/3]: " choice
  case "$choice" in 1) INSTALL_CORE=0;; 2) [[ -f "$CORE_BIN" ]] && cp -a "$CORE_BIN" "${CORE_BIN}.before-sudoku-ui";; *) exit 0;; esac
fi
if [[ $INSTALL_CORE -eq 1 ]]; then
  CORE_URL="$(release_asset "$CORE_REPO" "sudoku-linux-${ARCH}.tar.gz")"
  [[ -n "$CORE_URL" ]] || die "не найден релиз Sudoku Core для linux/${ARCH}"
  curl -fsSL "$CORE_URL" -o "$TMP/sudoku.tar.gz"
  tar -xzf "$TMP/sudoku.tar.gz" -C "$TMP" sudoku; chmod 755 "$TMP/sudoku"
  "$TMP/sudoku" -keygen | grep -q 'Master Public Key:' || die "Sudoku Core не прошёл keygen"
  install -m 755 "$TMP/sudoku" "$CORE_BIN"
fi

mkdir -p /etc/sudoku-ui /etc/sudoku /var/lib/sudoku-ui /var/backups/sudoku-ui
chmod 700 /etc/sudoku-ui /etc/sudoku
cat >/etc/systemd/system/sudoku-ui.service <<'UNIT'
[Unit]
Description=Sudoku UI
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
cat >/etc/systemd/system/sudoku.service <<'UNIT'
[Unit]
Description=Sudoku Server
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart=/usr/local/bin/sudoku -c /etc/sudoku/config.json
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576
[Install]
WantedBy=multi-user.target
UNIT

PORT=""; for _ in $(seq 1 200); do C=$((20000 + RANDOM % 30000)); if ! ss -lnt "sport = :$C" 2>/dev/null | grep -q LISTEN; then PORT="$C"; break; fi; done
[[ -n "$PORT" ]] || die "не удалось подобрать свободный порт панели"
IP="$(curl -4fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
[[ "$IP" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "не удалось определить публичный IPv4-адрес"
DOMAIN="${IP//./-}.sslip.io"
USERNAME="admin-$(od -An -N3 -tx1 /dev/urandom | tr -d ' \n')"
PASSWORD="$(od -An -N12 -tx1 /dev/urandom | tr -d ' \n')"
PUBLIC_PATH="$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')"
PANEL_SUFFIX="/"
if [[ "$PANEL_VERSION" == v0.1.* || "$PANEL_VERSION" == 0.1.* ]]; then
  "$BIN" --init --username "$USERNAME" --password "$PASSWORD" --listen "127.0.0.1:$PORT" --repo "$REPO"
else
  "$BIN" --init --username "$USERNAME" --password "$PASSWORD" --listen "127.0.0.1:$PORT" --repo "$REPO" --path "$PUBLIC_PATH"
  PANEL_SUFFIX="/${PUBLIC_PATH}/"
fi
systemctl daemon-reload
systemctl enable sudoku-ui >/dev/null
systemctl restart sudoku-ui
PANEL_READY=0
for _ in $(seq 1 30); do
  if curl -fsS --max-time 2 "http://127.0.0.1:${PORT}${PANEL_SUFFIX}" >/dev/null 2>&1; then
    PANEL_READY=1
    break
  fi
  sleep 1
done
if [[ $PANEL_READY -ne 1 ]]; then
  echo "Sudoku UI не запустился. Последние строки журнала:" >&2
  journalctl -u sudoku-ui.service --no-pager -n 30 >&2 || true
  die "панель не отвечает на локальном порту ${PORT}"
fi
if [[ $CADDY_WAS_PRESENT -eq 1 && -s /etc/caddy/Caddyfile ]] && ! grep -q '^# Managed by Sudoku UI$' /etc/caddy/Caddyfile; then
  die "Caddy уже настроен другим приложением; автоматическая HTTPS-настройка не будет перезаписана"
fi
cat >/etc/caddy/Caddyfile <<CADDY
# Managed by Sudoku UI
${DOMAIN} {
  encode zstd gzip
  reverse_proxy 127.0.0.1:${PORT}
}
CADDY
caddy validate --config /etc/caddy/Caddyfile >/dev/null || die "ошибка конфигурации HTTPS"
systemctl enable caddy >/dev/null
systemctl restart caddy
if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q '^Status: active'; then
  ufw allow 80/tcp >/dev/null
  ufw allow 443/tcp >/dev/null
fi
HTTPS_READY=0
for _ in $(seq 1 60); do
  if curl -fsS --max-time 3 --resolve "${DOMAIN}:443:127.0.0.1" "https://${DOMAIN}${PANEL_SUFFIX}" >/dev/null 2>&1; then
    HTTPS_READY=1
    break
  fi
  sleep 1
done
if [[ $HTTPS_READY -ne 1 ]]; then
  echo "HTTPS не запустился. Последние строки журнала:" >&2
  journalctl -u caddy.service --no-pager -n 40 >&2 || true
  die "не удалось получить HTTPS-сертификат; внешние порты 80 и 443 должны быть открыты"
fi
echo; echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; echo
echo "Sudoku UI успешно установлен"; echo; echo "Панель:"; echo "https://${DOMAIN}${PANEL_SUFFIX}"
echo; echo "Логин:"; echo "$USERNAME"; echo; echo "Пароль:"; echo "$PASSWORD"
echo; echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
