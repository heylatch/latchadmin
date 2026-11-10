import { useEffect, useState } from "react"
import { Link } from "react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { CheckCircle2Icon, ExternalLinkIcon, LinkIcon, ShieldCheckIcon, TriangleAlertIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmButton, ErrorNote, PageHeader, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatDateTime, formatRelative, isZeroTime } from "@/lib/format"
import { useCan } from "@/lib/session"

function LinkFlow({ status, onDone }) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  const [key, setKey] = useState("")
  const link = status.link
  const active = link?.status === "pending"

  useEffect(() => {
    if (link?.status === "approved") onDone()
  }, [link?.status, onDone])

  async function start() {
    setPending(true)
    setError(null)
    try {
      await api.latchLink()
      onDone()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  async function paste(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      await api.latchKey(key)
      setKey("")
      toast.success(t("latch.linked"))
      onDone()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("latch.linkTitle")}</CardTitle>
        <CardDescription>{t("latch.linkText")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {active ? (
          <div className="space-y-4 rounded-lg border p-4">
            <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
              <li>{t("latch.step1")}</li>
              <li>{t("latch.step2")}</li>
              <li>{t("latch.step3")}</li>
            </ol>
            <div className="flex flex-wrap items-center gap-4">
              <span className="rounded-lg bg-muted px-4 py-2 font-mono text-2xl tracking-widest select-all" aria-label={t("latch.code")}>
                {link.userCode}
              </span>
              <Button asChild>
                <a href={link.verifyUrlComplete} target="_blank" rel="noreferrer">
                  <ExternalLinkIcon aria-hidden="true" />
                  {t("latch.openLatch")}
                </a>
              </Button>
            </div>
            <p className="flex items-center gap-2 text-sm text-muted-foreground" aria-live="polite">
              <Spinner />
              {t("latch.waiting", { time: formatRelative(link.expiresAt) })}
            </p>
            <Button variant="ghost" size="sm" onClick={async () => (await api.latchCancelLink(), onDone())}>
              {t("common.cancel")}
            </Button>
          </div>
        ) : (
          <>
            {link && link.status !== "approved" && (
              <p className="text-sm text-muted-foreground">{t(`latch.linkStatus.${link.status}`, { error: link.error })}</p>
            )}
            <Button onClick={start} disabled={pending}>
              {pending ? <Spinner /> : <LinkIcon aria-hidden="true" />}
              {t("latch.linkButton")}
            </Button>
          </>
        )}
        <Collapsible>
          <CollapsibleTrigger className="text-sm text-muted-foreground underline underline-offset-4 hover:text-foreground">{t("latch.pasteToggle")}</CollapsibleTrigger>
          <CollapsibleContent>
            <form onSubmit={paste} className="mt-3 space-y-2">
              <Field>
                <FieldLabel htmlFor="lta">{t("latch.keyLabel")}</FieldLabel>
                <div className="flex gap-2">
                  <Input id="lta" type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} placeholder="lta_…" className="font-mono" />
                  <Button type="submit" variant="outline" disabled={!key || pending}>
                    {t("latch.useKey")}
                  </Button>
                </div>
                <FieldDescription>{t("latch.keyHelp")}</FieldDescription>
              </Field>
            </form>
          </CollapsibleContent>
        </Collapsible>
        <ErrorNote error={error} />
      </CardContent>
    </Card>
  )
}

