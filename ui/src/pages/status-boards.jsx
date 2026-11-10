import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowDownToLineIcon, CircleAlertIcon, EllipsisIcon, MonitorSmartphoneIcon, PencilIcon, PlusIcon, RefreshCwIcon, Trash2Icon, XIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { ConfirmDialog, EmptyState, ErrorNote, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatRelative, isZeroTime } from "@/lib/format"
import { cn } from "@/lib/utils"

const MAX_SERVERS = 10

// Same mapping as the panel uses for the Discord message (internal/app/status.go).
function boardState(status) {
  switch (status?.state) {
    case "running":
      return "online"
    case "starting":
    case "stopping":
    case "updating":
      return status.state
    case "crashed":
      return status.nextStart && !isZeroTime(status.nextStart) ? "restarting" : "offline"
    default:
      return "offline"
  }
}
const accent = { online: "border-l-success", offline: "border-l-destructive" }
const accentFor = (s) => accent[s] ?? "border-l-warning"

// Preview draws the message roughly as Discord shows it, in the board's language, from the servers' live status.
function Preview({ form, servers }) {
  const { i18n } = useTranslation()
  const t = i18n.getFixedT(form.lang)
  const rows = form.servers.map((e) => ({ ...e, server: servers.find((s) => s.id === e.serverId) })).filter((e) => e.server)
  const online = rows.filter((e) => boardState(e.server.status) === "online")
  const known = online.some((e) => e.server.status?.players != null)
  const players = online.reduce((n, e) => n + (e.server.status?.players ?? 0), 0)
  const buttons = form.buttons.filter((b) => b.label.trim())
  return (
    <div className="space-y-2 rounded-lg border bg-muted/30 p-3 text-sm" aria-label={t("board.preview")}>
      <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{t("board.preview")}</p>
      <div className="space-y-1 rounded-md border bg-background p-3">
        <p className="text-base font-semibold break-words">{form.title || t("board.titleDefault")}</p>
        <p className="text-muted-foreground">
          {t("board.summary", { online: online.length, total: rows.length })}
          {known && ` · ${t("board.players", { count: players })}`}
        </p>
        {form.note.trim() && <p className="pt-1 whitespace-pre-line break-words">{form.note.trim()}</p>}
      </div>
      {rows.map((e) => {
        const st = boardState(e.server.status)
        const p = e.server.status?.players
        return (
          <div key={e.serverId} className={cn("space-y-0.5 rounded-md border border-l-4 bg-background p-3", accentFor(st))}>
            <p className="font-semibold">{e.server.name}</p>
            <p>
              <span className="font-medium">{t(`board.state.${st}`)}</span>
              <span className="text-muted-foreground">
                {" "}
                · {e.server.gameName}
                {e.server.version ? ` ${e.server.version}` : ""}
              </span>
            </p>
            {st === "online" && p != null && (
              <p>
                <span className="font-medium">{t("board.f.players")}:</span> {p}
                {e.server.status?.maxPlayers ? ` / ${e.server.status.maxPlayers}` : ""}
              </p>
            )}
            {e.address.trim() && (
              <p>
                <span className="font-medium">{t("board.f.address")}:</span> <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">{e.address.trim()}</code>
              </p>
            )}
          </div>
        )
      })}
      <p className="text-xs text-muted-foreground">{t("board.updatedNow")} · LatchAdmin</p>
      {buttons.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {buttons.map((b, i) => (
            <span key={i} className="rounded-md bg-secondary px-3 py-1.5 text-xs font-medium text-secondary-foreground">
              {b.label}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

function BoardDialog({ board, servers, onClose, onSaved }) {
  const { t, i18n } = useTranslation()
  const editing = Boolean(board)
  const lang = board?.lang ?? (i18n.resolvedLanguage === "tr" ? "tr" : "en")
  const [form, setForm] = useState({
    name: board?.name ?? "",
    url: "",
    lang,
    title: board?.title ?? i18n.getFixedT(lang)("board.titleDefault"),
    note: board?.note ?? "",
    servers: board?.servers.map((s) => ({ serverId: s.serverId, address: s.address ?? "" })) ?? servers.slice(0, MAX_SERVERS).map((s) => ({ serverId: s.id, address: "" })),
    buttons: board?.buttons.map((b) => ({ ...b })) ?? [],
    enabled: board?.enabled ?? true,
  })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))
  const entry = (id) => form.servers.find((e) => e.serverId === id)
  const toggleServer = (id, on) =>
    set(
      "servers",
      on
        ? // keep the panel's server order
          servers.filter((s) => s.id === id || entry(s.id)).map((s) => entry(s.id) ?? { serverId: id, address: "" })
        : form.servers.filter((e) => e.serverId !== id),
    )
  const setAddress = (id, address) => set("servers", form.servers.map((e) => (e.serverId === id ? { ...e, address } : e)))
  const setButton = (i, k, v) => set("buttons", form.buttons.map((b, j) => (j === i ? { ...b, [k]: v } : b)))

  async function submit(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      const body = {
        Name: form.name,
        URL: form.url.trim(),
        Lang: form.lang,
        Title: form.title,
        Note: form.note,
        Servers: form.servers.map((s) => ({ serverId: s.serverId, address: s.address.trim() })),
        Buttons: form.buttons.filter((b) => b.label.trim() || b.url.trim()),
        Enabled: form.enabled,
      }
      const saved = editing ? await api.updateBoard(board.id, body) : await api.createBoard(body)
      if (saved.lastError) toast.error(saved.lastError)
      else toast.success(t(form.enabled ? "board.savedPosted" : "board.saved"))
      onSaved()
      onClose()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  const full = form.servers.length >= MAX_SERVERS
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-4xl">
        <form onSubmit={submit} className="grid gap-6">
          <DialogHeader>
            <DialogTitle>{editing ? t("notify.editTitle", { name: board.name }) : t("board.addTitle")}</DialogTitle>
            <DialogDescription>{t("board.dialogText")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,20rem)]">
            <div className="grid min-w-0 content-start gap-6">
              <FieldGroup>
                <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
                  <Field>
                    <FieldLabel htmlFor="bd-name">{t("notify.name")}</FieldLabel>
                    <Input id="bd-name" value={form.name} onChange={(e) => set("name", e.target.value)} placeholder={t("board.namePlaceholder")} maxLength={60} required />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="bd-lang">{t("notify.lang")}</FieldLabel>
                    <Select value={form.lang} onValueChange={(v) => set("lang", v)}>
                      <SelectTrigger id="bd-lang" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent position="popper">
                        <SelectItem value="en">English</SelectItem>
                        <SelectItem value="tr">Türkçe</SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                </div>
                <Field>
                  <FieldLabel htmlFor="bd-url">{t("notify.url")}</FieldLabel>
                  <Input
                    id="bd-url"
                    type="password"
                    autoComplete="off"
                    spellCheck={false}
                    value={form.url}
                    onChange={(e) => set("url", e.target.value)}
                    placeholder={editing ? t("notify.urlKeep", { url: board.url }) : "https://discord.com/api/webhooks/…"}
                    className="font-mono"
                    required={!editing}
                  />
                  <FieldDescription>{t("board.urlHelp")}</FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="bd-title">{t("board.title")}</FieldLabel>
                  <Input id="bd-title" value={form.title} onChange={(e) => set("title", e.target.value)} maxLength={80} required />
                </Field>
                <Field>
                  <FieldLabel htmlFor="bd-note">{t("board.note")}</FieldLabel>
                  <Textarea id="bd-note" value={form.note} onChange={(e) => set("note", e.target.value)} placeholder={t("board.notePlaceholder")} maxLength={1000} rows={3} />
                  <FieldDescription>{t("board.noteHelp")}</FieldDescription>
                </Field>
              </FieldGroup>

              <section className="space-y-2">
                <div className="flex items-baseline justify-between gap-2">
                  <h3 className="text-sm font-medium">{t("notify.serversTitle")}</h3>
                  <span className="text-xs text-muted-foreground tabular-nums">
                    {form.servers.length} / {MAX_SERVERS}
                  </span>
                </div>
                <div className="divide-y rounded-lg border">
                  {servers.map((s) => {
                    const e = entry(s.id)
                    return (
                      <div key={s.id} className="grid gap-2 p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,14rem)] sm:items-center">
                        <label htmlFor={`bds-${s.id}`} className="flex min-w-0 items-center gap-2.5 text-sm">
                          <Checkbox id={`bds-${s.id}`} checked={Boolean(e)} disabled={!e && full} onCheckedChange={(v) => toggleServer(s.id, v === true)} />
                          <span className="truncate">{s.name}</span>
                        </label>
                        {e && (
                          <Input
                            aria-label={t("board.addressFor", { name: s.name })}
                            value={e.address}
                            onChange={(ev) => setAddress(s.id, ev.target.value)}
                            placeholder={`play.example.com:${s.port}`}
                            spellCheck={false}
                            maxLength={100}
                            className="h-8 font-mono text-xs"
                          />
                        )}
                      </div>
                    )
                  })}
                </div>
                <p className="text-xs text-muted-foreground">{t("board.addressHelp")}</p>
              </section>

              <section className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <h3 className="text-sm font-medium">{t("board.buttons")}</h3>
                  <Button type="button" variant="outline" size="sm" disabled={form.buttons.length >= 3} onClick={() => set("buttons", [...form.buttons, { label: "", url: "" }])}>
                    <PlusIcon aria-hidden="true" />
                    {t("board.addButton")}
                  </Button>
                </div>
                {form.buttons.length === 0 ? (
                  <p className="text-xs text-muted-foreground">{t("board.buttonsHelp")}</p>
                ) : (
                  <div className="space-y-2">
                    {form.buttons.map((b, i) => (
                      <div key={i} className="grid grid-cols-[minmax(0,10rem)_minmax(0,1fr)_auto] gap-2">
                        <Input aria-label={t("board.buttonLabel")} value={b.label} onChange={(e) => setButton(i, "label", e.target.value)} placeholder={t("board.buttonLabelPlaceholder")} maxLength={40} className="h-8" />
                        <Input aria-label={t("board.buttonUrl")} value={b.url} onChange={(e) => setButton(i, "url", e.target.value)} placeholder="https://discord.gg/…" spellCheck={false} className="h-8 font-mono text-xs" />
                        <Button type="button" variant="ghost" size="icon-sm" aria-label={t("board.removeButton")} onClick={() => set("buttons", form.buttons.filter((_, j) => j !== i))}>
                          <XIcon aria-hidden="true" />
                        </Button>
                      </div>
                    ))}
                  </div>
                )}
              </section>

              <Field orientation="horizontal">
                <FieldContent>
                  <FieldLabel htmlFor="bd-enabled">{t("board.enabled")}</FieldLabel>
                  <FieldDescription>{t("board.enabledHelp")}</FieldDescription>
                </FieldContent>
                <Switch id="bd-enabled" checked={form.enabled} onCheckedChange={(v) => set("enabled", v)} />
              </Field>
            </div>
            <div className="min-w-0 lg:sticky lg:top-0 lg:self-start">
              <Preview form={form} servers={servers} />
            </div>
          </div>
          <ErrorNote error={error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending || !form.name || !form.title.trim() || !form.servers.length}>
              {pending && <Spinner />}
              {t(editing ? "common.save" : form.enabled ? "board.addPost" : "board.add")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function BoardCard({ board, servers, onEdit, onDelete, onChanged }) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  async function refresh(repost) {
    setBusy(true)
    try {
      await api.refreshBoard(board.id, repost)
      toast.success(t(repost ? "board.reposted" : "board.refreshed"))
    } catch (err) {
      toast.error(err.message)
    } finally {
      setBusy(false)
      onChanged()
    }
  }
  async function toggle(on) {
    try {
      await api.updateBoard(board.id, {
        Name: board.name,
        URL: "",
        Lang: board.lang,
        Title: board.title,
        Note: board.note,
        Servers: board.servers,
        Buttons: board.buttons,
        Enabled: on,
      })
      onChanged()
    } catch (err) {
      toast.error(err.message)
    }
  }
  return (
    <Card className={cn("gap-4", !board.enabled && "opacity-70")}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {board.name}
          {!board.enabled && <Badge variant="outline">{t("notify.paused")}</Badge>}
        </CardTitle>
        <CardDescription className="font-mono text-xs break-all">{board.url}</CardDescription>
        <CardAction className="flex items-center gap-2">
          <Switch checked={board.enabled} onCheckedChange={toggle} aria-label={t("board.enabled")} />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label={t("common.actions")}>
                <EllipsisIcon aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-52">
              <DropdownMenuItem onSelect={onEdit}>
                <PencilIcon aria-hidden="true" />
                {t("notify.edit")}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => refresh(false)} disabled={busy || !board.enabled}>
                <RefreshCwIcon aria-hidden="true" />
                {t("board.refresh")}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => refresh(true)} disabled={busy || !board.enabled}>
                <ArrowDownToLineIcon aria-hidden="true" />
                {t("board.repost")}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={onDelete}>
                <Trash2Icon aria-hidden="true" />
                {t("notify.delete")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        <ul className="divide-y rounded-lg border text-sm">
          {board.servers.map((e) => {
            const s = servers.find((x) => x.id === e.serverId)
            const st = boardState(s?.status)
            return (
              <li key={e.serverId} className="flex items-center justify-between gap-3 px-3 py-2">
                <span className="flex min-w-0 items-center gap-2">
                  <span className={cn("size-2 shrink-0 rounded-full", st === "online" ? "bg-success" : st === "offline" ? "bg-destructive" : "bg-warning")} aria-hidden="true" />
                  <span className="truncate">{s?.name ?? t("board.removedServer")}</span>
                </span>
                <span className="truncate font-mono text-xs text-muted-foreground">{e.address || t("board.noAddress")}</span>
              </li>
            )
          })}
        </ul>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <dt className="text-muted-foreground">{t("notify.lang")}</dt>
          <dd>{board.lang === "tr" ? "Türkçe" : "English"}</dd>
          <dt className="text-muted-foreground">{t("board.lastUpdate")}</dt>
          <dd>{isZeroTime(board.lastSentAt) ? t("notify.never") : formatRelative(board.lastSentAt)}</dd>
        </dl>
        {board.lastError && (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <CircleAlertIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            {board.lastError}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

export function StatusBoards({ servers }) {
  const { t } = useTranslation()
  const query = useApi(api.boards)
  const [dialog, setDialog] = useState(null)
  const [removing, setRemoving] = useState(null)
  return (
    <section className="space-y-4" aria-labelledby="boards-title">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="space-y-1">
          <h2 id="boards-title" className="text-lg font-semibold">
            {t("board.sectionTitle")}
          </h2>
          <p className="max-w-2xl text-sm text-muted-foreground">{t("board.sectionText")}</p>
        </div>
        <Button variant="outline" onClick={() => setDialog({})} disabled={!servers.length}>
          <PlusIcon aria-hidden="true" />
          {t("board.add")}
        </Button>
      </div>
      <Query query={query}>
        {(d) =>
          d.boards.length ? (
            <div className="grid items-start gap-4 lg:grid-cols-2">
              {d.boards.map((b) => (
                <BoardCard key={b.id} board={b} servers={servers} onEdit={() => setDialog({ board: b })} onDelete={() => setRemoving(b)} onChanged={query.reload} />
              ))}
            </div>
          ) : (
            <EmptyState icon={MonitorSmartphoneIcon} title={t("board.emptyTitle")} description={t("board.emptyText")}>
              <Button onClick={() => setDialog({})} disabled={!servers.length}>
                <PlusIcon aria-hidden="true" />
                {t("board.add")}
              </Button>
            </EmptyState>
          )
        }
      </Query>
      {dialog && <BoardDialog board={dialog.board} servers={servers} onClose={() => setDialog(null)} onSaved={query.reload} />}
      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("notify.deleteTitle", { name: removing?.name })}
        description={t("board.deleteText")}
        confirmLabel={t("notify.delete")}
        destructive
        onConfirm={async () => {
          await api.deleteBoard(removing.id)
          query.reload()
        }}
      />
    </section>
  )
}
