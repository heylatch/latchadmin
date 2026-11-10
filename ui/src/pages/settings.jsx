import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import qrcode from "qrcode-generator"
import { CheckCircle2Icon, RefreshCwIcon, SirenIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { ConfirmButton, ErrorNote, PageHeader, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatRelative, isZeroTime } from "@/lib/format"
import { useCan, useSession } from "@/lib/session"

function PanelUpdate({ panel, reload }) {
  const { t } = useTranslation()
  const can = useCan()
  const [checking, setChecking] = useState(false)
  const u = panel.update
  const latest = u.info?.latest
  const newer = latest && latest !== u.current && !u.dev
  async function check() {
    setChecking(true)
    try {
      await api.panelCheck()
      reload()
    } catch (err) {
      toast.error(err.message)
    } finally {
      setChecking(false)
    }
  }
  async function install(now) {
    const res = await api.panelUpdate(now)
    toast.success(t(res.status === "restarting" ? "panelUpdate.restarting" : "panelUpdate.queued"))
    if (res.status === "restarting") setTimeout(() => window.location.reload(), 8000)
    reload()
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("panelUpdate.title")}</CardTitle>
        <CardDescription>{t("panelUpdate.text")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-4">
          <div>
            <dt className="text-xs text-muted-foreground">{t("panelUpdate.current")}</dt>
            <dd className="font-mono text-lg">{u.current}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">{t("panelUpdate.latest")}</dt>
            <dd className="font-mono text-lg">{latest || "—"}</dd>
          </div>
        </dl>
        {u.dev ? (
          <p className="text-sm text-muted-foreground">{t("panelUpdate.dev")}</p>
        ) : u.info?.urgent ? (
          <Alert variant="destructive">
            <SirenIcon aria-hidden="true" />
            <AlertTitle>{t("banner.panelTitle", { version: u.info.urgent.version })}</AlertTitle>
            <AlertDescription>{u.info.urgent.note || t("banner.noNote")}</AlertDescription>
          </Alert>
        ) : !newer && latest ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <CheckCircle2Icon className="size-4 text-success" aria-hidden="true" />
            {t("panelUpdate.upToDate")}
          </p>
        ) : null}
        {u.pending && <p className="text-sm">{t("panelUpdate.pending")}</p>}
        <p className="text-xs text-muted-foreground">
          {panel.versions.error
            ? t("panelUpdate.checkFailed", { error: panel.versions.error })
            : isZeroTime(panel.versions.checkedAt)
              ? t("panelUpdate.notChecked")
              : t("panelUpdate.checked", { time: formatRelative(panel.versions.checkedAt), source: t(`panelUpdate.source.${panel.versions.source || "direct"}`) })}
        </p>
      </CardContent>
      <CardFooter className="flex-wrap justify-end gap-2">
        <Button variant="outline" onClick={check} disabled={checking}>
          {checking ? <Spinner /> : <RefreshCwIcon aria-hidden="true" />}
          {t("panelUpdate.check")}
        </Button>
        {can("panel.update") && newer && (
          <>
            <Button variant="outline" onClick={() => install(false).catch((e) => toast.error(e.message))}>
              {t("panelUpdate.whenQuiet")}
            </Button>
            <ConfirmButton title={t("panelUpdate.nowTitle", { version: latest })} description={t("panelUpdate.nowText")} confirmLabel={t("panelUpdate.now")} onConfirm={() => install(true)}>
              {t("panelUpdate.now")}
            </ConfirmButton>
          </>
        )}
      </CardFooter>
    </Card>
  )
}

