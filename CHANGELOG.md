# Changelog

## 0.1.1

A security update. Please update; panels with the default "urgent updates" setting install it on their own.

**Fixed**
- A user who could add servers could register another server's folder under a different spelling (a trailing dot, `\?\`, a network path, an 8.3 short name) and reach that server's files and settings. Folders are now compared by identity, can't sit inside or around another server's folder or the panel's data, and adding an existing folder needs every permission, because the panel runs the program it finds there.
- Sign-in limits could be passed with parallel requests or by rotating IPv6 addresses, and a burst of requests could use gigabytes of memory. Attempts are now counted before the password is checked, IPv6 addresses are limited per /64, each username has its own limit, at most four password hashes run at once, and setup locks after 50 wrong PINs.
- A user who could edit configuration could read a secret variable by un-marking it. Showing a secret now needs its value typed again, and variables that change how the server loads code (`PATH`, `LD_*`, `NODE_OPTIONS` and similar) can't be set.
- Users with access to some servers could see and change Discord alerts and status messages for other servers.
- Windows junctions inside a resource were not treated as links, so extraction could write outside it. "Keep" rules now ignore case on Windows, short (8.3) names in archives are refused, and archives are limited to 20,000 files.
- Downloads are limited to MafiaHub's and Latch's own storage and GitHub's release hosts, not any storage account. A Latch package must name the script and version that was asked for.
- A two-factor code could be used more than once within its 90 seconds.
- A failed game update at a scheduled restart left the server stopped; it now starts the previous version again. Version numbers from Latch are accepted only as plain `x.y.z`.
- `server.json` is written readable by the panel's account only, values named like a token or secret are masked in the editor, and clearing the server token also removes it from the backup.
- Signed download links and local folder paths no longer end up in the console log or Discord messages.

**Added**
- `--trusted-proxy`: behind a reverse proxy, sign-in limits and the activity log see visitors' real addresses, and the session cookie is marked secure.

**Changed**
- SECURITY.md and the permission descriptions now say plainly that game servers run as the panel's account, so managing resources, editing configuration and adding servers amount to running code on the machine.

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
