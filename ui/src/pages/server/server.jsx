import { Link, Navigate, useOutletContext, useParams } from "react-router"
import { useTranslation } from "react-i18next"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { PageHeader, Query, StateBadge } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { useCan } from "@/lib/session"
import { ServerControls } from "@/pages/overview"
import ConfigTab from "@/pages/server/config"
import ConsoleTab from "@/pages/server/console"
import ResourcesTab from "@/pages/server/resources"
import ScriptsTab from "@/pages/server/scripts"
import SettingsTab from "@/pages/server/settings"
import UpdatesTab from "@/pages/server/updates"

const TABS = [
  { id: "console", role: "viewer", el: ConsoleTab },
  { id: "resources", role: "viewer", el: ResourcesTab },
  { id: "scripts", role: "viewer", el: ScriptsTab },
  { id: "config", role: "admin", el: ConfigTab },
  { id: "updates", role: "admin", el: UpdatesTab },
  { id: "settings", role: "admin", el: SettingsTab },
]

export default function ServerPage() {
  const { t } = useTranslation()
  const can = useCan()
  const { id, tab = "console" } = useParams()
  const { servers } = useOutletContext()
  const query = useApi(() => api.server(id), [id], { poll: 4000 })
  const tabs = TABS.filter((x) => can(x.role))
  const current = tabs.find((x) => x.id === tab)
  if (!current) return <Navigate to={`/servers/${id}`} replace />
  const reload = () => (query.reload(), servers.reload())

  return (
    <Query query={query}>
      {(server) => (
        <div className="space-y-6">
          <PageHeader
            title={
              <span className="flex flex-wrap items-center gap-3">
                {server.name}
                <StateBadge state={server.status.state} />
              </span>
            }
            description={
              <>
                {server.gameName} {server.version ? <span className="font-mono">{server.version}</span> : t("server.versionUnknown")} ·{" "}
                <span className="font-mono break-all">{server.dir}</span>
              </>
            }
            actions={<ServerControls server={server} onChange={reload} />}
          />
          <Tabs value={tab}>
            <TabsList className="h-auto flex-wrap">
              {tabs.map((x) => (
                <TabsTrigger key={x.id} value={x.id} asChild>
                  <Link to={x.id === "console" ? `/servers/${id}` : `/servers/${id}/${x.id}`}>{t(`server.tabs.${x.id}`)}</Link>
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