function PanelSettings({ panel, reload }) {
  const { t } = useTranslation()
  const [name, setName] = useState(panel.settings.panelName)
  const [pending, setPending] = useState(false)
  async function save(patch) {
    setPending(true)
    try {
      await api.panelSettings(patch)
      toast.success(t("config.saved"))
      reload()
    } catch (err) {
      toast.error(err.message)
    } finally {
      setPending(false)
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("panelSettings.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="panel-name">{t("panelSettings.name")}</FieldLabel>
            <div className="flex gap-2">
              <Input id="panel-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={60} />
              <Button variant="outline" disabled={pending || !name.trim() || name === panel.settings.panelName} onClick={() => save({ PanelName: name })}>
                {t("common.save")}
              </Button>
            </div>
            <FieldDescription>{t("panelSettings.nameHelp")}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel>{t("panelSettings.selfUpdate")}</FieldLabel>
            <RadioGroup value={panel.settings.selfUpdate} onValueChange={(v) => save({ SelfUpdate: v })} className="gap-2">
              {["urgent", "all", "off"].map((v) => (
                <label key={v} htmlFor={`su-${v}`} className="flex cursor-pointer items-start gap-3 rounded-lg border p-3 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring">
                  <RadioGroupItem id={`su-${v}`} value={v} className="mt-0.5" />
                  <span>
                    <span className="block text-sm font-medium">{t(`panelSettings.su.${v}`)}</span>
                    <span className="block text-sm text-muted-foreground">{t(`panelSettings.su.${v}Help`)}</span>
                  </span>
                </label>
              ))}
            </RadioGroup>
          </Field>
          <Field orientation="horizontal">
            <FieldContent>
              <FieldLabel htmlFor="check-latch">{t("panelSettings.checkLatch")}</FieldLabel>
              <FieldDescription>{t("panelSettings.checkLatchHelp")}</FieldDescription>
            </FieldContent>
            <Switch id="check-latch" checked={panel.settings.checkLatch} onCheckedChange={(v) => save({ CheckLatch: v })} />
          </Field>
        </FieldGroup>
      </CardContent>
    </Card>
  )
}

function Password() {
  const { t } = useTranslation()
  const [form, setForm] = useState({ current: "", next: "" })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  async function submit(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      await api.changePassword({ Current: form.current, Next: form.next })
      setForm({ current: "", next: "" })
      toast.success(t("account.passwordChanged"))
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }
  return (
    <Card>
      <form onSubmit={submit} className="contents">
        <CardHeader>
          <CardTitle>{t("account.passwordTitle")}</CardTitle>
          <CardDescription>{t("account.passwordText")}</CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="pw-current">{t("account.current")}</FieldLabel>
              <Input id="pw-current" type="password" autoComplete="current-password" value={form.current} onChange={(e) => setForm((f) => ({ ...f, current: e.target.value }))} required />
            </Field>
            <Field>
              <FieldLabel htmlFor="pw-next">{t("account.next")}</FieldLabel>
              <Input id="pw-next" type="password" autoComplete="new-password" value={form.next} onChange={(e) => setForm((f) => ({ ...f, next: e.target.value }))} required minLength={10} />
              <FieldDescription>{t("auth.passwordHelp")}</FieldDescription>
            </Field>
            <ErrorNote error={error} />
          </FieldGroup>
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" disabled={pending}>
            {pending && <Spinner />}
            {t("account.changePassword")}
          </Button>
        </CardFooter>
      </form>
    </Card>
  )
}

function QR({ text }) {
  const { t } = useTranslation()
  const src = useMemo(() => {
    const qr = qrcode(0, "M")
    qr.addData(text)
    qr.make()
    return qr.createDataURL(5, 2)
  }, [text])
  return <img src={src} alt={t("account.qrAlt")} className="size-44 rounded-lg bg-white p-1 [image-rendering:pixelated]" />
}

function TwoFactor() {
  const { t } = useTranslation()
  const { user, refresh } = useSession()
  const [setup, setSetup] = useState(null)
  const [code, setCode] = useState("")
  const [password, setPassword] = useState("")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)

  async function run(fn) {
    setPending(true)
    setError(null)
    try {
      await fn()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("account.totpTitle")}</CardTitle>
        <CardDescription>{t(user.totp ? "account.totpOn" : "account.totpText")}</CardDescription>
      </CardHeader>
      {(user.totp || setup || error) && (
      <CardContent className="space-y-4">
        {user.totp ? (
          <Field>
            <FieldLabel htmlFor="totp-pw">{t("account.confirmWithPassword")}</FieldLabel>
            <Input id="totp-pw" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </Field>
        ) : setup ? (
          <div className="flex flex-col gap-4 sm:flex-row">
            <QR text={setup.uri} />
            <div className="min-w-0 flex-1 space-y-3">
              <p className="text-sm text-muted-foreground">{t("account.scan")}</p>
              <code className="block rounded bg-muted px-2 py-1 text-xs break-all">{setup.secret}</code>
              <Field>
                <FieldLabel htmlFor="totp-code">{t("auth.code")}</FieldLabel>
                <Input id="totp-code" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} className="font-mono tracking-widest" />
              </Field>
            </div>
          </div>
        ) : null}
        <ErrorNote error={error} />
      </CardContent>
      )}
      <CardFooter className="justify-end gap-2">
        {user.totp ? (
          <Button variant="outline" disabled={pending || !password} onClick={() => run(async () => (await api.totpDisable(password), setPassword(""), toast.success(t("account.totpDisabled")), refresh()))}>
            {pending && <Spinner />}
            {t("account.disableTotp")}
          </Button>
        ) : setup ? (
          <Button disabled={pending || code.length !== 6} onClick={() => run(async () => (await api.totpEnable(code), setSetup(null), setCode(""), toast.success(t("account.totpEnabled")), refresh()))}>
            {pending && <Spinner />}
            {t("account.enableTotp")}
          </Button>
        ) : (
          <Button onClick={() => run(async () => setSetup(await api.totpSetup()))} disabled={pending}>
            {t("account.setupTotp")}
          </Button>
        )}
      </CardFooter>
    </Card>
  )
}

export default function SettingsPage() {
  const { t } = useTranslation()
  const can = useCan()
  const panel = useApi(api.panel)
  return (
    <div className="space-y-6">
      <PageHeader title={t("nav.settings")} description={t("settings.text")} />
      <Query query={panel}>
        {(p) => (
          <div className="grid items-start gap-6 lg:grid-cols-2">
            <div className="space-y-6">
              <PanelUpdate panel={p} reload={panel.reload} />
              {can("panel.settings") && <PanelSettings panel={p} reload={panel.reload} />}
            </div>
            <div className="space-y-6">
              <Password />
              <TwoFactor />
            </div>
          </div>
        )}
      </Query>
    </div>
  )
}
