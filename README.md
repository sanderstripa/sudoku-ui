<p align="center">
  <img src="web/icon.png" width="128" height="128" alt="Sudoku UI">
</p>

<h1 align="center">Sudoku UI</h1>

<p align="center">
  A compact web panel for managing a Sudoku server, access keys, sessions, traffic, and updates.
</p>

<p align="center">
  <a href="https://github.com/sanderstripa/sudoku-ui/releases/latest"><img src="https://img.shields.io/github/v/release/sanderstripa/sudoku-ui?filter=v%2A&display_name=tag&label=Sudoku%20UI" alt="Latest Sudoku UI release"></a>
  <a href="https://github.com/sanderstripa/sudoku-ui/actions/workflows/ci.yml"><img src="https://github.com/sanderstripa/sudoku-ui/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/sanderstripa/sudoku-ui" alt="MIT License"></a>
</p>

Sudoku UI follows a simple model: **one VPS — one panel — one Sudoku server**. The panel installs and manages its own [compatible Sudoku Core](#compatible-sudoku-core), creates separate keys for devices, and shows the server state on a single screen.

The project is based on the official [SUDOKU-ASCII/sudoku](https://github.com/SUDOKU-ASCII/sudoku), but is not part of that project.

## Quick installation

Supported systems are **Ubuntu and Debian** on **amd64** and **arm64**. Run as `root`:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/sanderstripa/sudoku-ui/main/install.sh)
```

This is the permanent installation URL. The installer automatically downloads the latest stable Sudoku UI release and the latest compatible Sudoku Core release from this repository.

The installer will:

1. ask you to choose Russian or English;
2. install verified panel and compatible Core binaries;
3. create and start the `systemd` services;
4. obtain a short-lived Let's Encrypt HTTPS certificate directly for the public IPv4 address;
5. generate a random panel port, hidden path, username, and password;
6. print the complete HTTPS login URL.

For certificate requests, inbound **TCP port 80** must be free and reachable from the internet. You must also open the panel port selected by the installer. A custom domain, DNS configuration, and Caddy are not required. A dedicated `systemd` timer checks the certificate every six hours, renews it before expiry, and reloads the panel automatically.

If Sudoku UI or Sudoku Core is already present, the installer offers to keep the existing Core, replace it with or without a backup, perform a clean reinstall, or remove the managed installation. A normal repeated installation preserves the panel configuration and credentials.

## Panel features

### Keys and devices

- a separate private key for every device;
- a stable `user_hash` without exposing private keys through the general state API;
- `enabled`, `disabled`, and irreversible `revoked` states;
- disabling a key rejects new connections and closes its active sessions;
- deleting a key permanently revokes it so old copies can never connect again;
- QR code, `sudoku://` link, and parameters for manual configuration;
- migration of existing keys without reissuing them or changing old links.

### Sessions and statistics

- Online/Offline state and last activity for each key;
- all active external IP addresses with start time and duration;
- multiple transport connections from the same IP are grouped into one clear connection;
- separate RX, TX, and total traffic accounting for every key;
- a shared footer with CPU, RAM, disk, current speed, total traffic, and Core uptime.

### Server management

- AEAD, four table modes, Padding, Pure Downlink, HTTP Mask, and Multiplex;
- safe defaults and built-in help for the settings;
- panel and Core logs in one window;
- independent Sudoku UI and Sudoku Core versions and update buttons;
- compatibility checks, checksums, and automatic rollback after a failed update;
- Core updates only from compatible `core-v*` releases in this repository.

### Interface

- Russian and English languages;
- light and dark themes;
- responsive layout for desktop, tablet, and mobile screens;
- built-in classic Sudoku game with four difficulty levels, notes, hints, mistake limits, timer, and browser-side game saving.

## After installation

1. Open the HTTPS URL printed by the installer.
2. Sign in with the generated username and password.
3. Create the Sudoku server connection.
4. Issue a separate key for each device.
5. Scan the QR code or copy the connection link or parameters.

The panel does not create a protocol connection automatically. You confirm the port and protocol settings in the interface.

## Updates

The panel and Core update independently from the top bar:

- **Panel** installs the latest stable `v*` release;
- **Sudoku Core** installs only a compatible `core-v*` release.

If the versions are incompatible, the update is stopped and the panel explains which component must be updated first. Configuration, keys, subscriptions, links, and settings are preserved during normal updates.

Releases: [latest Sudoku UI](https://github.com/sanderstripa/sudoku-ui/releases/latest) · [all panel and Core releases](https://github.com/sanderstripa/sudoku-ui/releases)

## Compatible Sudoku Core

The official Core does not provide the per-user management data required by the panel. Sudoku UI therefore uses a compatible build based on a pinned commit from the official project and applies [`core-patches/access-control.patch`](core-patches/access-control.patch).

The compatible build adds:

- local API at `/run/sudoku/core.sock`;
- `enabled`, `disabled`, and `revoked` policy enforcement;
- immediate session termination for disabled or revoked keys;
- active sessions and external IP addresses by `user_hash`;
- per-key RX/TX accounting;
- persistent revoked-key storage.

Core releases are built by GitHub Actions from the official source plus the published patch and include checksums. The panel never downloads Core directly from upstream.

## Security and data

- private keys and configuration are stored locally with restricted permissions;
- panel passwords are protected with `bcrypt`;
- login sessions use `HttpOnly`/`SameSite` cookies;
- state-changing requests are protected by CSRF tokens;
- login attempts are rate limited;
- the interface is served over HTTPS on a random port and hidden path;
- built-in game data stays in the browser's `localStorage`.

Main server files:

```text
/usr/local/bin/sudoku-ui
/usr/local/bin/sudoku
/etc/sudoku-ui/config.json
/etc/sudoku-ui/state.json
/etc/sudoku-ui/core-version
/etc/sudoku-ui/tls/
/etc/sudoku/config.json
/etc/systemd/system/sudoku-ui.service
/etc/systemd/system/sudoku.service
/etc/systemd/system/sudoku-ui-cert-renew.timer
/usr/local/sbin/sudoku-ui-renew-cert
/run/sudoku/core.sock
```

## Development

```bash
go test ./...
go vet ./...
go build -o sudoku-ui .
node --check web/app.js
node --check web/game.js
bash -n install.sh
```

The frontend is embedded in the Go binary through `embed.FS`. CI runs the test suite, `go vet`, compilation, installer syntax checks, and JavaScript syntax checks.

## License

[MIT](LICENSE). Sudoku UI is an independent project and is not affiliated with the authors of SUDOKU-ASCII/sudoku.
