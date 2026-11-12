import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { MegaphoneIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldContent, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { AnnounceDialog } from "@/components/announce-dialog"
import { api } from "@/lib/api"
import { useCan } from "@/lib/session"

const CHOICES = [30, 15, 10, 5, 3, 1]

// NoticesCard: in-game announcements and restart warnings (the latchadmin resource) for one server.
export function NoticesCard({ server, reload }) {
  const { t } = useTranslation()
  const can = useCan()
  const n = server.notices ?? { enabled: true, warnings: [10, 5, 1], lang: "en" }
  const [form, setForm] = useState({ enabled: n.enabled, warnings: (n.warnings ?? []).map(String), lang: n.lang || "en" })
  const [pending, setPending] = useState(false)
  const [announce, setAnnounce] = useState(false)
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))
  const running = server.status.state === "running"
  const dirty = form.enabled !== n.enabled || form.lang !== (n.lang || "en") || [...form.warnings].sort().join() !== (n.warnings ?? []).map(String).sort().join()
  const editable = can("server.settings")
  const status = !n.enabled ? null : !running ? { tone: "outline", text: t("notices.statusStopped") } : n.connected ? { tone: "success", text: t("notices.statusOn") } : { tone: "warning", text: t("notices.statusWaiting") }

  async function save(e) {
    e.preventDefault()
    setPending(true)
    try {
      await api.patchServer(server.id, { NoticesOff: !form.enabled, Warnings: form.warnings.map(Number), NoticeLang: form.lang })
      toast.success(form.enabled !== n.enabled ? t("notices.savedRestart") : t("notices.saved"))
      reload()
    } catch (err) {
      toast.error(err.message)
    } finally {
      setPending(false)
    }
  }

  return (
    <Card>
      <form onSubmit={save} className="contents">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {t("notices.title")}
            {status && (
              <Badge variant="outline" className="gap-1.5 font-normal">
                <span className={status.tone === "success" ? "size-1.5 rounded-full bg-success" : status.tone === "warning" ? "size-1.5 rounded-full bg-warning" : "size-1.5 rounded-full bg-muted-foreground/50"} aria-hidden="true" />
                {status.text}
              </Badge>
            )}
          </CardTitle>
          <CardDescription>{t("notices.text")}</CardDescription>
          {can("console.write") && n.enabled && (
            <CardAction>
              <Button type="button" size="sm" variant="outline" onClick={() => setAnnounce(true)} disabled={!running}>
                <MegaphoneIcon aria-hidden="true" />
                {t("announce.open")}
              </Button>
            </CardAction>
          )}
        </CardHeader>
        <CardContent className="space-y-5">
          <Field orientation="horizontal">
            <FieldContent>
              <FieldLabel htmlFor="nt-on">{t("notices.enable")}</FieldLabel>
              <FieldDescription>{t("notices.enableHelp")}</FieldDescription>
            </FieldContent>
            <Switch id="nt-on" checked={form.enabled} onCheckedChange={(v) => set("enabled", v)} disabled={!editable} />
          </Field>
          {form.enabled && (
            <>
              <Field>
                <FieldLabel>{t("notices.warnings")}</FieldLabel>
                <ToggleGroup type="multiple" variant="outline" size="sm" value={form.warnings} onValueChange={(v) => set("warnings", v)} disabled={!editable} className="flex-wrap justify-start">
                  {CHOICES.map((m) => (
                    <ToggleGroupItem key={m} value={String(m)} className="min-w-14 tabular-nums data-[state=on]:border-foreground data-[state=on]:bg-foreground data-[state=on]:text-background">
                      {t("notices.minutes", { count: m })}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
                <FieldDescription>{server.restarts?.length ? t("notices.warningsHelp", { times: server.restarts.join(", ") }) : t("notices.noRestarts")}</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="nt-lang">{t("notices.lang")}</FieldLabel>
                <Select value={form.lang} onValueChange={(v) => set("lang", v)} disabled={!editable}>
                  <SelectTrigger id="nt-lang" className="w-48">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent position="popper">
                    <SelectItem value="en">English</SelectItem>
                    <SelectItem value="tr">Türkçe</SelectItem>
                  </SelectContent>
                </Select>
                <FieldDescription>{t("notices.langHelp")}</FieldDescription>
              </Field>
            </>
          )}
        </CardContent>
        {editable && (
          <CardFooter className="justify-end border-t">
            <Button type="submit" disabled={pending || !dirty}>
              {pending && <Spinner />}
              {t("common.save")}
            </Button>
          </CardFooter>
        )}
      </form>
      {announce && <AnnounceDialog server={server} open={announce} onOpenChange={setAnnounce} />}
    </Card>
  )
}
