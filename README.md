# Sudoku UI

Minimal web panel for managing the official [SUDOKU-ASCII/sudoku](https://github.com/SUDOKU-ASCII/sudoku) server on a single VPS.

The product model is deliberately simple: **one VPS = one Sudoku UI panel**. The panel manages Sudoku connections running on that VPS and generates client access keys for them.

## v0.1 features

- One-screen Russian UI.
- CPU, RAM, disk and network traffic monitoring.
- Create/delete/restart Sudoku connections on separate TCP ports.
- Official Sudoku key generation (`-keygen`, `-keygen -more`).
- Client `sudoku://` link, QR code and manual connection parameters.
- Logs in a modal window.
- Manual update of the official Sudoku core from GitHub Releases.
- Manual update of Sudoku UI from its own GitHub Releases.
- One-click installer that creates a random panel port, username and password.
- systemd template service (`sudoku@<id>.service`) for panel-managed connections.

## One-click install

Run as root on a supported Linux VPS:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/sanderstripa/sudoku-ui/main/install.sh)
```

When using a non-root shell, run `sudo -i` first and then execute the command above.

## Local development

```bash
go build -o sudoku-ui .
```

The production program expects `/etc/sudoku-ui/config.json`; the installer creates it automatically.

## Server-side layout

```text
/usr/local/bin/sudoku-ui
/usr/local/bin/sudoku
/etc/sudoku-ui/config.json
/etc/sudoku-ui/state.json
/etc/sudoku-ui/connections/<id>.json
/etc/systemd/system/sudoku-ui.service
/etc/systemd/system/sudoku@.service
```

## Key model

Sudoku uses a Master Public Key on the server. The panel stores the corresponding Master Private Key locally with mode `0600` so it can generate additional Split Private Keys for client devices using the upstream CLI. Master private keys are never returned by the HTTP API.

> Important: current upstream Sudoku does not provide a server-side per-client allow/revoke list like Xray UUID clients. Removing a key from this panel removes it from panel storage, but does not cryptographically revoke a copy that has already been exported. True per-key revocation requires rotating the connection's master key.

## Security notes

- Admin passwords are stored as a salted iterated SHA-256 hash, not plaintext.
- Session cookies are HttpOnly and SameSite=Strict.
- Panel state and private key material are written with `0600` permissions.
- For internet-facing production use, put the panel behind HTTPS (Caddy/Nginx) or restrict its port at the firewall.

## Upstream

Sudoku UI is not part of the SUDOKU-ASCII project. Sudoku core is installed from official SUDOKU-ASCII GitHub releases.
