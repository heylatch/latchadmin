import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { ErrorNote, Logo } from "@/components/common"
import { Prefs } from "@/components/layout"
import { api } from "@/lib/api"
import { useSession } from "@/lib/session"

function Frame({ children }) {
  return (
    <div className="flex min-h-dvh flex-col bg-muted/40">
      <div className="flex justify-end p-3">
        <Prefs />
      </div>
      <div className="flex flex-1 items-start justify-center px-4 pt-[8vh] pb-16">
        <div className="w-full max-w-sm space-y-6">
          <div className="flex items-center justify-center gap-2.5">
            <Logo className="size-9" />
            <span className="text-lg font-semibold tracking-tight">LatchAdmin</span>
          </div>
          {children}
        </div>
      </div>
    </div>
  )
}

const insecure = () => window.location.protocol === "http:" && !["localhost", "127.0.0.1", "[::1]"].includes(window.location.hostname)

export function SetupPage() {
  const { t } = useTranslation()
  const { setUser } = useSession()
  const [form, setForm] = useState({ pin: "", username: "", password: "", confirm: "" })
  const [error, setError] = useState(null)
  const [pending, setPending] = useState(false)
  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))
  const mismatch = form.confirm && form.password !== form.confirm

  async function submit(e) {
    e.preventDefault()
    if (mismatch) return
    setError(null)
    setPending(true)
    try {
      const res = await api.setup({ PIN: form.pin, Username: form.username, Password: form.password })
      setUser(res.user)
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Frame>
      <Card>
        <form onSubmit={submit} className="contents">
          <CardHeader>
            <CardTitle>{t("setup.title")}</CardTitle>
            <CardDescription>{t("setup.text")}</CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="pin">{t("setup.pin")}</FieldLabel>
                <Input id="pin" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={form.pin} onChange={set("pin")} className="font-mono tracking-widest" required autoFocus />
                <FieldDescription>{t("setup.pinHelp")}</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="username">{t("auth.username")}</FieldLabel>
                <Input id="username" autoComplete="username" value={form.username} onChange={set("username")} required />
              </Field>
              <Field>
                <FieldLabel htmlFor="password">{t("auth.password")}</FieldLabel>
                <Input id="password" type="password" autoComplete="new-password" value={form.password} onChange={set("password")} required minLength={10} />
                <FieldDescription>{t("auth.passwordHelp")}</FieldDescription>
              </Field>
              <Field data-invalid={mismatch || undefined}>
                <FieldLabel htmlFor="confirm">{t("auth.confirmPassword")}</FieldLabel>
                <Input id="confirm" type="password" autoComplete="new-password" value={form.confirm} onChange={set("confirm")} required aria-invalid={mismatch || undefined} />
                {mismatch && <FieldError>{t("auth.mismatch")}</FieldError>}
              </Field>
              <ErrorNote error={error} />
            </FieldGroup>
          </CardContent>
          <CardFooter>
            <Button type="submit" className="w-full" disabled={pending}>
              {pending && <Spinner />}
              {t("setup.submit")}
            </Button>
          </CardFooter>
        </form>
      </Card>
      {insecure() && <p className="text-center text-xs text-muted-foreground">{t("auth.httpWarning")}</p>}
    </Frame>
  )
}

export function LoginPage() {
  const { t } = useTranslation()
  const { setUser } = useSession()
  const [form, setForm] = useState({ username: "", password: "", code: "" })
  const [needCode, setNeedCode] = useState(false)
  const [error, setError] = useState(null)
  const [pending, setPending] = useState(false)
  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }))

  async function submit(e) {
    e.preventDefault()
    setError(null)
    setPending(true)
    try {
      const res = await api.login({ Username: form.username, Password: form.password, Code: form.code })
      setUser(res.user)
    } catch (err) {
      if (err.code === "codeRequired") setNeedCode(true)
      else setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Frame>
      <Card>
        <form onSubmit={submit} className="contents">
          <CardHeader>
            <CardTitle>{t(needCode ? "auth.codeTitle" : "auth.title")}</CardTitle>
            <CardDescription>{t(needCode ? "auth.codeText" : "auth.text")}</CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              {needCode ? (
                <Field>
                  <FieldLabel htmlFor="code">{t("auth.code")}</FieldLabel>
                  <Input id="code" inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={form.code} onChange={set("code")} className="font-mono tracking-widest" autoFocus required />
                </Field>
              ) : (
                <>
                  <Field>
                    <FieldLabel htmlFor="username">{t("auth.username")}</FieldLabel>
                    <Input id="username" autoComplete="username" value={form.username} onChange={set("username")} required autoFocus />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="password">{t("auth.password")}</FieldLabel>
                    <Input id="password" type="password" autoComplete="current-password" value={form.password} onChange={set("password")} required />
                  </Field>
                </>
              )}
              <ErrorNote error={error} />
            </FieldGroup>
          </CardContent>
          <CardFooter className="flex-col gap-2">
            <Button type="submit" className="w-full" disabled={pending}>
              {pending && <Spinner />}
              {t("auth.submit")}
            </Button>
            {needCode && (
              <Button type="button" variant="ghost" className="w-full" onClick={() => (setNeedCode(false), setForm((f) => ({ ...f, code: "" })))}>
                {t("common.back")}
              </Button>
            )}
          </CardFooter>
        </form>
      </Card>
      <p className="text-center text-xs text-muted-foreground">{t("auth.forgot")}</p>
      {insecure() && <p className="text-center text-xs text-muted-foreground">{t("auth.httpWarning")}</p>}
    </Frame>
  )
}
