# LatchAdmin

**A web panel for MafiaHub game servers.** Run one file, open your browser, and install, run, update and watch your Arcane MP, Kingdoms Connected or Mafia 2 Online servers from there. Made by [Latch](https://heylatch.app), open source under the MIT license.

## What it does

**Servers**
- Install a server from the official MafiaHub release in seconds (only the server files are downloaded, not the game client), or add a server folder you already have without changing any file in it.
- Start, stop and restart. Crash restarts with back-off and daily scheduled restarts.
- Live console with command suggestions taken from your server's own `help`, plus clear and download.
- Resources: list, enable and disable, upload a .zip, restart one while the server runs.
- Configuration: environment variables (secrets stored encrypted), `server.json`, resource `data/*.json` and the MafiaHub server token.

**HMP Foundations (Arcane MP)**
- Install [HMP Foundations](https://github.com/arcane-mp/foundations), the open-source gameplay framework, with a new server or later from its Updates tab, and update all 30 resources together. Old folders go to `backups/`; your `data/` settings are never overwritten, and resources that clash with Foundations are pointed out.
- Set up its database like txAdmin does: give an administrator login of MySQL or MariaDB (XAMPP's `root` works) and LatchAdmin creates the database and a user that can only use it, or enter a database you already have. The administrator login is used once and not saved.

**Updates**
- Game updates in one click, with a backup first and one-click restore (the last 5 backups are kept).
- Install new game versions by hand or at the next scheduled restart.
- LatchAdmin updates itself from signed GitHub releases.

**Latch scripts**
- Link your [Latch](https://heylatch.app) account and install the scripts you bought on each server.
- Keep them up to date by hand, at the next restart, or right away.

**Watching**
- Statistics for every server and the machine: players, CPU, memory, uptime and crashes, from 1 hour to 7 days.
- Discord alerts for your staff: crashes, restarts, game and panel updates, script updates, player records.
- A public status message for your players: which servers are online, how many are playing and how to connect, kept up to date in one Discord message.
- Activity log of who did what.

**People**
- Users with 16 permissions in 6 groups (txAdmin style), access to all servers or chosen ones, generated passwords and two-factor sign-in.

## Supported games

| Game | Install from LatchAdmin | Add an existing server |
| --- | --- | --- |
| Arcane MP | Yes | Yes |
| Kingdoms Connected | Yes | Yes |
| Mafia 2 Online | Yes | Yes |
| Fallout MP | When MafiaHub publishes a release | Yes (pick the server binary) |
| Mafia MP | When MafiaHub publishes a release | Yes (pick the server binary) |
| Vanta MP | When MafiaHub publishes a release | Yes (pick the server binary) |

## Quick start

### Windows

1. Download `latchadmin-windows-amd64.exe` from [Releases](https://github.com/heylatch/latchadmin/releases/latest).
2. Put it in its own folder (for example `C:\LatchAdmin`) and double-click it. Windows SmartScreen may say the app is unrecognised: choose **More info → Run anyway**. The exe isn't code-signed yet; every release lists the exe's SHA-256 in `SHA256SUMS` so you can check it.
3. A window opens with the panel's address and a first-time setup box. Your browser opens the setup page with the **setup PIN** already filled in; if it doesn't, Ctrl+click the link in the box (or open `http://localhost:40120` and type the PIN).
4. Create the owner account.

Keep the window open while the panel runs. Closing it stops your game servers cleanly; they start again with the panel.

Game servers need the [Microsoft Visual C++ Redistributable](https://aka.ms/vs/17/release/vc_redist.x64.exe) (2015–2022, 14.40 or newer). Without it a server exits right after starting.

### Linux

```sh
sudo useradd --system --create-home --home-dir /opt/latchadmin latchadmin
sudo -u latchadmin curl -L -o /opt/latchadmin/latchadmin https://github.com/heylatch/latchadmin/releases/latest/download/latchadmin-linux-amd64
sudo chmod +x /opt/latchadmin/latchadmin
```

Use `latchadmin-linux-arm64` on ARM machines. Then run it as a service:

```ini
# /etc/systemd/system/latchadmin.service
[Unit]
Description=LatchAdmin
After=network-online.target
Wants=network-online.target

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

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now latchadmin
journalctl -u latchadmin | grep -A4 'First-time setup'
```

Don't run it as root. The service user must be able to write to `/opt/latchadmin` (data and self-updates).

### Reaching the panel from another computer

The panel listens on port **40120** on every network interface. Open that TCP port in the firewall to reach it from home, and use HTTPS before signing in over the internet:

- pass `--tls-cert` and `--tls-key` with your certificate, or
- put it behind a reverse proxy that handles HTTPS (Caddy, nginx) and pass `--trusted-proxy 127.0.0.1`, or
- keep it private with `--host 127.0.0.1` and reach it through an SSH tunnel: `ssh -L 40120:127.0.0.1:40120 your-server`.

## Adding a server

**Add server** offers two ways:

- **Install a new server:** choose the game, a name, an empty folder and the port. The panel downloads the latest release from MafiaHub, sets up `resources/`, `data/` and `logs/`, and writes the port into `server.json`.
- **Use an existing server:** choose the folder with your server binary. LatchAdmin finds the game and port, and copies the variables your start script used (`server.env`, `.env`, `latch.env`). Nothing in the folder changes. From then on, start the server from the panel, not from your own script.

Players connect on the game port over **UDP and TCP**; open both in the firewall.

### MafiaHub server token

To show your server in the in-game server browser, register it on [mafiahub.dev](https://mafiahub.dev/) (name, IP, port, location, language and the mod) and paste the token you get there. You can do it while adding the server or later under **Configuration → MafiaHub server token**. The panel stores it in `server.json` and never shows it again. Without a token the server still runs, but players can only join with its address.

## Latch scripts

1. Open the **Latch** page and choose **Link with Latch**. The panel shows a code; approve it on heylatch.app. The panel receives a key that can only list and download the scripts in your Library. You can revoke it any time on heylatch.app under Settings > API keys.
2. Add your license key as `LATCH_LICENSE` under **Configuration → Environment variables** (mark it secret). Your keys are at [heylatch.app/app/keys](https://heylatch.app/app/keys).
3. On each server's **Latch scripts** tab, install scripts and choose how they update: when you click Update, at the next restart, or right away.

## Users and permissions

The first account has all permissions. Under **Users** you can add people with exactly the permissions they need, grouped like this:

| Group | Permissions |
| --- | --- |
| Servers | start/stop/restart, server settings, add servers, remove servers |
| Console | see the console, send commands |
| Resources | manage resources |
| Configuration | see configuration, edit configuration |
| Updates | game updates, Latch scripts |
| Panel | manage users, panel settings, update the panel, Latch account, activity log |

Each user also gets access to all servers or only chosen ones; other servers don't appear for them at all. Nobody can give a permission or a server they don't have themselves.

Game servers run as the same system account as the panel, so *manage resources*, *edit configuration* and *add servers* let someone run code on the machine. Give them only to people you'd trust with it. Adding an existing server folder needs every permission.

## Discord

Both kinds of message use a Discord webhook (Discord: Server settings → Integrations → Webhooks → New webhook → Copy webhook URL) and can be in English or Turkish. Webhook URLs are stored encrypted.

**Staff alerts.** Under **Notifications → Staff alerts**, pick the events and the servers a channel hears about. Messages carry the event, the server's name and details like versions or player counts, never config, keys or console output. Set **Settings → Public address** to get an "Open in panel" button on each message.

**Public server status.** Under **Notifications → Public server status**, add a message for a channel your players can see. Choose the servers, the address players connect to for each one, a short "how to join" text and up to 3 link buttons (Discord invite, website, rules); the dialog shows a preview. The panel posts one message and edits it in place when a server goes up or down or the player count changes (at most once a minute, and at least every 5 minutes so "Updated" stays current). When the panel shuts down, the servers show as offline. It shows only what you chose: never the panel's address, config or console. "Post again at the bottom" moves the message under newer ones in the channel.

## Updates

- **Game:** the server's **Updates** tab shows the installed and latest MafiaHub versions. Updating stops the server, backs up the runtime, `server.json` and `data/`, installs the new version and starts it again; if anything fails the backup is put back.
- **Panel:** under **Settings**, choose whether LatchAdmin installs urgent updates by itself (default), every update, or none. Updates install only while no server is running or at a scheduled restart, so players aren't kicked by a panel update.
- **Urgent releases:** the Latch team can mark a panel or game release urgent; the panel then shows a red banner until you update.

## Options

| Flag | Default | |
| --- | --- | --- |
| `--port` | `40120` | Web panel port |
| `--host` | `0.0.0.0` | `127.0.0.1` makes the panel reachable from this machine only |
| `--data` | `latchadmin-data` next to the binary | Settings, users, logs, statistics |
| `--tls-cert`, `--tls-key` | | Serve HTTPS with your certificate |
| `--trusted-proxy` | | Address or CIDR range of your reverse proxy, so sign-in limits and the audit log see visitors' real addresses |
| `--version` | | Print the version and exit |

Everything the panel keeps is in the data folder: `latchadmin.json` (settings and users, secrets encrypted), `secret.key` (protected with Windows DPAPI or `0600`), `audit.jsonl`, `metrics.json.gz` and the daily console logs. Back it up together with your server folders.

## Troubleshooting

- **A server exits right after starting on Windows:** install the Visual C++ Redistributable linked above.
- **"Server will not be announced to masterlist" in the console:** the MafiaHub server token is missing.
- **Players can't connect:** open the game port for both UDP and TCP, and check that the port in `server.json` matches the one you registered on MafiaHub.
- **Forgot your password:** another user with the "manage users" permission can set a new one. If you're the only user, stop the panel, move `latchadmin-data` away and set up again (your servers' files are untouched; add them back with "Use an existing server").

## Trust

LatchAdmin is open source so you can check exactly what it does:

- It only connects to `api.heylatch.app`, `api.mafiahub.dev`, GitHub and their download hosts, and the Discord webhooks you add. The list is in [`internal/netx/netx.go`](internal/netx/netx.go) and is enforced on every request and redirect.
- **Latch can't reach your panel or your servers.** The panel calls Latch, never the other way round, and no command comes from Latch. Latch scripts are code and run on your server, but only the versions you install (or chose to update automatically).
- Panel updates install only when their Ed25519 signature checks out against the key built into the binary.
- What the panel tells Latch (only while an account is linked) is in [docs/TELEMETRY.md](docs/TELEMETRY.md); the panel shows the exact report on its Latch page.

See [SECURITY.md](SECURITY.md) for the full model and how to report a vulnerability.

## Building from source

Needs Go (the version in `go.mod`) and Node.js 22.

```sh
cd ui && npm ci && npm run build && cd ..
go build -o latchadmin ./cmd/latchadmin
go test ./...
```

`npm run dev` in `ui/` serves the interface with hot reload and sends `/api` to a panel on `127.0.0.1:40120` (set `LATCHADMIN_URL` to change it). See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT. See [LICENSE](LICENSE).
