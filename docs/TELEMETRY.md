# What LatchAdmin sends to Latch

## Without a linked account

Nothing that identifies you or your servers. Every 30 minutes the panel reads the public version list from `GET https://api.heylatch.app/public/v1/latchadmin/manifest`. The request carries no key and no id. The answer holds the latest LatchAdmin and game versions and which of them the Latch team marked urgent.

Turn off **Settings → Read versions from Latch** and the panel doesn't contact Latch at all: it reads the latest game versions from `api.mafiahub.dev` and the latest LatchAdmin version from the GitHub API instead.

## With a linked account

Every 15 minutes the panel sends `POST https://api.heylatch.app/public/v1/me/panel` with your account API key. This is the whole payload:

```json
{
  "instanceId": "random id made when the panel was set up",
  "name": "the panel name from Settings",
  "panelVersion": "0.1.0",
  "os": "windows",
  "servers": [
    {
      "game": "arcane-mp",
      "gameVersion": "1.12.2",
      "state": "running",
      "scripts": [{ "scriptId": "id of a Latch script installed on this server", "version": "1.4.0" }]
    }
  ]
}
```

It's built in [`internal/app/latch.go`](../internal/app/latch.go) (`BuildReport`) and the panel shows the exact next report on its **Latch** page.

Latch keeps the last report per panel for 30 days so you and the Latch team can see which versions run where. It doesn't store your IP with it. Only scripts that are in your Latch Library are kept; other ids are dropped.

## Never sent

- IP addresses, player names or counts, chat
- Console output, logs, `server.json`, `data/` files, environment variables or license keys
- Resources that aren't Latch scripts, file names or folder paths

## Downloads

Installing or updating a Latch script asks Latch for a short-lived download link to your own copy of the package, the same file the Latch dashboard gives you. That download is counted as your downloaded version, as on the dashboard.
