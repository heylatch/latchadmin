import { Link, Navigate, useOutletContext, useParams } from "react-router"
import { useTranslation } from "react-i18next"
import { ArrowUpCircleIcon, CpuIcon, FolderIcon, MemoryStickIcon, NetworkIcon, PackageIcon, SirenIcon, TimerIcon, UsersIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Card } from "@/components/ui/card"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Query, StateBadge } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatBytes, formatDuration } from "@/lib/format"
import { useCan } from "@/lib/session"
import { cn } from "@/lib/utils"
import { ServerControls } from "@/pages/overview"
import ConfigTab from "@/pages/server/config"
import ConsoleTab from "@/pages/server/console"
import PerformanceTab from "@/pages/server/performance"
import ResourcesTab from "@/pages/server/resources"
import ScriptsTab from "@/pages/server/scripts"
import SettingsTab from "@/pages/server/settings"
import UpdatesTab from "@/pages/server/updates"

const TABS = [
  { id: "console", perm: "console.view", el: ConsoleTab },
  { id: "performance", el: PerformanceTab },
  { id: "resources", el: ResourcesTab },
  { id: "scripts", el: ScriptsTab },
  { id: "config", perm: "config.view", el: ConfigTab },
  { id: "updates", perm: "updates.game", el: UpdatesTab },
  { id: "settings", perm: "server.settings", el: SettingsTab },
]

function Stat({ icon: Icon, label, children, hint, className }) {
  return (
    <div className={cn("min-w-0 px-4 py-3", className)}>
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <Icon className="size-3.5" aria-hidden="true" />
        {label}
      </p>
      <div className="mt-1 truncate text-lg font-semibold tracking-tight tabular-nums">{children}</div>
      <p className="truncate text-xs text-muted-foreground">{hint || " "}</p>
    </div>
  )
}

function StatusStrip({ server }) {
  const { t } = useTranslation()
  const s = server.status
  const up = s.state === "running" || s.state === "starting" || s.state === "stopping"
  const latest = server.update?.latest
  const outdated = latest && server.version && latest !== server.version
  const players = s.players
  const max = s.maxPlayers || s.health?.maxPlayers
  return (
    <Card className="grid grid-cols-2 gap-0 divide-y p-0 sm:grid-cols-3 sm:divide-y-0 lg:grid-cols-6 lg:divide-x [&>*]:border-border">
      <Stat icon={UsersIcon} label={t("server.players")} hint={up && max ? t("server.slots", { count: max }) : ""}>
        {up && players != null ? (
          <>
            {players}
            <span className="text-sm font-normal text-muted-foreground">/{max ?? "?"}</span>
          </>
        ) : (
          "—"
        )}
      </Stat>
      <Stat icon={TimerIcon} label={t("server.uptime")} hint={!up && s.lastExit ? s.lastExit : up && s.crashes ? t("server.crashesSinceStart", { count: s.crashes }) : ""}>
        {up ? formatDuration(s.startedAt) : t(`state.${s.state}`)}
      </Stat>
      <Stat icon={CpuIcon} label="CPU" hint={t("server.cpuHint")}>
        {up && s.cpu != null ? `${Math.round(s.cpu)}%` : "—"}
      </Stat>
      <Stat icon={MemoryStickIcon} label={t("server.memory")}>
        {up && s.memBytes ? formatBytes(s.memBytes) : "—"}
      </Stat>
      <Stat icon={NetworkIcon} label={t("server.port")} hint={t("server.portHint")}>
        {server.port}
      </Stat>
      <Stat icon={PackageIcon} label={t("server.version")} hint={server.gameName}>
        <span className="inline-flex items-center gap-2">
          {server.version || "—"}
          {server.update?.urgent ? (
            <Badge variant="destructive" className="gap-1 text-[11px]">
              <SirenIcon aria-hidden="true" />
              {server.update.urgent.version}
            </Badge>
          ) : outdated ? (
            <Badge variant="outline" className="gap-1 text-[11px] font-normal">
              <ArrowUpCircleIcon aria-hidden="true" />
              {latest}
            </Badge>
          ) : null}
        </span>
      </Stat>
    </Card>
  )
}

export default function ServerPage() {
  const { t } = useTranslation()
  const can = useCan()
  const { id, tab = "console" } = useParams()
  const { servers } = useOutletContext()
  const query = useApi(() => api.server(id), [id], { poll: 4000 })
  const tabs = TABS.filter((x) => !x.perm || can(x.perm))
  const current = tabs.find((x) => x.id === tab) ?? (tab === "console" ? tabs[0] : null)
  if (!current) return <Navigate to={`/servers/${id}`} replace />
  const reload = () => (query.reload(), servers.reload())

  return (
    <Query query={query}>
      {(server) => (
        <div className="space-y-6">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0 space-y-1">
              <h1 className="flex flex-wrap items-center gap-3 text-2xl font-semibold tracking-tight break-words">
                {server.name}
                <StateBadge state={server.status.state} />
              </h1>
              <p className="flex min-w-0 items-center gap-1.5 text-sm text-muted-foreground">
                <FolderIcon className="size-3.5 shrink-0" aria-hidden="true" />
                <span className="truncate font-mono text-xs" title={server.dir}>
                  {server.dir}
                </span>
              </p>
            </div>
            <div className="flex shrink-0 flex-wrap items-center gap-2">
              <ServerControls server={server} onChange={reload} />
            </div>
          </div>
          <StatusStrip server={server} />
          <Tabs value={current.id}>
            <TabsList className="h-auto flex-wrap">
              {tabs.map((x) => (
                <TabsTrigger key={x.id} value={x.id} asChild>
                  <Link to={x.id === tabs[0].id ? `/servers/${id}` : `/servers/${id}/${x.id}`}>{t(`server.tabs.${x.id}`)}</Link>
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <current.el server={server} reload={reload} />
        </div>
      )}
    </Query>
  )
}
