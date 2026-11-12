import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { ChartCard, seriesColor, TimeChart } from "@/components/time-chart"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { cn } from "@/lib/utils"

const ms = (v) => (v == null ? "—" : `${v < 10 ? v.toFixed(2) : v.toFixed(1)} ms`)

// ResourcesPerf: what each resource costs, from the server's own "resmon" (read once a minute, last six hours).
export function ResourcesPerf({ server }) {
  const { t } = useTranslation()
  const [range, setRange] = useState("1h")
  const q = useApi(() => api.resmon(server.id, range), [server.id, range], { poll: 60000 })
  const d = q.data
  if (!d || (!d.supported && !d.latest.length)) return null

  // Resource names become chart keys; they're mapped to r0..r5 because names may hold characters CSS can't.
  const keys = d.top.map((name, i) => ({ name, key: `r${i}` }))
  const points = d.points.map((p) => Object.fromEntries([["t", p.t], ...keys.map((k) => [k.key, p[k.name] ?? null])]))
  const series = keys.map((k, i) => ({ key: k.key, label: k.name, color: seriesColor(i) }))
  const now = Math.floor(Date.now() / 1000)
  const xd = [now - (range === "6h" ? 6 * 3600 : 3600), now]
  const heaviest = d.latest[0]?.msPerTick ?? 0

  return (
    <ChartCard
      title={t("resmon.title")}
      description={t("resmon.text")}
      empty={!d.latest.length}
      action={
        <ToggleGroup type="single" variant="outline" size="sm" value={range} onValueChange={(v) => v && setRange(v)}>
          <ToggleGroupItem value="1h">{t("resmon.range1h")}</ToggleGroupItem>
          <ToggleGroupItem value="6h">{t("resmon.range6h")}</ToggleGroupItem>
        </ToggleGroup>
      }
    >
      <div className="space-y-5">
        {points.length > 1 ? (
          <TimeChart data={points} series={series} format={ms} range="1h" xDomain={xd} height={220} decimals />
        ) : (
          <p className="text-sm text-muted-foreground">{t("resmon.collecting")}</p>
        )}
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("resmon.resource")}</TableHead>
                <TableHead className="text-right">{t("resmon.perTick")}</TableHead>
                <TableHead className="text-right">{t("resmon.max")}</TableHead>
                <TableHead className="text-right">{t("resmon.heap")}</TableHead>
                <TableHead className="text-right">{t("resmon.gc")}</TableHead>
                <TableHead className="text-right">{t("resmon.slow")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {d.latest.slice(0, 15).map((r) => (
                <TableRow key={r.name}>
                  <TableCell className="font-mono text-xs">
                    <div className="flex items-center justify-between gap-3">
                      <span className="truncate">{r.name}</span>
                      {/* a small bar for the share of the heaviest resource */}
                      <span className="hidden h-1 w-16 shrink-0 overflow-hidden rounded-full bg-muted sm:block" aria-hidden="true">
                        <span className="block h-full rounded-full bg-foreground/50" style={{ width: `${heaviest ? Math.max(4, (r.msPerTick / heaviest) * 100) : 0}%` }} />
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">{ms(r.msPerTick)}</TableCell>
                  <TableCell className="text-right tabular-nums">{ms(r.maxMs)}</TableCell>
                  <TableCell className="text-right tabular-nums">{r.heapMb.toFixed(1)} MB</TableCell>
                  <TableCell className="text-right tabular-nums">{r.gcMs.toFixed(2)}</TableCell>
                  <TableCell className={cn("text-right tabular-nums", r.slow > 0 && "font-medium text-warning")}>{r.slow}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        <p className="text-xs text-muted-foreground">{t("resmon.help")}</p>
      </div>
    </ChartCard>
  )
}
