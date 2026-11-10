# Contributing

Thanks for helping. Bug reports, fixes and small features are welcome.

## Reporting

- **Bugs:** open an issue with your OS, the LatchAdmin version (`latchadmin --version`), the game and what you expected. Console lines help; remove tokens and keys first.
- **Security problems:** don't open an issue. See [SECURITY.md](SECURITY.md).

## Working on the code

```sh
cd ui && npm ci && npm run build && cd ..
go build -o latchadmin ./cmd/latchadmin
./latchadmin --data ./dev-data --host 127.0.0.1
```

For the interface, run `npm run dev` in `ui/` too: it serves on port 5190 with hot reload and sends `/api` to the panel on `127.0.0.1:40120` (or `LATCHADMIN_URL`).

Before sending a pull request:

```sh
gofmt -l .        # prints nothing
go vet ./...
go test ./...
cd ui && npm run build
```

## Ground rules

- **The network allow-list stays small.** A new outbound host in [`internal/netx/netx.go`](internal/netx/netx.go) needs a reason in the pull request, and a line in SECURITY.md.
- **Nothing a remote server answers is run.** Version numbers and flags from Latch or MafiaHub are data; downloads come only from the allow-listed hosts, and panel updates only with a valid signature.
- **Telemetry doesn't grow quietly.** A new field sent to Latch goes into [docs/TELEMETRY.md](docs/TELEMETRY.md) in the same pull request.
- **Every new action has a permission.** Add it to [`internal/perms`](internal/perms/perms.go) and check it in the route.
- **Text comes in English and Turkish.** Interface strings live in `ui/src/locales/en.js` and `tr.js` with the same keys; Discord texts in `internal/notify`.

By contributing you agree that your work is released under the [MIT license](LICENSE).
