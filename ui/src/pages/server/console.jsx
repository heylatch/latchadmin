import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { ArrowDownIcon, BoxIcon, DownloadIcon, EraserIcon, MegaphoneIcon, SendIcon, TerminalIcon, TriangleAlertIcon } from "lucide-react"
import { AnnounceDialog } from "@/components/announce-dialog"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Kbd } from "@/components/ui/kbd"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { useApi } from "@/hooks/use-api"
import { api, consoleStream } from "@/lib/api"
import { useCan } from "@/lib/session"
import { cn } from "@/lib/utils"

const MAX = 3000
// MafiaHub prints this for every export lookup; it buries everything else.
const noise = /Imported \d+ exports from/
// Commands that take a resource name.
const RESOURCE_VERBS = ["restart", "ensure", "start", "stop"]
// Used until the server has told us its own list ("help").
const FALLBACK = [
  { name: "status", description: "Show server status" },
  { name: "help", description: "List commands" },
  { name: "restart", description: "Reload a running resource's code: restart <resource>" },
  { name: "ensure", description: "Start a resource if it isn't running: ensure <resource>" },
  { name: "start", description: "Start a resource: start <resource>" },
  { name: "stop", description: "Stop a resource: stop <resource>. Alone, stops the server" },
  { name: "refresh", description: "Re-scan the resources directory for new/changed resources" },
  { name: "refreshall", description: "Re-scan and reload all running resources from disk" },
  { name: "quit", description: "Stop the server" },
]
const DANGER = new Set(["quit", "exit", "shutdown"])

// suggestions for what's typed so far: command names, or resource names after a resource command.
function suggest(text, commands, resources) {
  if (!text || text.startsWith(" ")) return []
  const space = text.indexOf(" ")
  if (space < 0) {
    const q = text.toLowerCase()
    return commands
      .filter((c) => c.name.toLowerCase().startsWith(q) && c.name.toLowerCase() !== q)
      .slice(0, 8)
      .map((c) => ({ kind: "command", value: c.name, label: c.name, description: c.description, danger: DANGER.has(c.name) }))
  }
  const verb = text.slice(0, space).toLowerCase()
  const rest = text.slice(space + 1)
  if (!RESOURCE_VERBS.includes(verb) || rest.includes(" ")) return []
  const q = rest.toLowerCase()
  return resources
    .filter((r) => r.enabled && r.name.toLowerCase().includes(q) && r.name.toLowerCase() !== q)
    .slice(0, 8)
    .map((r) => ({ kind: "resource", value: `${verb} ${r.name}`, label: r.name, description: r.version ? `v${r.version}` : "" }))
}

