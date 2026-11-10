# Changelog

## 0.1.0

The first release of LatchAdmin, a web panel for MafiaHub game servers on Windows and Linux.

**Servers**
- Install Arcane MP, Kingdoms Connected and Mafia 2 Online servers from the official MafiaHub releases, or add a server folder you already have (Fallout MP, Mafia MP and Vanta MP too, by picking the server binary).
- Start, stop and restart, crash restarts with back-off, daily scheduled restarts.
- Live console with command suggestions from the server's `help`, clear and download.
- Resources: enable, disable, upload, restart while running.
- Configuration: environment variables (secrets encrypted), `server.json`, resource data files, MafiaHub server token.

**Updates**
- Game updates with a backup first, automatic roll back on failure and one-click restore.
- Panel self-update from signed GitHub releases: urgent only (default), all, or off; installed only while no server runs or at a scheduled restart.

**Latch**
- Link a Latch account with a code, install and update the scripts in your Library per server, by hand, at the next restart or right away.

**Watching**
- Statistics for every server and the machine: players, CPU, memory, uptime and crashes, 1 hour to 7 days.
- Discord staff alerts (crashes, restarts, updates, player records) and a public status message players can see, edited in place.
- Activity log.

**People**
- Users with 16 permissions in 6 groups, access to all or chosen servers, generated passwords, two-factor sign-in.

Download `latchadmin-windows-amd64.exe`, `latchadmin-linux-amd64` or `latchadmin-linux-arm64` below. `SHA256SUMS` lists every file's hash and is signed (`SHA256SUMS.sig`, Ed25519) with the key built into the panel.
