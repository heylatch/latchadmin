import { useState } from "react"
import { Navigate, useOutletContext } from "react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { BellIcon, CircleAlertIcon, EllipsisIcon, PencilIcon, PlusIcon, SendIcon, Trash2Icon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { ConfirmDialog, EmptyState, ErrorNote, PageHeader, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatRelative, isZeroTime } from "@/lib/format"
import { useCan } from "@/lib/session"
import { StatusBoards } from "@/pages/status-boards"
import { cn } from "@/lib/utils"

// Sensible first choice: the events that need attention.
const DEFAULT_EVENTS = ["server.crash", "game.available", "game.failed", "panel.available", "latch.lost"]

function WebhookDialog({ hook, groups, servers, onClose, onSaved }) {
  const { t, i18n } = useTranslation()
  const editing = Boolean(hook)
  const [form, setForm] = useState({
    name: hook?.name ?? "",
    url: "",
    lang: hook?.lang ?? (i18n.resolvedLanguage === "tr" ? "tr" : "en"),
    events: hook?.events ?? DEFAULT_EVENTS,
    servers: hook?.servers ?? [],
    enabled: hook?.enabled ?? true,
  })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))
  const toggle = (e, on) => set("events", on ? [...new Set([...form.events, e])] : form.events.filter((x) => x !== e))
  const allServers = form.servers.length === 0

  async function submit(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      const body = { Name: form.name, URL: form.url.trim(), Lang: form.lang, Events: form.events, Servers: form.servers, Enabled: form.enabled }
      if (editing) await api.updateWebhook(hook.id, body)
      else await api.createWebhook(body)
      toast.success(t("notify.saved"))
      onSaved()
      onClose()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
        <form onSubmit={submit} className="grid gap-6">
          <DialogHeader>
            <DialogTitle>{editing ? t("notify.editTitle", { name: hook.name }) : t("notify.addTitle")}</DialogTitle>
            <DialogDescription>{t("notify.dialogText")}</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
              <Field>
                <FieldLabel htmlFor="wh-name">{t("notify.name")}</FieldLabel>
                <Input id="wh-name" value={form.name} onChange={(e) => set("name", e.target.value)} placeholder={t("notify.namePlaceholder")} maxLength={60} required />
              </Field>
              <Field>
                <FieldLabel htmlFor="wh-lang">{t("notify.lang")}</FieldLabel>
                <Select value={form.lang} onValueChange={(v) => set("lang", v)}>
                  <SelectTrigger id="wh-lang" className="w-full">
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
              <FieldLabel htmlFor="wh-url">{t("notify.url")}</FieldLabel>
              <Input
                id="wh-url"
                type="password"
                autoComplete="off"
                spellCheck={false}
                value={form.url}
                onChange={(e) => set("url", e.target.value)}
                placeholder={editing ? t("notify.urlKeep", { url: hook.url }) : "https://discord.com/api/webhooks/…"}
                className="font-mono"
                required={!editing}
              />
              <FieldDescription>{t("notify.urlHelp")}</FieldDescription>
            </Field>
          </FieldGroup>
          <section className="space-y-2">
            <h3 className="text-sm font-medium">{t("notify.eventsTitle")}</h3>
            <div className="grid gap-3 sm:grid-cols-2">
              {groups.map((g) => (
                <fieldset key={g.id} className="rounded-lg border">
                  <legend className="sr-only">{t(`notify.groups.${g.id}`)}</legend>
                  <p className="border-b bg-muted/40 px-3 py-2 text-sm font-medium">{t(`notify.groups.${g.id}`)}</p>
                  <div className="space-y-2.5 p-3">
                    {g.events.map((ev) => (
                      <label key={ev} htmlFor={`ev-${ev}`} className="flex items-start gap-2.5">
                        <Checkbox id={`ev-${ev}`} checked={form.events.includes(ev)} onCheckedChange={(v) => toggle(ev, v === true)} className="mt-0.5" />
                        <span className="min-w-0">
                          <span className="block text-sm leading-tight">{t(`notify.events.${ev}.label`)}</span>
                          <span className="block text-xs text-muted-foreground">{t(`notify.events.${ev}.help`)}</span>
                        </span>
                      </label>
                    ))}
                  </div>
                </fieldset>
              ))}
            </div>
          </section>
          <section className="space-y-2">
            <h3 className="text-sm font-medium">{t("notify.serversTitle")}</h3>
            <RadioGroup value={allServers ? "all" : "some"} onValueChange={(v) => set("servers", v === "all" ? [] : servers.slice(0, 1).map((s) => s.id))} className="gap-2">
              {["all", "some"].map((v) => (
                <label key={v} htmlFor={`whs-${v}`} className="flex items-start gap-3 rounded-lg border p-3 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring">
                  <RadioGroupItem id={`whs-${v}`} value={v} className="mt-0.5" disabled={v === "some" && !servers.length} />
                  <span>
                    <span className="block text-sm font-medium">{t(`notify.servers.${v}`)}</span>
                    <span className="block text-sm text-muted-foreground">{t(`notify.servers.${v}Help`)}</span>
                  </span>
                </label>
              ))}
            </RadioGroup>
            {!allServers && (
              <div className="space-y-2 rounded-lg border p-3">
                {servers.map((s) => (
                  <label key={s.id} htmlFor={`whsp-${s.id}`} className="flex items-center gap-2.5 text-sm">
                    <Checkbox
                      id={`whsp-${s.id}`}
                      checked={form.servers.includes(s.id)}
                      onCheckedChange={(v) => set("servers", v === true ? [...form.servers, s.id] : form.servers.filter((x) => x !== s.id))}
                    />
                    {s.name}
                  </label>
                ))}
              </div>
            )}
          </section>
          <Field orientation="horizontal">
            <FieldContent>
              <FieldLabel htmlFor="wh-enabled">{t("notify.enabled")}</FieldLabel>
              <FieldDescription>{t("notify.enabledHelp")}</FieldDescription>
            </FieldContent>
            <Switch id="wh-enabled" checked={form.enabled} onCheckedChange={(v) => set("enabled", v)} />
          </Field>
          <ErrorNote error={error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending || !form.name || !form.events.length || (!allServers && !form.servers.length)}>
              {pending && <Spinner />}
              {t(editing ? "common.save" : "notify.add")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function HookCard({ hook, servers, onEdit, onDelete, onChanged }) {
  const { t } = useTranslation()
  const [testing, setTesting] = useState(false)
  async function test() {
    setTesting(true)
    try {
      await api.testWebhook(hook.id)
      toast.success(t("notify.testSent"))
    } catch (err) {
      toast.error(err.message)
    } finally {
      setTesting(false)
      onChanged()
    }
  }
  async function toggle(on) {
    try {
      await api.updateWebhook(hook.id, { Name: hook.name, URL: "", Lang: hook.lang, Events: hook.events, Servers: hook.servers, Enabled: on })
      onChanged()
    } catch (err) {
      toast.error(err.message)
    }
  }
  const names = hook.servers.length ? hook.servers.map((id) => servers.find((s) => s.id === id)?.name ?? "?").join(", ") : t("notify.servers.all")
  return (
    <Card className={cn("gap-4", !hook.enabled && "opacity-70")}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {hook.name}
          {!hook.enabled && <Badge variant="outline">{t("notify.paused")}</Badge>}
        </CardTitle>
        <CardDescription className="font-mono text-xs break-all">{hook.url}</CardDescription>
        <CardAction className="flex items-center gap-2">
          <Switch checked={hook.enabled} onCheckedChange={toggle} aria-label={t("notify.enabled")} />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label={t("common.actions")}>
                <EllipsisIcon aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuItem onSelect={onEdit}>
                <PencilIcon aria-hidden="true" />
                {t("notify.edit")}
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={test} disabled={testing}>
                <SendIcon aria-hidden="true" />
                {t("notify.test")}
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
        <div className="flex flex-wrap gap-1.5">
          {hook.events.map((e) => (
            <Badge key={e} variant="secondary" className="font-normal">
              {t(`notify.events.${e}.label`)}
            </Badge>
          ))}
        </div>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
          <dt className="text-muted-foreground">{t("notify.serversTitle")}</dt>
          <dd className="truncate">{names}</dd>
          <dt className="text-muted-foreground">{t("notify.lang")}</dt>
          <dd>{hook.lang === "tr" ? "Türkçe" : "English"}</dd>
          <dt className="text-muted-foreground">{t("notify.lastSent")}</dt>
          <dd>{isZeroTime(hook.lastSentAt) ? t("notify.never") : formatRelative(hook.lastSentAt)}</dd>
        </dl>
        {hook.lastError && (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <CircleAlertIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            {hook.lastError}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

export default function NotificationsPage() {
  const { t } = useTranslation()
  const can = useCan()
  const { servers } = useOutletContext()
  const query = useApi(api.webhooks)
  const [dialog, setDialog] = useState(null)
  const [removing, setRemoving] = useState(null)
  if (!can("panel.settings")) return <Navigate to="/" replace />

  return (
    <div className="space-y-10">
      <PageHeader title={t("nav.notifications")} description={t("notify.pageText")} />
      <section className="space-y-4" aria-labelledby="alerts-title">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div className="space-y-1">
            <h2 id="alerts-title" className="text-lg font-semibold">
              {t("notify.sectionTitle")}
            </h2>
            <p className="max-w-2xl text-sm text-muted-foreground">{t("notify.text")}</p>
          </div>
          <Button onClick={() => setDialog({})} disabled={!query.data}>
            <PlusIcon aria-hidden="true" />
            {t("notify.add")}
          </Button>
        </div>
        <Query query={query}>
          {(d) =>
            d.webhooks.length ? (
              <div className="grid items-start gap-4 lg:grid-cols-2">
                {d.webhooks.map((h) => (
                  <HookCard key={h.id} hook={h} servers={servers.data ?? []} onEdit={() => setDialog({ hook: h })} onDelete={() => setRemoving(h)} onChanged={query.reload} />
                ))}
              </div>
            ) : (
              <EmptyState icon={BellIcon} title={t("notify.emptyTitle")} description={t("notify.emptyText")}>
                <Button onClick={() => setDialog({})}>
                  <PlusIcon aria-hidden="true" />
                  {t("notify.add")}
                </Button>
              </EmptyState>
            )
          }
        </Query>
      </section>
      <StatusBoards servers={servers.data ?? []} />
      <p className="text-xs text-muted-foreground">{t("notify.privacy")}</p>
      {dialog && query.data && <WebhookDialog hook={dialog.hook} groups={query.data.groups} servers={servers.data ?? []} onClose={() => setDialog(null)} onSaved={query.reload} />}
      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("notify.deleteTitle", { name: removing?.name })}
        description={t("notify.deleteText")}
        confirmLabel={t("notify.delete")}
        destructive
        onConfirm={async () => {
          await api.deleteWebhook(removing.id)
          query.reload()
        }}
      />
    </div>
  )
}
