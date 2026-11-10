import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { EyeOffIcon, LockIcon, PlusIcon, Trash2Icon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { ErrorNote, Loading } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"

function restartNote(server, t) {
  return server.status.state === "running" ? t("config.restartToApply") : null
}

// JsonEditor edits a JSON file as text, checking it parses before saving.
function JsonEditor({ title, description, load, save, deps, server }) {
  const { t } = useTranslation()
  const query = useApi(load, deps)
  const [text, setText] = useState("")
  const [dirty, setDirty] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    if (query.data !== undefined) {
      try {
        setText(JSON.stringify(JSON.parse(query.data || "{}"), null, 2))
      } catch {
        setText(query.data)
      }
      setDirty(false)
    }
  }, [query.data])

  let parseError = null
  try {
    JSON.parse(text || "{}")
  } catch (e) {
    parseError = e.message
  }

  async function submit() {
    setPending(true)
    setError(null)
    try {
      await save(text)
      toast.success(t("config.saved"))
      setDirty(false)
      const note = restartNote(server, t)
      if (note) toast.message(note)
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent className="space-y-3">
        {query.data === undefined && !query.error ? (
          <Loading rows={2} />
        ) : (
          <Textarea
            value={text}
            onChange={(e) => (setText(e.target.value), setDirty(true))}
            spellCheck={false}
            className="min-h-72 font-mono text-xs"
            aria-invalid={parseError ? true : undefined}
            aria-label={title}
          />
        )}
        {parseError && dirty && <p className="text-sm text-destructive">{t("config.invalidJson", { error: parseError })}</p>}
        <ErrorNote error={error ?? query.error} />
      </CardContent>
      <CardFooter className="justify-end gap-2">
        <Button variant="ghost" disabled={!dirty || pending} onClick={() => query.reload()}>
          {t("config.discard")}
        </Button>
        <Button disabled={!dirty || pending || Boolean(parseError)} onClick={submit}>
          {pending && <Spinner />}
          {t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  )
}

const secretGuess = /(pass|secret|token|key|license|pwd|auth)/i

function EnvEditor({ server }) {
  const { t } = useTranslation()
  const query = useApi(() => api.env(server.id), [server.id])
  const [rows, setRows] = useState(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    if (query.data) setRows(query.data.map((e) => ({ key: e.key, value: e.secret ? "" : e.value, secret: e.secret, stored: e.secret && e.set, changed: false })))
  }, [query.data])

  const update = (i, patch) => setRows((r) => r.map((x, j) => (j === i ? { ...x, ...patch, changed: true } : x)))

  async function submit() {
    setPending(true)
    setError(null)
    try {
      // A stored secret left empty keeps its value (the value never comes back to the browser).
      const body = rows
        .filter((r) => r.key.trim())
        .map((r) => ({ Key: r.key.trim(), Secret: r.secret, ...(r.secret && r.stored && !r.value ? {} : { Value: r.value }) }))
      const saved = await api.saveEnv(server.id, body)
      query.setData(saved)
      toast.success(t("config.saved"))
      const note = restartNote(server, t)
      if (note) toast.message(note)
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  const dirty = rows?.some((r) => r.changed) || (query.data && rows && rows.length !== query.data.length)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("config.envTitle")}</CardTitle>
        <CardDescription>{t("config.envText")}</CardDescription>
        <CardAction>
          <Button size="sm" variant="outline" onClick={() => setRows((r) => [...(r ?? []), { key: "", value: "", secret: false, stored: false, changed: true }])}>
            <PlusIcon aria-hidden="true" />
            {t("config.addVariable")}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-2">
        {!rows ? (
          <Loading rows={2} />
        ) : !rows.length ? (
          <p className="text-sm text-muted-foreground">{t("config.envEmpty")}</p>
        ) : (
          rows.map((r, i) => (
            <div key={i} className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto_auto] items-center gap-2">
              <Input
                value={r.key}
                onChange={(e) => {
                  const key = e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, "")
                  update(i, { key, secret: r.changed && !r.stored && !r.value ? secretGuess.test(key) : r.secret })
                }}
                placeholder="NAME"
                className="font-mono"
                aria-label={t("config.varName")}
              />
              <Input
                type={r.secret ? "password" : "text"}
                value={r.value}
                onChange={(e) => update(i, { value: e.target.value })}
                placeholder={r.stored ? t("config.secretStored") : t("config.value")}
                className="font-mono"
                autoComplete="off"
                aria-label={t("config.varValue", { name: r.key })}
              />
              <label className="flex items-center gap-1.5 text-xs text-muted-foreground" title={t("config.secretHelp")}>
                <Checkbox checked={r.secret} onCheckedChange={(v) => update(i, { secret: v === true })} />
                {r.secret ? <LockIcon className="size-3.5" aria-hidden="true" /> : <EyeOffIcon className="size-3.5 opacity-40" aria-hidden="true" />}
                <span className="sr-only sm:not-sr-only">{t("config.secret")}</span>
              </label>
              <Button variant="ghost" size="icon-sm" onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))} aria-label={t("config.removeVar", { name: r.key })}>
                <Trash2Icon aria-hidden="true" />
              </Button>
            </div>
          ))
        )}
        <ErrorNote error={error} />
      </CardContent>
      <CardFooter className="justify-between gap-2">
        <p className="text-xs text-muted-foreground">{t("config.envFoot")}</p>
        <Button disabled={!dirty || pending} onClick={submit}>
          {pending && <Spinner />}
          {t("common.save")}
        </Button>
      </CardFooter>
    </Card>
  )
}

