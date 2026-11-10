import { useEffect, useState } from "react"
import { useNavigate, useOutletContext, useSearchParams } from "react-router"
import { useTranslation } from "react-i18next"
import { CheckIcon, DownloadIcon, FolderInputIcon, FolderOpenIcon, InfoIcon } from "lucide-react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { ErrorNote, PageHeader } from "@/components/common"
import { FolderPicker } from "@/components/folder-picker"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { cn } from "@/lib/utils"

function ModeCard({ active, icon: Icon, title, text, onClick }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn("flex items-start gap-3 rounded-xl border p-4 text-left transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none", active && "border-foreground/40 bg-muted/50")}
    >
      <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted">
        <Icon className="size-4" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="font-medium">{title}</p>
        <p className="text-sm text-muted-foreground">{text}</p>
      </div>
      {active && <CheckIcon className="size-4 shrink-0" aria-hidden="true" />}
    </button>
  )
}

function FolderField({ id, label, value, onChange, help, onBrowsed }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <div className="flex gap-2">
        <Input id={id} value={value} onChange={(e) => onChange(e.target.value)} placeholder={navigator.platform.startsWith("Win") ? "D:\\servers\\my-server" : "/srv/my-server"} className="font-mono" spellCheck={false} />
        <Button type="button" variant="outline" onClick={() => setOpen(true)}>
          <FolderOpenIcon aria-hidden="true" />
          {t("add.browse")}
        </Button>
      </div>
      {help && <FieldDescription>{help}</FieldDescription>}
      {open && <FolderPicker open={open} onOpenChange={setOpen} start={value} onPick={(p) => (onBrowsed ?? onChange)(p)} />}
    </Field>
  )
}

const slug = (s) =>
  s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 40) || "server"

function joinPath(parent, name) {
  const sep = parent.includes("\\") ? "\\" : "/"
  return parent.replace(/[\\/]+$/, "") + sep + name
}

