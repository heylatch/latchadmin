import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { CircleAlertIcon, MegaphoneIcon } from "lucide-react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { ErrorNote } from "@/components/common"
import { api } from "@/lib/api"
import { useSession } from "@/lib/session"

// NoticeCard draws a notice the way the latchadmin resource shows it in game (internal/bridge/resource/ui), so the
// preview matches what players see. It's dark on purpose: it sits on the game, not on the panel.
export function NoticeCard({ label, title, text }) {
  return (
    <div className="relative grid grid-cols-[auto_1fr] gap-3.5 overflow-hidden rounded-2xl border border-white/10 bg-[rgba(16,16,19,0.92)] p-4 pb-[18px] text-[#fafafa] shadow-xl">
      <span className="flex size-9.5 items-center justify-center rounded-[11px] bg-white/8">
        <MegaphoneIcon className="size-[19px]" aria-hidden="true" />
      </span>
      <div className="min-w-0">
        <p className="text-[11px] font-semibold tracking-[0.08em] text-white/50 uppercase [overflow-wrap:anywhere]">{label}</p>
        {title && <p className="mt-0.5 text-[15.5px] leading-snug font-semibold [overflow-wrap:anywhere]">{title}</p>}
        {text && <p className="mt-1 text-[13.5px] leading-normal whitespace-pre-line text-white/75 [overflow-wrap:anywhere]">{text}</p>}
      </div>
      <span className="absolute inset-x-0 bottom-0 h-[3px] bg-white/6">
        <span className="block h-full w-2/3 bg-white/35" />
      </span>
    </div>
  )
}

export function AnnounceDialog({ server, open, onOpenChange }) {
  const { t } = useTranslation()
  const { user } = useSession()
  const [form, setForm] = useState({ title: "", text: "", showName: false })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))
  const running = server.status.state === "running"
  const n = server.notices ?? {}
  const label = form.showName ? t("announce.from", { name: user?.username }) : t("announce.label")
  const blocked = !n.enabled ? t("announce.off") : !running ? t("announce.notRunning") : null

  async function submit(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      const res = await api.announce(server.id, { Title: form.title, Text: form.text, ShowName: form.showName })
      if (res.connected) toast.success(t("announce.sent"))
      else toast.warning(t("announce.queued"))
      setForm({ title: "", text: "", showName: form.showName })
      onOpenChange(false)
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle>{t("announce.title")}</DialogTitle>
            <DialogDescription>{t("announce.text", { server: server.name })}</DialogDescription>
          </DialogHeader>
          {blocked && (
            <Alert>
              <CircleAlertIcon aria-hidden="true" />
              <AlertDescription>{blocked}</AlertDescription>
            </Alert>
          )}
          <div className="grid gap-5 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="grid content-start gap-4">
              <Field>
                <FieldLabel htmlFor="an-title">{t("announce.titleField")}</FieldLabel>
                <Input id="an-title" value={form.title} onChange={(e) => set("title", e.target.value)} maxLength={80} placeholder={t("announce.titlePlaceholder")} />
              </Field>
              <Field>
                <FieldLabel htmlFor="an-text">{t("announce.message")}</FieldLabel>
                <Textarea id="an-text" value={form.text} onChange={(e) => set("text", e.target.value)} maxLength={400} rows={4} placeholder={t("announce.messagePlaceholder")} />
                <FieldDescription className="tabular-nums">{t("announce.left", { count: 400 - form.text.length })}</FieldDescription>
              </Field>
              <Field orientation="horizontal">
                <Checkbox id="an-name" checked={form.showName} onCheckedChange={(v) => set("showName", v === true)} />
                <FieldLabel htmlFor="an-name" className="font-normal">
                  {t("announce.showName", { name: user?.username })}
                </FieldLabel>
              </Field>
            </div>
            <div className="space-y-2">
              <p className="text-xs font-medium text-muted-foreground">{t("announce.preview")}</p>
              <div className="rounded-xl bg-[linear-gradient(160deg,#5b6b82,#20262f)] p-4">
                <NoticeCard label={label} title={form.title || (!form.text ? t("announce.titlePlaceholder") : "")} text={form.text} />
              </div>
              <p className="text-xs text-muted-foreground">{t("announce.previewHelp")}</p>
            </div>
          </div>
          <ErrorNote error={error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending || Boolean(blocked) || (!form.title.trim() && !form.text.trim())}>
              {pending ? <Spinner /> : <MegaphoneIcon aria-hidden="true" />}
              {t("announce.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