function DataFiles({ server }) {
  const { t } = useTranslation()
  const files = useApi(() => api.dataFiles(server.id), [server.id])
  const [file, setFile] = useState("")
  useEffect(() => {
    if (!file && files.data?.length) setFile(files.data[0])
  }, [files.data, file])
  if (!files.data?.length) return null
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-sm font-medium">{t("config.dataTitle")}</h2>
        <Select value={file} onValueChange={setFile}>
          <SelectTrigger size="sm" className="w-64" aria-label={t("config.dataTitle")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent position="popper">
            {files.data.map((f) => (
              <SelectItem key={f} value={f}>
                {f}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {file && (
        <JsonEditor
          key={file}
          title={`data/${file}`}
          description={t("config.dataText")}
          deps={[server.id, file]}
          load={() => api.dataFile(server.id, file)}
          save={(text) => api.saveDataFile(server.id, file, text)}
          server={server}
        />
      )}
    </div>
  )
}

function TokenCard({ server, reload }) {
  const { t } = useTranslation()
  const [token, setToken] = useState("")
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  async function save(value) {
    setPending(true)
    setError(null)
    try {
      await api.setToken(server.id, value)
      setToken("")
      toast.success(t(value ? "token.saved" : "token.cleared"))
      const note = restartNote(server, t)
      if (note) toast.message(note)
      reload?.()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("token.title")}</CardTitle>
        <CardDescription>
          {t("token.text", { port: server.port })}{" "}
          <a href="https://mafiahub.dev/" target="_blank" rel="noreferrer" className="underline underline-offset-4">
            mafiahub.dev
          </a>
        </CardDescription>
        <CardAction>
          {server.tokenSet ? (
            <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
              <span className="size-1.5 rounded-full bg-success" aria-hidden="true" />
              {t("token.set")}
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5 text-sm text-warning">
              <span className="size-1.5 rounded-full bg-warning" aria-hidden="true" />
              {t("token.missing")}
            </span>
          )}
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        <form
          onSubmit={(e) => {
            e.preventDefault()
            save(token.trim())
          }}
          className="flex gap-2"
        >
          <Input
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder={server.tokenSet ? t("config.secretStored") : t("token.placeholder")}
            className="font-mono"
            aria-label={t("token.title")}
          />
          <Button type="submit" disabled={pending || !token.trim()}>
            {pending && <Spinner />}
            {t("common.save")}
          </Button>
          {server.tokenSet && (
            <Button type="button" variant="ghost" disabled={pending} onClick={() => save("")}>
              {t("token.clear")}
            </Button>
          )}
        </form>
        <ErrorNote error={error} />
      </CardContent>
    </Card>
  )
}

export default function ConfigTab({ server, reload }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-6">
      <TokenCard server={server} reload={reload} />
      <EnvEditor server={server} />
      <JsonEditor
        title="server.json"
        description={t("config.serverJsonText")}
        deps={[server.id]}
        load={() => api.config(server.id)}
        save={(text) => api.saveConfig(server.id, text)}
        server={server}
      />
      <DataFiles server={server} />
    </div>
  )
}
