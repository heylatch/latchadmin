# Security

## Reporting a vulnerability

Please report it privately through GitHub: **Security → Report a vulnerability** on this repository. Don't open a public issue. We'll answer within a few days and credit you in the release notes if you'd like.

## Security model

LatchAdmin runs on a game server machine with the rights of the account that starts it, and it starts game servers. It is built so that neither the network nor Latch can turn it into a way into that machine.

### Who can control the panel

- **Setup:** until an owner exists, the setup page needs a 6-digit PIN printed only to the panel's console. Five wrong PINs replace it.
- **Sign-in:** argon2id password hashes; optional TOTP two-factor codes; 10 failed sign-ins per IP per 15 minutes, then a wait.
- **Sessions:** random 256-bit tokens in an `HttpOnly`, `SameSite=Strict` cookie. Only their SHA-256 is stored. Changing a password signs out other sessions.
- **Request forgery:** every state-changing request must carry an `X-LatchAdmin` header, which a browser only sends from the panel's own pages (the panel has no CORS), and its `Origin`, if present, must match the panel's host.
- **Permissions:** each user has a set of permissions (16, in six groups, or "all") and access to all servers or chosen ones. A server the user can't access answers as if it didn't exist. Nobody can grant a permission or server they don't hold, or change a user who holds more than they do.
- **Headers:** a strict Content-Security-Policy (no inline scripts), `X-Frame-Options: DENY`, `nosniff`, `no-referrer`.

Serve the panel over HTTPS before signing in across the internet (see the README).

### What the panel connects to

All outgoing traffic goes through [`internal/netx`](internal/netx/netx.go), which allows HTTPS to these hosts only, on every request and every redirect:

| Host | Why |
| --- | --- |
| `api.heylatch.app` | Account link, Library, version manifest, panel report |
| `api.mafiahub.dev` | Game server releases |
| `*.r2.cloudflarestorage.com` | Release zips and Latch packages (signed, short-lived links) |
| `api.github.com`, `github.com`, `*.githubusercontent.com` | LatchAdmin releases |

The only other connection is a health check to `127.0.0.1:<game port>`.

### Latch has no way in

- The panel makes every request. There is no listening socket, websocket, command queue or remote setting through which Latch, or anyone controlling Latch, can reach the panel or a server.
- Answers from Latch are data: version numbers, urgent flags, and download links that still have to pass the allow-list above. Nothing in them is executed, and no file path comes from them.
- The Latch admin page can only read what panels report and mark a release urgent. An urgent mark shows a banner; with the default "urgent updates" policy it lets the panel install a **signed** LatchAdmin release. It never updates a game server without the owner.

### Updates of LatchAdmin itself

Releases are built and signed in GitHub Actions ([`.github/workflows/release.yml`](.github/workflows/release.yml)). `SHA256SUMS` starts with `# latchadmin <version>` and is signed with an Ed25519 key kept only as a GitHub Actions secret. The panel downloads the binary from `github.com`, checks the signature against the public key built into it ([`internal/selfupdate`](internal/selfupdate/selfupdate.go)), checks that the signed file names the version it asked for, and checks the binary's hash. It refuses anything else, and never installs an older version.

### Files

- Zip archives (game releases, resources, Latch packages) are extracted with checks against path traversal, absolute paths, Windows device names and trailing dots, links, and size limits ([`internal/archive`](internal/archive/archive.go)).
- LatchAdmin never writes through a symbolic link or junction into a resource, and never overwrites a resource folder that isn't a Latch package.
- Removing a resource moves it to `backups/removed-resources`; game updates make a backup first.

### Secrets at rest

The Latch account key, environment values marked secret and TOTP secrets are encrypted with AES-256-GCM. The key lives in `latchadmin-data/secret.key`, protected with Windows DPAPI for the account that runs the panel, or with `0600` permissions on Linux. Secret values are never sent back to the browser.

### Game servers

Servers get a minimal environment (system paths plus the variables you set), not the panel's full environment. On Windows they run in a job object, and on Linux they get a parent-death signal, so they never outlive the panel as orphans.
