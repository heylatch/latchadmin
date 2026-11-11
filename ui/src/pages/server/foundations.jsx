import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowUpRightIcon, CircleAlertIcon, DatabaseIcon, DownloadIcon, LayersIcon, PencilIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { ErrorNote } from "@/components/common"
import { DatabaseFields, databaseRequest, emptyDatabase } from "@/components/database-fields"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { useCan } from "@/lib/session"

function DatabaseDialog({ server, onClose, onSaved }) {
  const { t } = useTranslation()
  const [value, setValue] = useState(emptyDatabase)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  async function submit(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      await api.setDatabase(server.id, databaseRequest(value))
      toast.success(t("db.saved"))
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
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle>{t("db.title")}</DialogTitle>
            <DialogDescription>{t("db.text")}</DialogDescription>
          </DialogHeader>
          <DatabaseFields value={value} onChange={setValue} disabled={pending} idPrefix="srv-db" />
          <ErrorNote error={error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending}>
              {pending && <Spinner />}
              {value.mode === "create" ? t("db.createSubmit") : t("db.saveSubmit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// FoundationsCard: HMP Foundations (github.com/arcane-mp/foundations) on an Arcane MP server, installed and
// updated as one pack, and the database it needs.
export function FoundationsCard({ server, reload }) {
  const { t } = useTranslation()
  const can = useCan()
  const status = useApi(() => api.foundations(server.id), [server.id])
  const [job, setJob] = useState(null)
  const [dialog, setDialog] = useState(false)
  const s = status.data
  const busy = job && !job.finished

  useEffect(() => {
    if (!job || job.finished) return
    const timer = setTimeout(async () => {
      const next = await api.job(job.id).catch(() => null)
      if (next) setJob(next)
      if (next?.finished) {
        status.reload()
        reload()
        if (next.error) toast.error(next.error)
        else toast.success(t("found.done", { version: next.result?.version }))
      }
    }, 1000)
    return () => clearTimeout(timer)
  }, [job, status, reload, t])

  async function install() {
    try {
      setJob(await api.installFoundations(server.id))
    } catch (err) {
      toast.error(err.message)
    }
  }

  const outdated = s?.installed && s?.latest && s.installed !== s.latest
  const running = server.status.state === "running" || server.status.state === "starting"
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LayersIcon className="size-4 text-muted-foreground" aria-hidden="true" />
          HMP Foundations
          {s?.installed && <Badge variant="secondary" className="font-mono font-normal">{s.installed}</Badge>}
        </CardTitle>
        <CardDescription>{t("found.text")}</CardDescription>
        <CardAction>
          <Button variant="ghost" size="sm" asChild>
            <a href={s?.url || "https://github.com/arcane-mp/foundations"} target="_blank" rel="noreferrer">
              {t("found.notes")}
              <ArrowUpRightIcon data-icon="inline-end" aria-hidden="true" />
            </a>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-4">
        {!s ? (
          status.error ? <ErrorNote error={status.error} /> : <Skeleton className="h-16 w-full" />
        ) : (
          <>
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4">
              <div className="space-y-0.5">
                <p className="font-medium">
                  {!s.installed ? t("found.notInstalled") : outdated ? t("found.available", { version: s.latest }) : t("found.upToDate", { version: s.installed })}
                </p>
                <p className="text-sm text-muted-foreground">
                  {s.error ? t("found.checkFailed") : !s.installed ? t("found.installHelp", { version: s.latest }) : outdated ? t("found.updateHelp") : t("found.latestHelp")}
                </p>
              </div>
              {can("resources.manage") && (!s.installed || outdated) && (
                <Button onClick={install} disabled={busy || !s.latest}>
                  {busy ? <Spinner /> : <DownloadIcon aria-hidden="true" />}
                  {s.installed ? t("found.update") : t("found.install")}
                </Button>
              )}
            </div>
            {busy && <p className="text-sm text-muted-foreground" aria-live="polite">{t(`found.stage.${job.stage}`, { defaultValue: t("found.working") })}{running ? ` ${t("found.restarts")}` : ""}</p>}

            {s.conflicts.length > 0 && (
              <Alert>
                <CircleAlertIcon aria-hidden="true" />
                <AlertTitle>{t("found.conflictsTitle")}</AlertTitle>
                <AlertDescription>
                  <p>{t("found.conflictsText")}</p>
                  <p className="font-mono text-xs">{s.conflicts.join(", ")}</p>
                </AlertDescription>
              </Alert>
            )}

            <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4">
              <div className="flex min-w-0 items-start gap-3">
                <DatabaseIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <div className="min-w-0 space-y-0.5">
                  <p className="font-medium">{t("found.database")}</p>
                  {s.database ? (
                    <p className="truncate font-mono text-xs text-muted-foreground">
                      {s.database.user}@{s.database.host}:{s.database.port || 3306}/{s.database.database}
                    </p>
                  ) : (
                    <p className="text-sm text-muted-foreground">{t("found.noDatabase")}</p>
                  )}
                </div>
              </div>
              {can("all") && (
                <Button variant={s.database ? "outline" : "default"} size="sm" onClick={() => setDialog(true)}>
                  {s.database ? <PencilIcon aria-hidden="true" /> : <DatabaseIcon aria-hidden="true" />}
                  {s.database ? t("found.changeDatabase") : t("found.setupDatabase")}
                </Button>
              )}
            </div>
            {s.database && <p className="text-xs text-muted-foreground">{t("found.restartNote")}</p>}
          </>
        )}
      </CardContent>
      {dialog && <DatabaseDialog server={server} onClose={() => setDialog(false)} onSaved={status.reload} />}
    </Card>
  )
}