function Linked({ status, onChange }) {
  const { t } = useTranslation()
  const can = useCan()
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CheckCircle2Icon className="size-4 text-success" aria-hidden="true" />
          {t("latch.linkedTitle", { name: status.displayName })}
        </CardTitle>
        <CardDescription>
          <span className="font-mono">{status.accountId}</span> · {t("latch.linkedSince", { date: formatDateTime(status.linkedAt) })}
        </CardDescription>
        {can("panel.latch") && (
          <CardAction>
            <ConfirmButton
              variant="outline"
              size="sm"
              title={t("latch.unlinkTitle")}
              description={t("latch.unlinkText")}
              confirmLabel={t("latch.unlink")}
              destructive
              onConfirm={async () => (await api.latchUnlink(), onChange())}
            >
              {t("latch.unlink")}
            </ConfirmButton>
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="text-sm text-muted-foreground">
        {isZeroTime(status.lastReportAt) ? t("latch.noReportYet") : t("latch.lastReport", { time: formatRelative(status.lastReportAt) })}
      </CardContent>
    </Card>
  )
}

function Telemetry({ status }) {
  const { t } = useTranslation()
  const sends = ["send1", "send2", "send3"]
  const never = ["never1", "never2", "never3"]
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckIcon className="size-4" aria-hidden="true" />
          {t("latch.privacyTitle")}
        </CardTitle>
        <CardDescription>{t(status.linked ? "latch.privacyLinked" : "latch.privacyUnlinked")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <p className="mb-1.5 text-sm font-medium">{t("latch.sendsTitle")}</p>
            <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
              {sends.map((s) => (
                <li key={s}>{t(`latch.${s}`)}</li>
              ))}
            </ul>
          </div>
          <div>
            <p className="mb-1.5 text-sm font-medium">{t("latch.neverTitle")}</p>
            <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
              {never.map((s) => (
                <li key={s}>{t(`latch.${s}`)}</li>
              ))}
            </ul>
          </div>
        </div>
        <p className="text-sm text-muted-foreground">{t("latch.noInbound")}</p>
        <Collapsible>
          <CollapsibleTrigger className="text-sm underline underline-offset-4">{t("latch.showPayload")}</CollapsibleTrigger>
          <CollapsibleContent>
            <pre className="mt-3 max-h-96 overflow-auto rounded-lg bg-muted p-3 text-xs">{JSON.stringify(status.nextReport, null, 2)}</pre>
          </CollapsibleContent>
        </Collapsible>
      </CardContent>
      <CardFooter>
        <p className="text-xs text-muted-foreground">
          {t("latch.versionCheck", { state: t(status.checkLatch ? "latch.on" : "latch.off") })}{" "}
          <Link to="/settings" className="underline underline-offset-4">
            {t("nav.settings")}
          </Link>
        </p>
      </CardFooter>
    </Card>
  )
}

function Library({ servers }) {
  const { t } = useTranslation()
  const lib = useApi(api.library)
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-sm font-medium">{t("latch.libraryTitle")}</h2>
        <p className="text-sm text-muted-foreground">{t("latch.libraryText")}</p>
      </div>
      <Query query={lib}>
        {(rows) =>
          !rows.length ? (
            <p className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">{t("scripts.emptyText")}</p>
          ) : (
            <div className="rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("scripts.script")}</TableHead>
                    <TableHead>{t("scripts.latest")}</TableHead>
                    <TableHead>{t("latch.installedOn")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((r) => (
                    <TableRow key={r.item.script.id}>
                      <TableCell>
                        <p className="font-medium">{r.item.script.name}</p>
                        <p className="text-xs text-muted-foreground">{r.item.workspace.name}</p>
                      </TableCell>
                      <TableCell className="font-mono text-sm">{r.item.script.latestVersion ?? "—"}</TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-1.5">
                          {(servers ?? [])
                            .filter((s) => r.servers[s.id]?.installed)
                            .map((s) => (
                              <Badge key={s.id} variant="outline" asChild>
                                <Link to={`/servers/${s.id}/scripts`}>
                                  {s.name} <span className="font-mono">{r.servers[s.id].installed}</span>
                                </Link>
                              </Badge>
                            ))}
                          {!(servers ?? []).some((s) => r.servers[s.id]?.installed) && <span className="text-sm text-muted-foreground">{t("scripts.notInstalled")}</span>}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )
        }
      </Query>
    </section>
  )
}

export default function LatchPage() {
  const { t } = useTranslation()
  const can = useCan()
  const status = useApi(api.latch, [], { poll: 0 })
  const servers = useApi(api.servers)
  // Poll quickly while a link code is waiting for approval.
  const waiting = status.data?.link?.status === "pending"
  useEffect(() => {
    if (!waiting) return
    const timer = setInterval(status.reload, 2000)
    return () => clearInterval(timer)
  }, [waiting, status.reload])

  return (
    <div className="space-y-6">
      <PageHeader title="Latch" description={t("latch.text")} />
      <Query query={status}>
        {(s) => (
          <div className="space-y-6">
            {s.disconnected && (
              <Alert variant="destructive">
                <TriangleAlertIcon aria-hidden="true" />
                <AlertTitle>{t("latch.disconnectedTitle")}</AlertTitle>
                <AlertDescription>{t("latch.disconnectedText")}</AlertDescription>
              </Alert>
            )}
            <div className="grid items-start gap-6 lg:grid-cols-2">
              <div className="space-y-6">
                {s.linked && !s.disconnected ? <Linked status={s} onChange={status.reload} /> : can("panel.latch") ? <LinkFlow status={s} onDone={status.reload} /> : <p className="text-sm text-muted-foreground">{t("latch.ownerOnly")}</p>}
              </div>
              <Telemetry status={s} />
            </div>
            {s.linked && !s.disconnected && <Library servers={servers.data} />}
          </div>
        )}
      </Query>
    </div>
  )
}