export default function ConsoleTab({ server }) {
  const { t } = useTranslation()
  const can = useCan()
  const resourcesQ = useApi(() => api.resources(server.id), [server.id])
  const [lines, setLines] = useState([])
  const [clearedAt, setClearedAt] = useState(0)
  const [announcing, setAnnouncing] = useState(false)
  const [hideNoise, setHideNoise] = useState(true)
  const [filter, setFilter] = useState("")
  const [follow, setFollow] = useState(true)
  const [connected, setConnected] = useState(true)
  const [cmd, setCmd] = useState("")
  const [active, setActive] = useState(0)
  const [menuOpen, setMenuOpen] = useState(true)
  const [error, setError] = useState(null)
  const history = useRef([])
  const histPos = useRef(-1)
  const box = useRef(null)
  const input = useRef(null)

  useEffect(() => {
    setLines([])
    setClearedAt(0)
    return consoleStream(server.id, {
      onHistory: (h) => (setLines(h.slice(-MAX)), setConnected(true)),
      onLine: (l) => setLines((prev) => (prev.length >= MAX ? [...prev.slice(-MAX + 1), l] : [...prev, l])),
      onStatus: () => setConnected(true),
      onError: () => setConnected(false),
    })
  }, [server.id])

  const visible = useMemo(() => lines.filter((l) => l.n > clearedAt), [lines, clearedAt])
  const shown = useMemo(() => {
    const f = filter.toLowerCase()
    return visible.filter((l) => (!hideNoise || !noise.test(l.text)) && (!f || l.text.toLowerCase().includes(f)))
  }, [visible, hideNoise, filter])

  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [shown, follow])

  const commands = server.commands?.length ? server.commands : FALLBACK
  const resourceNames = (resourcesQ.data ?? []).map((r) => ({ name: r.name || r.folder, enabled: r.enabled, version: r.version }))
  const items = menuOpen ? suggest(cmd, commands, resourceNames) : []
  const running = server.status.state === "running" || server.status.state === "starting"

  function onScroll() {
    const el = box.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    if (atBottom !== follow) setFollow(atBottom)
  }

  function pick(item) {
    const next = item.kind === "command" && RESOURCE_VERBS.includes(item.value) ? `${item.value} ` : item.value
    setCmd(next)
    setActive(0)
    setMenuOpen(true)
    input.current?.focus()
  }

  async function send(e) {
    e?.preventDefault()
    const c = cmd.trim()
    if (!c) return
    setError(null)
    try {
      await api.command(server.id, c)
      history.current = [c, ...history.current.filter((x) => x !== c)].slice(0, 50)
      histPos.current = -1
      setCmd("")
      setFollow(true)
      if (c === "refresh" || c === "refreshall") resourcesQ.reload()
    } catch (err) {
      setError(err.message)
    }
  }

  function onKey(e) {
    if (items.length) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault()
        setActive((a) => (a + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length)
        return
      }
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) {
        e.preventDefault()
        pick(items[Math.min(active, items.length - 1)])
        return
      }
      if (e.key === "Escape") {
        e.preventDefault()
        setMenuOpen(false)
        return
      }
    }
    if (e.key === "ArrowUp" && history.current.length) {
      e.preventDefault()
      histPos.current = Math.min(history.current.length - 1, histPos.current + 1)
      setCmd(history.current[histPos.current])
      setMenuOpen(false)
    } else if (e.key === "ArrowDown" && histPos.current >= 0) {
      e.preventDefault()
      histPos.current = Math.max(-1, histPos.current - 1)
      setCmd(histPos.current < 0 ? "" : history.current[histPos.current])
      setMenuOpen(false)
    }
  }

  function download() {
    const text = visible.map((l) => `${new Date(l.at).toLocaleString()} ${l.text}`).join("\n")
    const url = URL.createObjectURL(new Blob([text + "\n"], { type: "text/plain;charset=utf-8" }))
    const a = document.createElement("a")
    a.href = url
    a.download = `${server.name.replace(/[^\w.-]+/g, "-")}-console-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.log`
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("console.filter")} className="h-8 max-w-xs" aria-label={t("console.filter")} />
        <div className="flex items-center gap-2">
          <Switch id="noise" checked={hideNoise} onCheckedChange={setHideNoise} />
          <Label htmlFor="noise" className="font-normal">
            {t("console.hideNoise")}
          </Label>
        </div>
        {!connected && <span className="text-sm text-warning">{t("console.reconnecting")}</span>}
        <div className="ml-auto flex items-center gap-1">
          <span className="mr-2 text-xs text-muted-foreground tabular-nums">{t("console.lines", { count: shown.length })}</span>
          {can("console.write") && server.notices?.enabled && (
            <Button variant="outline" size="sm" onClick={() => setAnnouncing(true)} disabled={server.status.state !== "running"}>
              <MegaphoneIcon aria-hidden="true" />
              {t("announce.open")}
            </Button>
          )}
          <Button variant="ghost" size="sm" onClick={() => setClearedAt(lines.at(-1)?.n ?? 0)} disabled={!visible.length}>
            <EraserIcon aria-hidden="true" />
            {t("console.clear")}
          </Button>
          <Button variant="ghost" size="sm" onClick={download} disabled={!visible.length}>
            <DownloadIcon aria-hidden="true" />
            {t("console.download")}
          </Button>
          {announcing && <AnnounceDialog server={server} open={announcing} onOpenChange={setAnnouncing} />}
        </div>
      </div>
      <Card className="relative gap-0 overflow-visible p-0">
        <div ref={box} onScroll={onScroll} role="log" aria-live="off" className="h-[60vh] min-h-80 overflow-y-auto rounded-t-xl bg-muted/30 p-3 font-mono text-xs leading-5">
          {shown.length ? (
            shown.map((l) => (
              <div key={l.n} className={cn("break-words whitespace-pre-wrap", l.src === "err" && "text-destructive", l.src === "panel" && "text-info")}>
                {l.text}
              </div>
            ))
          ) : (
            <p className="text-muted-foreground">{t(running ? "console.waiting" : "console.empty")}</p>
          )}
        </div>
        {!follow && (
          <Button size="sm" variant="secondary" className="absolute right-4 bottom-16 shadow" onClick={() => setFollow(true)}>
            <ArrowDownIcon aria-hidden="true" />
            {t("console.follow")}
          </Button>
        )}
        {can("console.write") && (
          <form onSubmit={send} className="relative border-t p-2">
            {items.length > 0 && (
              <div role="listbox" aria-label={t("console.suggestions")} className="absolute inset-x-2 bottom-full z-20 mb-2 overflow-hidden rounded-lg border bg-popover shadow-lg">
                <p className="border-b px-3 py-1.5 text-xs text-muted-foreground">{t(items[0].kind === "command" ? "console.commandsTitle" : "console.resourcesTitle")}</p>
                {items.map((it, i) => (
                  <button
                    key={it.value}
                    type="button"
                    role="option"
                    aria-selected={i === active}
                    onMouseEnter={() => setActive(i)}
                    onMouseDown={(e) => (e.preventDefault(), pick(it))}
                    className={cn("flex w-full items-center gap-3 px-3 py-2 text-left text-sm", i === active && "bg-accent text-accent-foreground")}
                  >
                    {it.kind === "command" ? <TerminalIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" /> : <BoxIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />}
                    <span className="font-mono">{it.label}</span>
                    <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{it.description}</span>
                    {it.danger && <TriangleAlertIcon className="size-4 shrink-0 text-warning" aria-label={t("console.stopsServer")} />}
                  </button>
                ))}
                <p className="flex items-center gap-1.5 border-t px-3 py-1.5 text-xs text-muted-foreground">
                  <Kbd>↑</Kbd>
                  <Kbd>↓</Kbd> {t("console.navigate")} <Kbd>Tab</Kbd> {t("console.complete")} <Kbd>Esc</Kbd> {t("console.close")}
                </p>
              </div>
            )}
            <div className="flex gap-2">
              <span className="flex items-center pl-1 font-mono text-sm text-muted-foreground" aria-hidden="true">
                &gt;
              </span>
              <Input
                ref={input}
                value={cmd}
                onChange={(e) => (setCmd(e.target.value), setActive(0), setMenuOpen(true), (histPos.current = -1))}
                onKeyDown={onKey}
                onBlur={() => setTimeout(() => setMenuOpen(false), 100)}
                onFocus={() => setMenuOpen(true)}
                disabled={!running}
                placeholder={running ? t("console.commandPlaceholder") : t("console.notRunning")}
                className="border-0 font-mono shadow-none focus-visible:ring-0 dark:bg-transparent"
                aria-label={t("console.command")}
                aria-autocomplete="list"
                aria-expanded={items.length > 0}
                autoComplete="off"
                spellCheck={false}
              />
              <Button type="submit" size="sm" disabled={!running || !cmd.trim()}>
                <SendIcon aria-hidden="true" />
                {t("console.send")}
              </Button>
            </div>
          </form>
        )}
      </Card>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <p className="text-xs text-muted-foreground">{t("console.help")}</p>
    </div>
  )
}
