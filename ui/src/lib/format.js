import i18n from "@/lib/i18n"

const locale = () => (i18n.language?.startsWith("tr") ? "tr-TR" : "en-GB")

export function formatBytes(n) {
  if (!n) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)))
  return `${(n / 1024 ** i).toLocaleString(locale(), { maximumFractionDigits: i > 1 ? 1 : 0 })} ${units[i]}`
}

export function formatDateTime(v) {
  if (!v || String(v).startsWith("0001")) return "—"
  return new Date(v).toLocaleString(locale(), { dateStyle: "medium", timeStyle: "short" })
}

export function formatRelative(v) {
  if (!v || String(v).startsWith("0001")) return "—"
  const diff = (new Date(v).getTime() - Date.now()) / 1000
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: "auto" })
  const abs = Math.abs(diff)
  if (abs < 45) return rtf.format(Math.round(diff), "second")
  if (abs < 2700) return rtf.format(Math.round(diff / 60), "minute")
  if (abs < 79200) return rtf.format(Math.round(diff / 3600), "hour")
  return rtf.format(Math.round(diff / 86400), "day")
}

export function formatDuration(from) {
  if (!from || String(from).startsWith("0001")) return "—"
  let s = Math.max(0, Math.floor((Date.now() - new Date(from).getTime()) / 1000))
  const d = Math.floor(s / 86400)
  s -= d * 86400
  const h = Math.floor(s / 3600)
  s -= h * 3600
  const m = Math.floor(s / 60)
  if (d) return `${d}d ${h}h`
  if (h) return `${h}h ${m}m`
  return `${m}m`
}

export const isZeroTime = (v) => !v || String(v).startsWith("0001")
