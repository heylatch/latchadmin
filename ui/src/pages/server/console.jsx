import { useEffect, useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { ArrowDownIcon, SendIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { api, consoleStream } from "@/lib/api"
import { useCan } from "@/lib/session"
import { cn } from "@/lib/utils"

const MAX = 3000
// MafiaHub prints this for every export lookup; it buries everything else.
const noise = /Imported \d+ exports from/

export default function ConsoleTab({ server }) {
  const { t } = useTranslation()
  const can = useCan()
  const [lines, setLines] = useState([])
  const [hideNoise, setHideNoise] = useState(true)
  const [filter, setFilter] = useState("")
  const [follow, setFollow] = useState(true)
  const [connected, setConnected] = useState(true)
  const [cmd, setCmd] = useState("")
  const [error, setError] = useState(null)
  const history = useRef([])
  const histPos = useRef(-1)
  const box = useRef(null)

  useEffect(() => {
    setLines([])
    return consoleStream(server.id, {
      onHistory: (h) => (setLines(h.slice(-MAX)), setConnected(true)),
      onLine: (l) => setLines((prev) => (prev.length >= MAX ? [...prev.slice(-MAX + 1), l] : [...prev, l])),
      onStatus: () => setConnected(true),
      onError: () => setConnected(false),
    })
  }, [server.id])

  const shown = useMemo(() => {
    const f = filter.toLowerCase()
    return lines.filter((l) => (!hideNoise || !noise.test(l.text)) && (!f || l.text.toLowerCase().includes(f)))
  }, [lines, hideNoise, filter])

  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [shown, follow])

  function onScroll() {
    const el = box.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    if (atBottom !== follow) setFollow(atBottom)
  }

  async function send(e) {
    e.preventDefault()
    const c = cmd.trim()
    if (!c) return
    setError(null)
    try {
      await api.command(server.id, c)
      history.current = [c, ...history.current.filter((x) => x !== c)].slice(0, 50)
      histPos.current = -1
      setCmd("")
      setFollow(true)
    } catch (err) {
      setError(err.message)
    }
  }

  function onKey(e) {
    if (e.key === "ArrowUp" && history.current.length) {
      e.preventDefault()
      histPos.current = Math.min(history.current.length - 1, histPos.current + 1)
      setCmd(history.current[histPos.current])
    } else if (e.key === "ArrowDown") {
      e.preventDefault()
      histPos.current = Math.max(-1, histPos.current - 1)
      setCmd(histPos.current < 0 ? "" : history.current[histPos.current])
    }
  }

  const running = server.status.state === "running" || server.status.state === "starting"
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
        <span className="ml-auto text-xs text-muted-foreground tabular-nums">{t("console.lines", { count: shown.length })}</span>
      </div>
      <Card className="relative gap-0 overflow-hidden p-0">
        <div ref={box} onScroll={onScroll} role="log" aria-live="off" className="h-[60vh] min-h-80 overflow-y-auto bg-muted/30 p-3 font-mono text-xs leading-5">
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
          <Button size="sm" variant="secondary" className="absolute right-4 bottom-4 shadow" onClick={() => setFollow(true)}>
            <ArrowDownIcon aria-hidden="true" />
            {t("console.follow")}
          </Button>
        )}
        {can("admin") && (
          <form onSubmit={send} className="flex gap-2 border-t p-2">
            <span className="flex items-center pl-1 font-mono text-sm text-muted-foreground" aria-hidden="true">
              &gt;
            </span>
            <Input
              value={cmd}
              onChange={(e) => setCmd(e.target.value)}
              onKeyDown={onKey}
              disabled={!running}
              placeholder={running ? t("console.commandPlaceholder") : t("console.notRunning")}
              className="border-0 font-mono shadow-none focus-visible:ring-0"
              aria-label={t("console.command")}
              autoComplete="off"
              spellCheck={false}
            />
            <Button type="submit" size="sm" disabled={!running || !cmd.trim()}>
              <SendIcon aria-hidden="true" />
              {t("console.send")}
            </Button>
          </form>
        )}
      </Card>
      {error && <p className="text-sm text-destructive">{error}</p>}
      <p className="text-xs text-muted-foreground">{t("console.help")}</p>
    </div>
  )
}
