import { useEffect, useState } from "react"
import { Link } from "react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowUpCircleIcon, CheckCircle2Icon, SirenIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmButton, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { useCan } from "@/lib/session"
import { FoundationsCard } from "@/pages/server/foundations"
import { formatBytes, formatDateTime } from "@/lib/format"

export default function UpdatesTab({ server, reload }) {
  const { t } = useTranslation()
  const can = useCan()
  const backups = useApi(() => api.backups(server.id), [server.id])
  const [job, setJob] = useState(server.job ?? null)
  const latest = server.update?.latest
  const urgent = server.update?.urgent
  const outdated = latest && server.version && latest !== server.version
  const running = server.status.state === "running" || server.status.state === "starting"
  const stopped = server.status.state === "stopped" || server.status.state === "crashed"

  useEffect(() => {
    if (!job || job.finished) return
    const timer = setTimeout(async () => {
      const next = await api.job(job.id).catch(() => null)
      if (next) setJob(next)
      if (next?.finished) {
        reload()
        backups.reload()
        if (next.error) toast.error(next.error)
        else toast.success(t("updates.done", { version: next.result?.version }))
      }
    }, 1000)
    return () => clearTimeout(timer)
  }, [job, reload, backups, t])

  async function setPolicy(v) {
    try {
      await api.patchServer(server.id, { GameUpdate: v })
      reload()
    } catch (err) {
      toast.error(err.message)
    }
  }

  const busy = job && !job.finished
  const pct = job?.total ? Math.round((job.done / job.total) * 100) : 0
  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("updates.title", { game: server.gameName })}</CardTitle>
          <CardDescription>{t("updates.text")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <div>
              <dt className="text-xs text-muted-foreground">{t("updates.installed")}</dt>
              <dd className="font-mono text-lg">{server.version || "—"}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">{t("updates.latest")}</dt>
              <dd className="font-mono text-lg">{latest || "—"}</dd>
            </div>
          </dl>
          {urgent ? (
            <Alert variant="destructive">
              <SirenIcon aria-hidden="true" />
              <AlertTitle>{t("updates.urgentTitle", { version: urgent.version })}</AlertTitle>
              <AlertDescription>{urgent.note || t("banner.noNote")}</AlertDescription>
            </Alert>
          ) : outdated ? (
            <Alert>
              <ArrowUpCircleIcon aria-hidden="true" />
              <AlertTitle>{t("updates.availableTitle", { version: latest })}</AlertTitle>
              <AlertDescription>{t("updates.availableText")}</AlertDescription>
            </Alert>
          ) : latest && server.version ? (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <CheckCircle2Icon className="size-4 text-success" aria-hidden="true" />
              {t("updates.upToDate")}
            </p>
          ) : !server.version ? (
            <p className="text-sm text-muted-foreground">{t("updates.unknownVersion")}</p>
          ) : null}
          {busy && (
            <div className="space-y-2" aria-live="polite">
              <Progress value={job.stage === "download" ? pct : 100} />
              <p className="text-sm text-muted-foreground">
                {job.stage === "download" && job.total ? t("add.progress", { done: formatBytes(job.done), total: formatBytes(job.total) }) : t(`updates.stage.${job.stage}`, { defaultValue: job.stage })}
              </p>
            </div>
          )}
        </CardContent>
        <CardFooter className="flex-wrap justify-between gap-3">
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">{t("updates.policy")}</span>
            <Select value={server.gameUpdate || "manual"} onValueChange={setPolicy}>
              <SelectTrigger size="sm" className="w-64" aria-label={t("updates.policy")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent position="popper">
                <SelectItem value="manual">{t("updates.policies.manual")}</SelectItem>
                <SelectItem value="restart">{t("updates.policies.restart")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {latest && (outdated || !server.version) && (
            <ConfirmButton
              disabled={busy}
              title={t("updates.confirmTitle", { version: latest })}
              description={t(running ? "updates.confirmRunning" : "updates.confirmStopped")}
              confirmLabel={t("updates.update")}
              onConfirm={async () => setJob(await api.serverAction(server.id, "update", { Version: latest }))}
            >
              {t("updates.updateTo", { version: latest })}
            </ConfirmButton>
          )}
        </CardFooter>
      </Card>
      {server.game === "arcane-mp" && can("resources.manage") && <FoundationsCard server={server} reload={reload} />}
      {server.gameUpdate === "restart" && !server.restarts?.length && (
        <p className="text-sm text-muted-foreground">
          {t("updates.needsSchedule")}{" "}
          <Link to={`/servers/${server.id}/settings`} className="underline underline-offset-4">
            {t("updates.setSchedule")}
          </Link>
        </p>
      )}
      <section className="space-y-3">
        <div>
          <h2 className="text-sm font-medium">{t("updates.backups")}</h2>
          <p className="text-sm text-muted-foreground">{t("updates.backupsText")}</p>
        </div>
        <Query query={backups}>
          {(list) =>
            !list.length ? (
              <p className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">{t("updates.noBackups")}</p>
            ) : (
              <div className="rounded-xl border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("updates.backupVersion")}</TableHead>
                      <TableHead>{t("updates.backupDate")}</TableHead>
                      <TableHead className="text-right">
                        <span className="sr-only">{t("common.actions")}</span>
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {list.map((b) => (
                      <TableRow key={b.name}>
                        <TableCell className="font-mono">{b.version}</TableCell>
                        <TableCell>{formatDateTime(b.createdAt)}</TableCell>
                        <TableCell className="text-right">
                          <ConfirmButton
                            size="sm"
                            variant="outline"
                            disabled={!stopped || busy}
                            title={t("updates.restoreTitle", { version: b.version })}
                            description={t("updates.restoreText")}
                            confirmLabel={t("updates.restore")}
                            destructive
                            onConfirm={async () => {
                              await api.restoreBackup(server.id, b.name)
                              toast.success(t("updates.restored", { version: b.version }))
                              reload()
                            }}
                          >
                            {t("updates.restore")}
                          </ConfirmButton>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )
          }
        </Query>
        {!stopped && <p className="text-xs text-muted-foreground">{t("updates.stopToRestore")}</p>}
      </section>
    </div>
  )
}
