# LatchAdmin

A web panel for running MafiaHub game servers: Arcane MP, Kingdoms Connected and Mafia 2 Online. One file to run, then everything from your browser:

- **Install a server** from the official MafiaHub releases, or **use a server folder you already have**. Nothing in it is changed.
- **Run it:** start, stop and restart; live console with commands; crash restarts; daily scheduled restarts.
- **Resources:** list, enable and disable, upload a .zip, restart one while the server runs.
- **Configuration:** environment variables (secrets stored encrypted), `server.json` and the resources' `data/*.json`.
- **Game updates:** one click, with a backup first and a one-click restore.
- **Latch scripts:** link your [Latch](https://heylatch.app) account, then install and update the scripts you've bought, by hand or automatically.
- **Self-update:** LatchAdmin updates itself from signed GitHub releases.
- **Users:** owner, admin and viewer roles, two-factor sign-in and an activity log.

Runs on Windows and Linux (x64, and arm64 on Linux).

## Install

### Windows

1. Download `latchadmin-windows-amd64.exe` from [Releases](https://github.com/heylatch/latchadmin/releases).
2. Put it in its own folder and run it. Windows may warn that the file is unrecognised: choose **More info → Run anyway**. The binary isn't code-signed yet; you can check it against `SHA256SUMS` on the release page.
3. Open the address it prints (`http://localhost:40120`) and enter the **setup PIN** it shows to create the owner account.

The panel keeps its settings in `latchadmin-data` next to the exe. Game servers run as child processes; if you close the panel, they stop with it, and they start again when the panel starts.

### Linux

```sh
curl -LO https://github.com/heylatch/latchadmin/releases/latest/download/latchadmin-linux-amd64
chmod +x latchadmin-linux-amd64
sudo mv latchadmin-linux-amd64 /opt/latchadmin/latchadmin
```

Run it as a service with a dedicated user, not as root:

```ini
# /etc/systemd/system/latchadmin.service
[Unit]
Description=LatchAdmin
After=network-online.target

[Service]
User=latchadmin
WorkingDirectory=/opt/latchadmin
ExecStart=/opt/latchadmin/latchadmin --data /opt/latchadmin/data
Restart=always
KillMode=mixed
TimeoutStopSec=60

[Install]
WantedBy=multi-user.target
```

`sudo systemctl enable --now latchadmin`, then read the setup PIN with `journalctl -u latchadmin`.

The service user must be able to write to the panel binary's folder for self-updates.

### Options

| Flag | Default | |
| --- | --- | --- |
| `--port` | `40120` | Web panel port |
| `--host` | `0.0.0.0` | `127.0.0.1` makes the panel reachable from this machine only |
| `--data` | `./latchadmin-data` | Settings, users, logs |
| `--tls-cert`, `--tls-key` | | Serve HTTPS with your certificate |

### HTTPS

Out of the box the panel speaks plain HTTP. Before you sign in over the internet, either pass `--tls-cert`/`--tls-key`, or put it behind a reverse proxy (Caddy, nginx) that terminates HTTPS, or reach it through an SSH tunnel and run it with `--host 127.0.0.1`.

## Using an existing server

**Add server → Use an existing server**, then pick the folder that holds the server binary (for example `ArcaneMPServer.exe`). LatchAdmin reads the port from `server.json` and copies variables from `server.env`, `.env` and `latch.env` into the panel; values that look secret are stored encrypted. From then on, start the server from LatchAdmin instead of your own script.

## Latch scripts

On the **Latch** page, choose **Link with Latch** and approve the code on heylatch.app. The panel receives an account API key that can only list and download the scripts in your Library; you can revoke it on heylatch.app under Settings > API keys. Each server's **Latch scripts** tab installs and updates scripts. Add your license key as the `LATCH_LICENSE` variable under **Configuration**.

## Trust

LatchAdmin is open source so you can check exactly what it does. In short:

- It only connects to `api.heylatch.app`, `api.mafiahub.dev`, `github.com`/`api.github.com` and their download hosts. The list is in [`internal/netx/netx.go`](internal/netx/netx.go) and is enforced on every request and redirect.
- **Latch can't reach your panel or your servers.** The panel calls Latch, never the other way round, and nothing Latch answers is run.
- Updates to LatchAdmin are installed only when their Ed25519 signature checks out against the key built into the binary.
- What the panel reports to Latch (only while an account is linked) is listed in [docs/TELEMETRY.md](docs/TELEMETRY.md), and the panel shows the exact report on its Latch page.

See [SECURITY.md](SECURITY.md) for the full model and how to report a vulnerability.

## Building from source

Needs Go (see `go.mod`) and Node.js 22.

```sh
cd ui && npm ci && npm run build && cd ..
go build -o latchadmin ./cmd/latchadmin
go test ./...
```

`npm run dev` in `ui/` serves the interface with hot reload and sends `/api` to a panel on `127.0.0.1:40120` (set `LATCHADMIN_URL` to change it).

## License

MIT. See [LICENSE](LICENSE).
