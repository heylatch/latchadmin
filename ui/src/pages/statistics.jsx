import { useState } from "react"
import { Link } from "react-router"
import { useTranslation } from "react-i18next"
import { CpuIcon, HardDriveIcon, MemoryStickIcon, ServerIcon, UsersIcon } from "lucide-react"
import { Card, CardContent } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { EmptyState, PageHeader, Query } from "@/components/common"
import { ChartCard, hasValues, merge, RANGES, seriesColor, TimeChart } from "@/components/time-chart"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { cn } from "@/lib/utils"

export const pct = (v) => `${Math.round(v)}%`
export const gb = (v) => formatBytes(v)
export const whole = (v) => String(Math.round(v))

export function RangePicker({ value, onChange }) {
  const { t } = useTranslation()
  return (
    <ToggleGroup type="single" variant="outline" size="sm" value={value} onValueChange={(v) => v && onChange(v)} aria-label={t("stats.range")}>
      {RANGES.map((r) => (
        <ToggleGroupItem key={r} value={r} className="px-3">
          {t(`stats.ranges.${r}`)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}

// Tile: one headline number. Side-by-side tiles keep the same height (one-line hint, meter on the edge).
export function Tile({ icon: Icon, label, value, hint, meter }) {
  return (
    <Card className="relative gap-0 overflow-hidden py-4">
      <CardContent className="px-4">
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          {Icon && <Icon className="size-4" aria-hidden="true" />}
          {label}
        </p>
        <p className="mt-1.5 text-2xl font-semibold tracking-tight tabular-nums">{value}</p>
        <p className="mt-0.5 truncate text-xs text-muted-foreground">{hint || " "}</p>
      </CardContent>
      {meter != null && (
        <div role="progressbar" aria-valuenow={Math.round(meter)} aria-valuemin={0} aria-valuemax={100} aria-label={label} className="absolute inset-x-0 bottom-0 h-1 bg-muted">
          <div className={cn("h-full bg-foreground/70", meter >= 90 && "bg-destructive")} style={{ width: `${Math.min(100, meter)}%` }} />
        </div>
      )}
    </Card>
  )
}

export const uptimeText = (u) => (u == null ? "—" : `${(u * 100).toFixed(u >= 0.9995 ? 0 : 1)}%`)

export default function StatisticsPage() {
  const { t } = useTranslation()
  const [range, setRange] = useState("24h")
  const stats = useApi(() => api.stats(range), [range], { poll: 30000 })

  return (
    <div className="space-y-6">
      <PageHeader title={t("nav.statistics")} description={t("stats.text")} actions={<RangePicker value={range} onChange={setRange} />} />
      <Query query={stats} rows={4}>
        {(d) => {
          const servers = d.servers
          const host = d.host
          const last = host.points.findLast((p) => p.cpu != null)
          const running = servers.filter((s) => s.status.state === "running")
          const playersNow = running.reduce((n, s) => n + (s.status.players ?? 0), 0)
          const slots = running.reduce((n, s) => n + (s.status.maxPlayers ?? 0), 0)
          const key = (s) => `s_${s.id}`
          const series = servers.map((s, i) => ({ key: key(s), label: s.name, color: seriesColor(i) }))
          const lists = servers.map((s) => ({ key: key(s), points: s.points }))
          const players = merge(lists, (p) => p.players)
          const cpu = merge(lists, (p) => p.cpu)
          const mem = merge(lists, (p) => p.mem)
          const hostCpu = host.points.map((p) => ({ t: p.t, cpu: p.cpu }))
          const hostMem = host.points.map((p) => ({ t: p.t, mem: p.mem }))
          const keys = series.map((s) => s.key)
          const memTotal = host.info.memTotal
          const xd = [d.from, Math.floor(Date.now() / 1000)]

          return (
            <>
              <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
                <Tile icon={UsersIcon} label={t("stats.playersOnline")} value={playersNow} hint={slots ? t("stats.ofSlots", { count: slots }) : t("stats.noRunning")} />
                <Tile icon={ServerIcon} label={t("stats.serversRunning")} value={`${running.length}/${servers.length}`} hint={t("stats.crashesInRange", { count: servers.reduce((n, s) => n + s.summary.crashes, 0) })} />
                <Tile icon={CpuIcon} label={t("stats.hostCpu")} value={last?.cpu != null ? pct(last.cpu) : "—"} hint={t("stats.cores", { count: host.info.cpus })} meter={last?.cpu} />
                <Tile
                  icon={MemoryStickIcon}
                  label={t("stats.hostMemory")}
                  value={last?.mem != null ? gb(last.mem) : "—"}
                  hint={memTotal ? t("stats.ofTotal", { total: gb(memTotal) }) : ""}
                  meter={last?.mem != null && memTotal ? (last.mem / memTotal) * 100 : null}
                />
              </div>
              {!servers.length ? (
                <EmptyState icon={ServerIcon} title={t("overview.emptyTitle")} description={t("stats.noServers")} />
              ) : (
                <ChartCard title={t("stats.players")} description={t("stats.playersText")} empty={!hasValues(players, keys)}>
                  <TimeChart data={players} series={series} format={whole} range={range} xDomain={xd} height={260} />
                </ChartCard>
              )}
              <div className="grid gap-4 lg:grid-cols-2">
                <ChartCard title={t("stats.hostCpu")} description={t("stats.hostCpuText")} empty={!hasValues(hostCpu, ["cpu"])}>
                  <TimeChart data={hostCpu} series={[{ key: "cpu", label: t("stats.hostCpu"), color: "var(--series-1)" }]} format={pct} domain={[0, 100]} range={range} xDomain={xd} />
                </ChartCard>
                <ChartCard title={t("stats.hostMemory")} description={memTotal ? t("stats.hostMemoryText", { total: gb(memTotal) }) : ""} empty={!hasValues(hostMem, ["mem"])}>
                  <TimeChart data={hostMem} series={[{ key: "mem", label: t("stats.hostMemory"), color: "var(--series-7)" }]} format={gb} domain={[0, memTotal || "auto"]} range={range} xDomain={xd} />
                </ChartCard>
                {servers.length > 0 && (
                  <>
                    <ChartCard title={t("stats.serverCpu")} description={t("stats.serverCpuText")} empty={!hasValues(cpu, keys)}>
                      <TimeChart data={cpu} series={series} format={pct} range={range} xDomain={xd} />
                    </ChartCard>
                    <ChartCard title={t("stats.serverMemory")} description={t("stats.serverMemoryText")} empty={!hasValues(mem, keys)}>
                      <TimeChart data={mem} series={series} format={gb} range={range} xDomain={xd} />
                    </ChartCard>
                  </>
                )}
              </div>
              {servers.length > 0 && (
                <section className="space-y-3">
                  <div>
                    <h2 className="text-sm font-medium">{t("stats.summaryTitle")}</h2>
                    <p className="text-sm text-muted-foreground">{t("stats.summaryText", { range: t(`stats.ranges.${range}`) })}</p>
                  </div>
                  <div className="rounded-xl border">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t("stats.server")}</TableHead>
                          <TableHead className="text-right">{t("stats.uptime")}</TableHead>
                          <TableHead className="text-right">{t("stats.avgPlayers")}</TableHead>
                          <TableHead className="text-right">{t("stats.peakPlayers")}</TableHead>
                          <TableHead className="hidden text-right md:table-cell">{t("stats.avgCpu")}</TableHead>
                          <TableHead className="hidden text-right md:table-cell">{t("stats.peakMemory")}</TableHead>
                          <TableHead className="text-right">{t("stats.starts")}</TableHead>
                          <TableHead className="text-right">{t("stats.crashes")}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {servers.map((s, i) => (
                          <TableRow key={s.id}>
                            <TableCell>
                              <Link to={`/servers/${s.id}/performance`} className="inline-flex items-center gap-2 font-medium hover:underline">
                                <span className="size-2.5 rounded-[2px]" style={{ background: seriesColor(i) }} aria-hidden="true" />
                                {s.name}
                              </Link>
                            </TableCell>
                            <TableCell className="text-right tabular-nums">{uptimeText(s.summary.uptime)}</TableCell>
                            <TableCell className="text-right tabular-nums">{s.summary.avgPlayers ?? "—"}</TableCell>
                            <TableCell className="text-right tabular-nums">{s.summary.peakPlayers ?? "—"}</TableCell>
                            <TableCell className="hidden text-right tabular-nums md:table-cell">{s.summary.avgCpu != null ? pct(s.summary.avgCpu) : "—"}</TableCell>
                            <TableCell className="hidden text-right tabular-nums md:table-cell">{s.summary.peakMem != null ? gb(s.summary.peakMem) : "—"}</TableCell>
                            <TableCell className="text-right tabular-nums">{s.summary.starts}</TableCell>
                            <TableCell className={cn("text-right tabular-nums", s.summary.crashes > 0 && "font-medium text-destructive")}>{s.summary.crashes}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                </section>
              )}
              {host.info.diskTotal > 0 && (
                <p className="flex items-center gap-2 text-xs text-muted-foreground">
                  <HardDriveIcon className="size-3.5" aria-hidden="true" />
                  {t("stats.disk", { free: gb(host.info.diskFree), total: gb(host.info.diskTotal) })}
                </p>
              )}
            </>
          )
        }}
      </Query>
    </div>
  )
}
