import { Link, useOutletContext } from "react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ArrowUpCircleIcon, FolderInputIcon, PlayIcon, PlusIcon, RotateCwIcon, ServerIcon, SirenIcon, SquareIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { EmptyState, PageHeader, Query, StateBadge } from "@/components/common"
import { api } from "@/lib/api"
import { formatBytes, formatDuration } from "@/lib/format"
import { useCan } from "@/lib/session"

export function ServerControls({ server, onChange, size = "sm" }) {
  const { t } = useTranslation()
  const can = useCan()
  if (!can("server.control")) return null
  const state = server.status.state
  const run = async (action) => {
    try {
      await api.serverAction(server.id, action)
      toast.success(t(`server.toast.${action}`, { name: server.name }))
      onChange?.()
    } catch (err) {
      toast.error(err.message)
    }
  }
  const busy = state === "stopping" || state === "updating" || Boolean(server.job)
  if (state === "running" || state === "starting")
    return (
      <>
        <Button variant="outline" size={size} disabled={busy} onClick={() => run("restart")}>
          <RotateCwIcon aria-hidden="true" />
          {t("server.restart")}
        </Button>
        <Button variant="outline" size={size} disabled={busy} onClick={() => run("stop")}>
          <SquareIcon aria-hidden="true" />
          {t("server.stop")}
        </Button>
      </>
    )
  return (
    <Button size={size} disabled={busy} onClick={() => run("start")}>
      <PlayIcon aria-hidden="true" />
      {t("server.start")}
    </Button>
  )
}

function Stat({ label, value }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm font-medium tabular-nums">{value}</dd>
    </div>
  )
}

function ServerCard({ server, onChange }) {
  const { t } = useTranslation()
  const s = server.status
  const up = s.state === "running" || s.state === "starting"
  const latest = server.update?.latest
  const outdated = latest && server.version && latest !== server.version
  return (
    <Card className="gap-4">
      <CardHeader>
        <CardTitle className="truncate">
          <Link to={`/servers/${server.id}`} className="hover:underline">
            {server.name}
          </Link>
        </CardTitle>
        <CardDescription>
          {server.gameName} {server.version ? <span className="font-mono">{server.version}</span> : t("server.versionUnknown")}
        </CardDescription>
        <CardAction>
          <StateBadge state={s.state} />
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        <dl className="grid grid-cols-4 gap-3">
          <Stat label={t("server.uptime")} value={up ? formatDuration(s.startedAt) : "—"} />
          <Stat label="CPU" value={up && s.cpu != null ? `${Math.round(s.cpu)}%` : "—"} />
          <Stat label={t("server.memory")} value={up && s.memBytes ? formatBytes(s.memBytes) : "—"} />
          <Stat label={t("server.port")} value={server.port} />
        </dl>
        {server.update?.urgent ? (
          <Badge variant="destructive" className="gap-1">
            <SirenIcon aria-hidden="true" />
            {t("server.urgentUpdate", { version: server.update.urgent.version })}
          </Badge>
        ) : outdated ? (
          <Badge variant="outline" className="gap-1 font-normal">
            <ArrowUpCircleIcon aria-hidden="true" />
            {t("server.updateAvailable", { version: latest })}
          </Badge>
        ) : null}
        {s.state === "crashed" && s.lastExit && <p className="text-sm text-destructive">{t("server.crashedNote", { exit: s.lastExit })}</p>}
      </CardContent>
      <CardFooter className="mt-auto flex-wrap gap-2">
        <ServerControls server={server} onChange={onChange} />
        <Button variant="ghost" size="sm" className="ml-auto" asChild>
          <Link to={`/servers/${server.id}`}>{t("server.open")}</Link>
        </Button>
      </CardFooter>
    </Card>
  )
}

export default function OverviewPage() {
  const { t } = useTranslation()
  const can = useCan()
  const { servers } = useOutletContext()
  return (
    <div className="space-y-6">
      <PageHeader
        title={t("overview.title")}
        description={t("overview.text")}
        actions={
          can("server.add") && (
            <Button asChild>
              <Link to="/servers/new">
                <PlusIcon aria-hidden="true" />
                {t("nav.addServer")}
              </Link>
            </Button>
          )
        }
      />
      <Query query={servers}>
        {(list) =>
          list.length ? (
            <div className="grid gap-4 md:grid-cols-2">
              {list.map((s) => (
                <ServerCard key={s.id} server={s} onChange={servers.reload} />
              ))}
            </div>
          ) : (
            <EmptyState icon={ServerIcon} title={t("overview.emptyTitle")} description={t(can("server.add") ? "overview.emptyText" : "overview.emptyViewer")}>
              {can("server.add") && (
                <>
                  <Button asChild>
                    <Link to="/servers/new?mode=install">
                      <PlusIcon aria-hidden="true" />
                      {t("add.installTitle")}
                    </Link>
                  </Button>
                  <Button variant="outline" asChild>
                    <Link to="/servers/new?mode=import">
                      <FolderInputIcon aria-hidden="true" />
                      {t("add.importTitle")}
                    </Link>
                  </Button>
                </>
              )}
            </EmptyState>
          )
        }
      </Query>
    </div>
  )
}
