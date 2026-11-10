import { useState } from "react"
import { Link } from "react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { KeyRoundIcon, LinkIcon, TriangleAlertIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { EmptyState, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { useCan } from "@/lib/session"

function cmp(a, b) {
  const pa = String(a).split(".").map(Number)
  const pb = String(b).split(".").map(Number)
  for (let i = 0; i < 3; i++) if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) - (pb[i] || 0)
  return 0
}

export default function ScriptsTab({ server }) {
  const { t } = useTranslation()
  const can = useCan()
  const latch = useApi(api.latch)
  const library = useApi(() => (latch.data?.linked && !latch.data.disconnected ? api.library() : Promise.resolve(null)), [latch.data?.linked, latch.data?.disconnected])
  const [busy, setBusy] = useState(null)
  const running = server.status.state === "running"
  const hasLicense = server.env?.some((e) => (e.key === "LATCH_LICENSE" || e.key === "LATCH_KEY") && e.set)

  if (latch.data && (!latch.data.linked || latch.data.disconnected))
    return (
      <EmptyState icon={LinkIcon} title={t(latch.data.disconnected ? "scripts.disconnectedTitle" : "scripts.unlinkedTitle")} description={t("scripts.unlinkedText")}>
        <Button asChild>
          <Link to="/latch">{t("scripts.goLink")}</Link>
        </Button>
      </EmptyState>
    )

  async function install(row, restart) {
    setBusy(row.item.script.id)
    try {
      const res = await api.installScript(server.id, row.item.script.id, restart)
      toast.success(res.upToDate ? t("scripts.upToDate", { name: row.item.script.name }) : t("scripts.installedToast", { name: row.item.script.name, version: res.version }))
      library.reload()
    } catch (err) {
      toast.error(err.message)
    } finally {
      setBusy(null)
    }
  }

  async function setPolicy(row, policy) {
    try {
      await api.scriptPolicy(server.id, row.item.script.id, policy)
      library.reload()
    } catch (err) {
      toast.error(err.message)
    }
  }

  return (
    <div className="space-y-4">
      {can("config.edit") && server.env && !hasLicense && (
        <Alert>
          <KeyRoundIcon aria-hidden="true" />
          <AlertTitle>{t("scripts.noLicenseTitle")}</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>
              {t("scripts.noLicenseText")}{" "}
              <a href="https://heylatch.app/app/keys" target="_blank" rel="noreferrer" className="underline underline-offset-4">
                heylatch.app/app/keys
              </a>
            </span>
            <Button size="sm" variant="outline" asChild>
              <Link to={`/servers/${server.id}/config`}>{t("scripts.addLicense")}</Link>
            </Button>
          </AlertDescription>
        </Alert>
      )}
      <Query query={library}>
        {(rows) =>
          !rows?.length ? (
            <EmptyState icon={KeyRoundIcon} title={t("scripts.emptyTitle")} description={t("scripts.emptyText")} />
          ) : (
            <div className="rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("scripts.script")}</TableHead>
                    <TableHead>{t("scripts.installed")}</TableHead>
                    <TableHead className="hidden sm:table-cell">{t("scripts.latest")}</TableHead>
                    <TableHead className="hidden md:table-cell">{t("scripts.policy")}</TableHead>
                    <TableHead className="text-right">
                      <span className="sr-only">{t("common.actions")}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((row) => {
                    const it = row.item
                    const here = row.servers[server.id] ?? {}
                    const latest = it.script.latestVersion
                    const usable = (it.status === "active" || it.own) && latest
                    const outdated = here.installed && latest && cmp(latest, here.installed) > 0
                    return (
                      <TableRow key={it.script.id}>
                        <TableCell>
                          <p className="font-medium">{it.script.name}</p>
                          <p className="text-xs text-muted-foreground">{it.workspace.name}</p>
                        </TableCell>
                        <TableCell>
                          {here.installed ? (
                            <span className="inline-flex items-center gap-2">
                              <span className="font-mono text-sm">{here.installed}</span>
                              {!here.enabled && <Badge variant="outline">{t("resources.disabled")}</Badge>}
                            </span>
                          ) : (
                            <span className="text-muted-foreground">{t("scripts.notInstalled")}</span>
                          )}
                        </TableCell>
                        <TableCell className="hidden font-mono text-sm sm:table-cell">{latest ?? "—"}</TableCell>
                        <TableCell className="hidden md:table-cell">
                          <Select value={here.policy ?? "manual"} onValueChange={(v) => setPolicy(row, v)} disabled={!can("updates.scripts") || !here.installed}>
                            <SelectTrigger size="sm" className="w-48" aria-label={t("scripts.policy")}>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent position="popper">
                              {["manual", "restart", "now"].map((p) => (
                                <SelectItem key={p} value={p}>
                                  {t(`scripts.policies.${p}`)}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </TableCell>
                        <TableCell className="text-right">
                          {!usable ? (
                            <span className="text-sm text-muted-foreground">{t(it.status === "revoked" ? "scripts.revoked" : "scripts.unpublished")}</span>
                          ) : can("updates.scripts") && (!here.installed || outdated) ? (
                            <Button size="sm" variant={outdated ? "default" : "outline"} disabled={busy === it.script.id} onClick={() => install(row, running && outdated)}>
                              {busy === it.script.id && <Spinner />}
                              {outdated ? t("scripts.update") : t("scripts.install")}
                            </Button>
                          ) : here.installed ? (
                            <span className="text-sm text-muted-foreground">{t("scripts.current")}</span>
                          ) : null}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )
        }
      </Query>
      <div className="space-y-1 text-xs text-muted-foreground">
        <p>{t("scripts.policyHelp")}</p>
        {running && <p className="flex items-center gap-1.5"><TriangleAlertIcon className="size-3.5" aria-hidden="true" />{t("scripts.liveHelp")}</p>}
      </div>
    </div>
  )
}
