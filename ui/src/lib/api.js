// Client for the panel's own API (same origin). Every write carries X-LatchAdmin, which the server requires as
// proof the request came from these pages.

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message)
    this.status = status
    this.code = code
  }
}

let onUnauthorized = () => {}
export const setUnauthorizedHandler = (fn) => (onUnauthorized = fn)

async function request(method, path, body, { raw = false } = {}) {
  const headers = { "X-LatchAdmin": "1" }
  let payload
  if (body instanceof FormData) payload = body
  else if (typeof body === "string") {
    headers["Content-Type"] = "application/json"
    payload = body
  } else if (body !== undefined) {
    headers["Content-Type"] = "application/json"
    payload = JSON.stringify(body)
  }
  const res = await fetch(`/api${path}`, { method, headers, body: payload, credentials: "same-origin" })
  if (raw && res.ok) return res.text()
  const json = await res.json().catch(() => null)
  if (!res.ok) {
    const err = new ApiError(res.status, json?.error?.code ?? "failed", json?.error?.message ?? `Request failed (${res.status})`)
    if (res.status === 401 && err.code === "unauthorized") onUnauthorized()
    throw err
  }
  return json
}

const get = (p, o) => request("GET", p, undefined, o)
const post = (p, b) => request("POST", p, b ?? {})
const put = (p, b) => request("PUT", p, b)
const patch = (p, b) => request("PATCH", p, b)
const del = (p, b) => request("DELETE", p, b ?? {})
const id = encodeURIComponent

export const api = {
  session: () => get("/session"),
  setup: (b) => post("/setup", b),
  login: (b) => post("/login", b),
  logout: () => post("/logout"),
  changePassword: (b) => post("/me/password", b),
  totpSetup: () => post("/me/totp/setup"),
  totpEnable: (code) => post("/me/totp", { Code: code }),
  totpDisable: (password) => del("/me/totp", { Password: password }),

  permissions: () => get("/permissions"),
  webhooks: () => get("/webhooks"),
  createWebhook: (b) => post("/webhooks", b),
  updateWebhook: (w, b) => patch(`/webhooks/${id(w)}`, b),
  deleteWebhook: (w) => del(`/webhooks/${id(w)}`),
  testWebhook: (w) => post(`/webhooks/${id(w)}/test`),
  users: () => get("/users"),
  createUser: (b) => post("/users", b),
  updateUser: (u, b) => patch(`/users/${id(u)}`, b),
  deleteUser: (u) => del(`/users/${id(u)}`),

  panel: () => get("/panel"),
  panelSettings: (b) => patch("/panel/settings", b),
  panelCheck: () => post("/panel/check"),
  panelUpdate: (now) => post("/panel/update", { Now: now }),
  audit: () => get("/audit"),
  browse: (path) => get(`/browse?path=${id(path ?? "")}`),
  stats: (range) => get(`/stats?range=${id(range)}`),
  serverStats: (s, range) => get(`/servers/${id(s)}/stats?range=${id(range)}`),

  games: () => get("/games"),
  servers: () => get("/servers"),
  server: (s) => get(`/servers/${id(s)}`),
  installServer: (b) => post("/servers", b),
  importPreview: (dir) => post("/servers/import/preview", { Dir: dir }),
  importServer: (dir, name, game, exe) => post("/servers/import", { Dir: dir, Name: name, Game: game ?? "", Exe: exe ?? "" }),
  setToken: (s, token) => put(`/servers/${id(s)}/token`, { Token: token }),
  patchServer: (s, b) => patch(`/servers/${id(s)}`, b),
  deleteServer: (s) => del(`/servers/${id(s)}`),
  serverAction: (s, action, body) => post(`/servers/${id(s)}/${action}`, body),
  command: (s, command) => post(`/servers/${id(s)}/command`, { Command: command }),
  logs: (s) => get(`/servers/${id(s)}/logs`),
  resources: (s) => get(`/servers/${id(s)}/resources`),
  uploadResource: (s, file) => {
    const fd = new FormData()
    fd.append("file", file)
    return request("POST", `/servers/${id(s)}/resources`, fd)
  },
  resourceAction: (s, folder, action) => post(`/servers/${id(s)}/resources/${id(folder)}/${action}`),
  removeResource: (s, folder) => del(`/servers/${id(s)}/resources/${id(folder)}`),
  config: (s) => get(`/servers/${id(s)}/config`, { raw: true }),
  saveConfig: (s, text) => put(`/servers/${id(s)}/config`, text),
  env: (s) => get(`/servers/${id(s)}/env`),
  saveEnv: (s, list) => put(`/servers/${id(s)}/env`, list),
  dataFiles: (s) => get(`/servers/${id(s)}/data`),
  dataFile: (s, f) => get(`/servers/${id(s)}/data/${id(f)}`, { raw: true }),
  saveDataFile: (s, f, text) => put(`/servers/${id(s)}/data/${id(f)}`, text),
  backups: (s) => get(`/servers/${id(s)}/backups`),
  restoreBackup: (s, name) => post(`/servers/${id(s)}/backups/${id(name)}/restore`),
  job: (j) => get(`/jobs/${id(j)}`),

  latch: () => get("/latch"),
  latchLink: () => post("/latch/link"),
  latchCancelLink: () => del("/latch/link"),
  latchKey: (key) => post("/latch/key", { Key: key }),
  latchUnlink: () => del("/latch"),
  library: () => get("/latch/library"),
  installScript: (s, script, restart) => post(`/servers/${id(s)}/scripts/${id(script)}/install`, { Restart: restart }),
  scriptPolicy: (s, script, policy) => put(`/servers/${id(s)}/scripts/${id(script)}/policy`, { Policy: policy }),
}

// consoleStream opens the server's live console (server-sent events).
export function consoleStream(serverId, { onHistory, onLine, onStatus, onError }) {
  const es = new EventSource(`/api/servers/${id(serverId)}/console`)
  es.addEventListener("history", (e) => onHistory?.(JSON.parse(e.data)))
  es.addEventListener("line", (e) => onLine?.(JSON.parse(e.data)))
  es.addEventListener("status", (e) => onStatus?.(JSON.parse(e.data)))
  es.onerror = () => onError?.()
  return () => es.close()
}