function InstallForm() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { servers } = useOutletContext()
  const games = useApi(api.games)
  const [form, setForm] = useState({ game: "arcane-mp", name: "", dir: "", port: 27015, examples: false, license: "" })
  const [job, setJob] = useState(null)
  const [error, setError] = useState(null)
  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }))
  const game = games.data?.find((g) => g.id === form.game)

  useEffect(() => {
    if (!job || job.finished) return
    const t = setTimeout(async () => {
      try {
        setJob(await api.job(job.id))
      } catch (err) {
        setError(err)
      }
    }, 1000)
    return () => clearTimeout(t)
  }, [job])

  useEffect(() => {
    if (job?.finished && !job.error && job.result?.serverId) {
      servers.reload()
      navigate(`/servers/${job.result.serverId}`)
    }
    if (job?.finished && job.error) setError(job.error)
  }, [job, navigate, servers])

  async function submit(e) {
    e.preventDefault()
    setError(null)
    try {
      setJob(await api.installServer({ ...form, port: Number(form.port) || 0 }))
    } catch (err) {
      setError(err)
    }
  }

  const running = job && !job.finished
  const pct = job?.total ? Math.round((job.done / job.total) * 100) : 0
  return (
    <Card>
      <form onSubmit={submit} className="contents">
        <CardHeader>
          <CardTitle>{t("add.installTitle")}</CardTitle>
          <CardDescription>{t("add.installText")}</CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="game">{t("add.game")}</FieldLabel>
              <Select value={form.game} onValueChange={(v) => set("game", v)} disabled={running}>
                <SelectTrigger id="game" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent position="popper">
                  {(games.data ?? []).map((g) => (
                    <SelectItem key={g.id} value={g.id}>
                      {g.label} {g.latest && <span className="font-mono text-muted-foreground">{g.latest}</span>}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldDescription>{game?.latest ? t("add.gameHelp", { version: game.latest }) : t("add.gameHelpUnknown")}</FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="name">{t("add.name")}</FieldLabel>
              <Input id="name" value={form.name} onChange={(e) => set("name", e.target.value)} maxLength={60} required disabled={running} placeholder={t("add.namePlaceholder")} />
            </Field>
            <FolderField
              id="dir"
              label={t("add.folder")}
              value={form.dir}
              onChange={(v) => set("dir", v)}
              onBrowsed={(p) => set("dir", joinPath(p, slug(form.name || game?.label || "server")))}
              help={t("add.folderHelp")}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="port">{t("add.port")}</FieldLabel>
                <Input id="port" type="number" min={1} max={65535} value={form.port} onChange={(e) => set("port", e.target.value)} disabled={running} />
                <FieldDescription>{t("add.portHelp")}</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="license">{t("add.license")}</FieldLabel>
                <Input id="license" type="password" autoComplete="off" value={form.license} onChange={(e) => set("license", e.target.value)} placeholder="LT-…" disabled={running} />
                <FieldDescription>{t("add.licenseHelp")}</FieldDescription>
              </Field>
            </div>
            <Field orientation="horizontal">
              <Checkbox id="examples" checked={form.examples} onCheckedChange={(v) => set("examples", v === true)} disabled={running} />
              <FieldLabel htmlFor="examples" className="font-normal">
                {t("add.examples")}
              </FieldLabel>
            </Field>
            {running && (
              <div className="space-y-2" aria-live="polite">
                <Progress value={pct} />
                <p className="text-sm text-muted-foreground">{job.total ? t("add.progress", { done: formatBytes(job.done), total: formatBytes(job.total) }) : t("add.starting")}</p>
              </div>
            )}
            <ErrorNote error={error} />
          </FieldGroup>
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" disabled={running || !form.name || !form.dir}>
            {running ? <Spinner /> : <DownloadIcon aria-hidden="true" />}
            {t("add.install")}
          </Button>
        </CardFooter>
      </form>
    </Card>
  )
}

function ImportForm() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { servers } = useOutletContext()
  const [dir, setDir] = useState("")
  const [name, setName] = useState("")
  const [preview, setPreview] = useState(null)
  const [error, setError] = useState(null)
  const [pending, setPending] = useState(null)

  async function check(path = dir) {
    setError(null)
    setPreview(null)
    setPending("check")
    try {
      const p = await api.importPreview(path)
      setPreview(p)
      if (!name) setName(p.dir.split(/[\\/]/).filter(Boolean).pop() ?? "")
    } catch (err) {
      setError(err)
    } finally {
      setPending(null)
    }
  }

  async function add(e) {
    e.preventDefault()
    setError(null)
    setPending("add")
    try {
      const sv = await api.importServer(preview.dir, name)
      servers.reload()
      navigate(`/servers/${sv.id}`)
    } catch (err) {
      setError(err)
    } finally {
      setPending(null)
    }
  }

  const gameLabel = { "arcane-mp": "Arcane MP", "kingdoms-connected": "Kingdoms Connected", "mafia2-online": "Mafia 2 Online" }
  return (
    <Card>
      <form onSubmit={add} className="contents">
        <CardHeader>
          <CardTitle>{t("add.importTitle")}</CardTitle>
          <CardDescription>{t("add.importText")}</CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <FolderField
              id="import-dir"
              label={t("add.serverFolder")}
              value={dir}
              onChange={(v) => (setDir(v), setPreview(null))}
              onBrowsed={(p) => (setDir(p), check(p))}
              help={t("add.serverFolderHelp")}
            />
            {!preview && (
              <div>
                <Button type="button" variant="outline" onClick={() => check()} disabled={!dir || pending === "check"}>
                  {pending === "check" && <Spinner />}
                  {t("add.checkFolder")}
                </Button>
              </div>
            )}
            {preview && (
              <>
                <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 rounded-lg border p-4 text-sm">
                  <dt className="text-muted-foreground">{t("add.game")}</dt>
                  <dd>
                    {gameLabel[preview.game] ?? preview.game} <span className="text-muted-foreground">({preview.exe})</span>
                  </dd>
                  <dt className="text-muted-foreground">{t("add.port")}</dt>
                  <dd className="font-mono">{preview.port}</dd>
                  <dt className="text-muted-foreground">{t("add.resources")}</dt>
                  <dd>{t("add.resourcesFound", { count: preview.resources, latch: preview.latch })}</dd>
                  <dt className="text-muted-foreground">{t("add.env")}</dt>
                  <dd className="min-w-0">
                    {preview.envKeys?.length ? (
                      <>
                        <p>{t("add.envFound", { count: preview.envKeys.length, files: preview.envFrom })}</p>
                        <p className="mt-1 font-mono text-xs break-words text-muted-foreground">{preview.envKeys.join(", ")}</p>
                      </>
                    ) : (
                      t("add.envNone")
                    )}
                  </dd>
                </dl>
                <Field>
                  <FieldLabel htmlFor="import-name">{t("add.name")}</FieldLabel>
                  <Input id="import-name" value={name} onChange={(e) => setName(e.target.value)} maxLength={60} required />
                </Field>
                <Alert>
                  <InfoIcon aria-hidden="true" />
                  <AlertDescription>{t("add.importNote")}</AlertDescription>
                </Alert>
              </>
            )}
            <ErrorNote error={error} />
          </FieldGroup>
        </CardContent>
        {preview && (
          <CardFooter className="justify-end">
            <Button type="submit" disabled={!name || pending === "add"}>
              {pending === "add" && <Spinner />}
              {t("add.addServer")}
            </Button>
          </CardFooter>
        )}
      </form>
    </Card>
  )
}

export default function AddServerPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const mode = params.get("mode") === "import" ? "import" : "install"
  return (
    <div className="space-y-6">
      <PageHeader title={t("nav.addServer")} description={t("add.text")} />
      <div className="grid gap-3 sm:grid-cols-2">
        <ModeCard active={mode === "install"} icon={DownloadIcon} title={t("add.installTitle")} text={t("add.installCard")} onClick={() => setParams({ mode: "install" }, { replace: true })} />
        <ModeCard active={mode === "import"} icon={FolderInputIcon} title={t("add.importTitle")} text={t("add.importCard")} onClick={() => setParams({ mode: "import" }, { replace: true })} />
      </div>
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        {mode === "install" ? <InstallForm /> : <ImportForm />}
        <aside className="space-y-3 rounded-xl border bg-muted/30 p-4 text-sm">
          <p className="font-medium">{t("add.howTitle")}</p>
          <ol className="list-decimal space-y-2 pl-5 text-muted-foreground">
            {["1", "2", "3"].map((n) => (
              <li key={n}>{t(`add.${mode}How${n}`)}</li>
            ))}
          </ol>
        </aside>
      </div>
    </div>
  )
}
