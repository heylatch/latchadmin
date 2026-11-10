import { Navigate } from "react-router"
import { useTranslation } from "react-i18next"
import { HistoryIcon } from "lucide-react"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { EmptyState, PageHeader, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatDateTime } from "@/lib/format"
import { useCan } from "@/lib/session"

export default function ActivityPage() {
  const { t } = useTranslation()
  const can = useCan()
  const audit = useApi(api.audit, [], { poll: 15000 })
  if (!can("admin")) return <Navigate to="/" replace />
  return (
    <div className="space-y-6">
      <PageHeader title={t("nav.activity")} description={t("activity.text")} />
      <Query query={audit}>
        {(list) =>
          !list?.length ? (
            <EmptyState icon={HistoryIcon} title={t("activity.empty")} />
          ) : (
            <div className="rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("activity.when")}</TableHead>
                    <TableHead>{t("activity.who")}</TableHead>
                    <TableHead>{t("activity.what")}</TableHead>
                    <TableHead className="hidden md:table-cell">{t("activity.details")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {list.map((e, i) => (
                    <TableRow key={i}>
                      <TableCell className="whitespace-nowrap text-muted-foreground">{formatDateTime(e.at)}</TableCell>
                      <TableCell className="font-medium">{e.user}</TableCell>
                      <TableCell>
                        {t(`activity.actions.${e.action.replace(".", "_")}`, { defaultValue: e.action })}
                        {e.target && <span className="text-muted-foreground"> · {e.target}</span>}
                      </TableCell>
                      <TableCell className="hidden max-w-sm truncate font-mono text-xs text-muted-foreground md:table-cell">
                        {e.detail ? Object.entries(e.detail).map(([k, v]) => `${k}=${v}`).join("  ") : ""}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )
        }
      </Query>
    </div>
  )
}
