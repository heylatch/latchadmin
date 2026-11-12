import { useState } from "react"
import { useNavigate, useOutletContext } from "react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { DownloadIcon, PlusIcon, XIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { ConfirmButton, ErrorNote, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { useCan } from "@/lib/session"
import { NoticesCard } from "@/pages/server/notices"

function General({ server, reload }) {
  const { t } = useTranslation()
  const [form, setForm] = useState({ name: server.name, autoStart: server.autoStart, autoRestart: server.autoRestart, restarts: server.restarts ?? [] })
  const [time, setTime] = useState("06:00")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))

  async function save(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      await api.patchServer(server.id, { Name: form.name, AutoStart: form.autoStart, AutoRestart: form.autoRestart, Restarts: form.restarts })
      toast.success(t("config.saved"))
      reload()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Card>
      <form onSubmit={save} className="contents">
        <CardHeader>
          <CardTitle>{t("serverSettings.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="sv-name">{t("add.name")}</FieldLabel>
              <Input id="sv-name" value={form.name} onChange={(e) => set("name", e.target.value)} maxLength={60} required />
            </Field>
            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel htmlFor="sv-autostart">{t("serverSettings.autoStart")}</FieldLabel>
                <FieldDescription>{t("serverSettings.autoStartHelp")}</FieldDescription>
              </FieldContent>
              <Switch id="sv-autostart" checked={form.autoStart} onCheckedChange={(v) => set("autoStart", v)} />
            </Field>
            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel htmlFor="sv-autorestart">{t("serverSettings.autoRestart")}</FieldLabel>
                <FieldDescription>{t("serverSettings.autoRestartHelp")}</FieldDescription>
              </FieldContent>
              <Switch id="sv-autorestart" checked={form.autoRestart} onCheckedChange={(v) => set("autoRestart", v)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="sv-time">{t("serverSettings.restarts")}</FieldLabel>
              <div className="flex flex-wrap items-center gap-2">
                {form.restarts.map((r) => (
                  <Badge key={r} variant="secondary" className="gap-1 font-mono">
                    {r}
                    <button type="button" onClick={() => set("restarts", form.restarts.filter((x) => x !== r))} aria-label={t("serverSettings.removeTime", { time: r })} className="rounded-sm hover:text-foreground">
                      <XIcon className="size-3" aria-hidden="true" />
                    </button>
                  </Badge>
                ))}
                <Input id="sv-time" type="time" value={time} onChange={(e) => setTime(e.target.value)} className="h-8 w-32" />
                <Button type="button" size="sm" variant="outline" disabled={!time || form.restarts.includes(time)} onClick={() => set("restarts", [...form.restarts, time].sort())}>
                  <PlusIcon aria-hidden="true" />
                  {t("serverSettings.addTime")}
                </Button>
              </div>
              <FieldDescription>{t("serverSettings.restartsHelp")}</FieldDescription>
            </Field>
            <ErrorNote error={error} />
          </FieldGroup>
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" disabled={pending}>
            {pending && <Spinner />}
            {t("common.save")}
          </Button>
        </CardFooter>
      </form>
    </Card>
  )
}

function Logs({ server }) {
  const { t } = useTranslation()
  const logs = useApi(() => api.logs(server.id), [server.id])
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("serverSettings.logs")}</CardTitle>
        <CardDescription>{t("serverSettings.logsText")}</CardDescription>
      </CardHeader>
      <CardContent>
        <Query query={logs} rows={1}>
          {(list) =>
            list.length ? (
              <ul className="divide-y rounded-lg border">
                {list.slice(0, 14).map((l) => (
                  <li key={l.name} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                    <span className="font-mono">{l.name}</span>
                    <span className="ml-auto text-muted-foreground tabular-nums">{formatBytes(l.size)}</span>
                    <Button size="icon-sm" variant="ghost" asChild>
                      <a href={`/api/servers/${server.id}/logs/${l.name}`} aria-label={t("serverSettings.download", { file: l.name })}>
                        <DownloadIcon aria-hidden="true" />
                      </a>
                    </Button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground">{t("serverSettings.noLogs")}</p>
            )
          }
        </Query>
      </CardContent>
    </Card>
  )
}

export default function SettingsTab({ server, reload }) {
  const { t } = useTranslation()
  const can = useCan()
  const navigate = useNavigate()
  const { servers } = useOutletContext()
  const stopped = server.status.state === "stopped" || server.status.state === "crashed"
  return (
    <div className="grid items-start gap-6 lg:grid-cols-2">
      <div className="space-y-6">
        <General server={server} reload={reload} />
        <NoticesCard server={server} reload={reload} />
      </div>
      <div className="space-y-6">
        {can("console.view") && <Logs server={server} />}
        {can("server.remove") && (
          <Card>
            <CardHeader>
              <CardTitle>{t("serverSettings.removeTitle")}</CardTitle>
              <CardDescription>{t("serverSettings.removeText", { dir: server.dir })}</CardDescription>
            </CardHeader>
            <CardFooter className="justify-between gap-3">
              {!stopped && <p className="text-xs text-muted-foreground">{t("serverSettings.stopFirst")}</p>}
              <ConfirmButton
                variant="destructive"
                className="ml-auto"
                disabled={!stopped}
                title={t("serverSettings.removeConfirm", { name: server.name })}
                description={t("serverSettings.removeText", { dir: server.dir })}
                confirmLabel={t("serverSettings.remove")}
                destructive
                onConfirm={async () => {
                  await api.deleteServer(server.id)
                  servers.reload()
                  navigate("/")
                }}
              >
                {t("serverSettings.remove")}
              </ConfirmButton>
            </CardFooter>
          </Card>
        )}
      </div>
    </div>
  )
}
