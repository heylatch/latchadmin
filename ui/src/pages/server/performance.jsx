import { useState } from "react"
import { useTranslation } from "react-i18next"
import { CircleStopIcon, CpuIcon, FlameIcon, MemoryStickIcon, PlayIcon, RadioIcon, TimerIcon, UsersIcon } from "lucide-react"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Query } from "@/components/common"
import { ChartCard, hasValues, TimeChart } from "@/components/time-chart"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatDateTime, formatRelative } from "@/lib/format"
import { cn } from "@/lib/utils"
import { gb, pct, RangePicker, Tile, uptimeText, whole } from "@/pages/statistics"

const eventIcon = { start: PlayIcon, ready: RadioIcon, stop: CircleStopIcon, crash: FlameIcon }

function Events({ events }) {
  const { t } = useTranslation()
  return (
    <Card className="gap-4">
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("perf.eventsTitle")}</CardTitle>
        <CardDescription>{t("perf.eventsText")}</CardDescription>
      </CardHeader>
      <CardContent>
        {events.length ? (
          <ol className="max-h-80 space-y-0 overflow-y-auto">
            {events.map((e, i) => {
              const Icon = eventIcon[e.kind] ?? RadioIcon
              return (
                <li key={i} className="flex items-start gap-3 border-b py-2 last:border-0">
                  <span className={cn("mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md bg-muted", e.kind === "crash" && "bg-destructive/15 text-destructive")}>
                    <Icon className="size-3.5" aria-hidden="true" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm">
                      {t(`perf.events.${e.kind}`, { defaultValue: e.kind })}
                      {e.detail && <span className="text-muted-foreground"> · {e.detail}</span>}
                    </p>
                    <p className="text-xs text-muted-foreground" title={formatDateTime(e.t * 1000)}>
                      {formatRelative(e.t * 1000)}
                    </p>
                  </div>
                </li>
              )
            })}
          </ol>
        ) : (
          <p className="text-sm text-muted-foreground">{t("perf.noEvents")}</p>
        )}
      </CardContent>
    </Card>
  )
}

export default function PerformanceTab({ server }) {
  const { t } = useTranslation()
  const [range, setRange] = useState("24h")
  const stats = useApi(() => api.serverStats(server.id, range), [server.id, range], { poll: 30000 })

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">{t("perf.text")}</p>
        <RangePicker value={range} onChange={setRange} />
      </div>
      <Query query={stats} rows={3}>
        {(d) => {
          const s = d.summary
          const xd = [d.from, Math.floor(Date.now() / 1000)]
          const players = d.points.map((p) => ({ t: p.t, players: p.players }))
          const cpu = d.points.map((p) => ({ t: p.t, cpu: p.cpu }))
          const mem = d.points.map((p) => ({ t: p.t, mem: p.mem }))
          return (
            <>
              <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
                <Tile icon={TimerIcon} label={t("stats.uptime")} value={uptimeText(s.uptime)} hint={t("perf.uptimeHint")} />
                <Tile icon={UsersIcon} label={t("stats.peakPlayers")} value={s.peakPlayers ?? "—"} hint={s.avgPlayers != null ? t("perf.avgPlayers", { value: s.avgPlayers }) : ""} />
                <Tile icon={CpuIcon} label={t("stats.avgCpu")} value={s.avgCpu != null ? pct(s.avgCpu) : "—"} hint={s.peakCpu != null ? t("perf.peak", { value: pct(s.peakCpu) }) : ""} />
                <Tile icon={MemoryStickIcon} label={t("stats.peakMemory")} value={s.peakMem != null ? gb(s.peakMem) : "—"} hint={s.avgMem != null ? t("perf.avg", { value: gb(s.avgMem) }) : ""} />
                <Tile icon={FlameIcon} label={t("stats.crashes")} value={s.crashes} hint={t("perf.starts", { count: s.starts })} />
              </div>
              <ChartCard title={t("stats.players")} description={t("perf.playersText")} empty={!hasValues(players, ["players"])}>
                <TimeChart data={players} series={[{ key: "players", label: t("stats.players"), color: "var(--series-1)" }]} format={whole} domain={[0, server.status.maxPlayers || "auto"]} range={range} xDomain={xd} height={240} />
              </ChartCard>
              <div className="grid gap-4 lg:grid-cols-2">
                <ChartCard title="CPU" description={t("perf.cpuText")} empty={!hasValues(cpu, ["cpu"])}>
                  <TimeChart data={cpu} series={[{ key: "cpu", label: "CPU", color: "var(--series-3)" }]} format={pct} range={range} xDomain={xd} />
                </ChartCard>
                <ChartCard title={t("server.memory")} description={t("perf.memText")} empty={!hasValues(mem, ["mem"])}>
                  <TimeChart data={mem} series={[{ key: "mem", label: t("server.memory"), color: "var(--series-7)" }]} format={gb} range={range} xDomain={xd} />
                </ChartCard>
              </div>
              <Events events={d.events} />
            </>
          )
        }}
      </Query>
    </div>
  )
}
