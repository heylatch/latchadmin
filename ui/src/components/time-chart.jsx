import { useId } from "react"
import { useTranslation } from "react-i18next"
import { Area, AreaChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts"
import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent } from "@/components/ui/chart"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { cn } from "@/lib/utils"

// Fixed categorical order: a server keeps its color (by its position in the panel), whatever is filtered.
export const seriesColor = (i) => `var(--series-${(i % 8) + 1})`

export const RANGES = ["1h", "6h", "24h", "7d"]

function tickTime(t, range) {
  const d = new Date(t * 1000)
  if (range === "7d") return d.toLocaleDateString(undefined, { weekday: "short", day: "numeric" })
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
}

const fullTime = (t) => new Date(t * 1000).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })

// TimeChart draws one measure over time. One series: a soft area, no legend (the title names it).
// Several: lines with a legend. Missing values are gaps (server stopped), never zero.
export function TimeChart({ data, series, format, domain, xDomain, range, height = 220, className }) {
  const gid = useId().replace(/:/g, "")
  const config = Object.fromEntries(series.map((s) => [s.key, { label: s.label, color: s.color }]))
  const single = series.length === 1
  // Evenly spaced time ticks across the whole range (recharts only labels where data is otherwise).
  const ticks = xDomain ? Array.from({ length: 6 }, (_, i) => Math.round(xDomain[0] + ((xDomain[1] - xDomain[0]) * i) / 5)) : undefined
  const axis = {
    x: <XAxis dataKey="t" type="number" scale="time" domain={xDomain ?? ["dataMin", "dataMax"]} ticks={ticks} allowDataOverflow tickFormatter={(t) => tickTime(t, range)} tickLine={false} axisLine={false} minTickGap={48} tickMargin={8} fontSize={11} />,
    y: <YAxis tickFormatter={format} domain={domain ?? [0, "auto"]} tickLine={false} axisLine={false} width={68} fontSize={11} allowDecimals={false} />,
    grid: <CartesianGrid vertical={false} strokeDasharray="3 3" className="stroke-border/60" />,
    tip: (
      <ChartTooltip
        cursor={{ strokeWidth: 1 }}
        content={<ChartTooltipContent labelFormatter={(_, p) => (p?.[0] ? fullTime(p[0].payload.t) : "")} formatter={(v, name, item) => <TooltipRow value={v} label={config[name]?.label ?? name} color={item.color} format={format} />} />}
      />
    ),
  }
  return (
    <ChartContainer config={config} className={cn("aspect-auto w-full", className)} style={{ height }}>
      {single ? (
        <AreaChart data={data} margin={{ left: 0, right: 8, top: 8 }}>
          <defs>
            <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={`var(--color-${series[0].key})`} stopOpacity={0.25} />
              <stop offset="100%" stopColor={`var(--color-${series[0].key})`} stopOpacity={0.02} />
            </linearGradient>
          </defs>
          {axis.grid}
          {axis.x}
          {axis.y}
          {axis.tip}
          <Area dataKey={series[0].key} type="monotone" stroke={`var(--color-${series[0].key})`} strokeWidth={2} fill={`url(#${gid})`} connectNulls={false} isAnimationActive={false} dot={false} activeDot={{ r: 4, strokeWidth: 2 }} />
        </AreaChart>
      ) : (
        <LineChart data={data} margin={{ left: 0, right: 8, top: 8 }}>
          {axis.grid}
          {axis.x}
          {axis.y}
          {axis.tip}
          <ChartLegend content={<ChartLegendContent />} />
          {series.map((s) => (
            <Line key={s.key} dataKey={s.key} type="monotone" stroke={`var(--color-${s.key})`} strokeWidth={2} dot={false} activeDot={{ r: 4, strokeWidth: 2 }} connectNulls={false} isAnimationActive={false} />
          ))}
        </LineChart>
      )}
    </ChartContainer>
  )
}

function TooltipRow({ value, label, color, format }) {
  return (
    <div className="flex w-full items-center gap-2">
      <span className="size-2.5 shrink-0 rounded-[2px]" style={{ background: color }} aria-hidden="true" />
      <span className="text-muted-foreground">{label}</span>
      <span className="ml-auto font-mono font-medium text-foreground tabular-nums">{value == null ? "—" : format(value)}</span>
    </div>
  )
}

// ChartCard frames a chart with a title, a one-line description and an empty state.
export function ChartCard({ title, description, action, empty, children, className }) {
  const { t } = useTranslation()
  return (
    <Card className={cn("gap-4", className)}>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
        {action && <CardAction>{action}</CardAction>}
      </CardHeader>
      <CardContent>{empty ? <p className="flex h-[220px] items-center justify-center rounded-lg border border-dashed text-sm text-muted-foreground">{t("stats.noData")}</p> : children}</CardContent>
    </Card>
  )
}

// merge turns per-series point lists into chart rows keyed by time.
export function merge(seriesList, pick) {
  const rows = new Map()
  for (const { key, points } of seriesList) {
    for (const p of points) {
      const row = rows.get(p.t) ?? { t: p.t }
      row[key] = pick(p)
      rows.set(p.t, row)
    }
  }
  return [...rows.values()].sort((a, b) => a.t - b.t)
}

export const hasValues = (rows, keys) => rows.some((r) => keys.some((k) => r[k] != null))
