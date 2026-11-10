import { useMemo, useState } from "react"
import { Navigate, useOutletContext } from "react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { CheckIcon, CopyIcon, EllipsisIcon, KeyRoundIcon, PencilIcon, PlusIcon, ShieldCheckIcon, Trash2Icon, WandSparklesIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Input } from "@/components/ui/input"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Spinner } from "@/components/ui/spinner"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmDialog, ErrorNote, PageHeader, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatDateTime } from "@/lib/format"
import { hasPerm, useCan, useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

// 18 characters from an alphabet without look-alikes (no 0/O, 1/l/I): about 100 bits.
const ALPHABET = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
function generatePassword() {
  const bytes = crypto.getRandomValues(new Uint32Array(18))
  const chars = [...bytes].map((b) => ALPHABET[b % ALPHABET.length])
  return `${chars.slice(0, 6).join("")}-${chars.slice(6, 12).join("")}-${chars.slice(12).join("")}`
}

function PasswordField({ id, value, onChange, label, help }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error(t("users.copyFailed"))
    }
  }
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <div className="flex gap-2">
        <InputGroup>
          <InputGroupInput id={id} value={value} onChange={(e) => onChange(e.target.value)} autoComplete="new-password" spellCheck={false} className="font-mono" required minLength={10} />
          <InputGroupAddon align="inline-end">
            <InputGroupButton size="icon-xs" onClick={copy} disabled={!value} aria-label={t("users.copy")}>
              {copied ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
            </InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
        <Button type="button" variant="outline" onClick={() => onChange(generatePassword())}>
          <WandSparklesIcon aria-hidden="true" />
          {t("users.generate")}
        </Button>
      </div>
      <FieldDescription>{help}</FieldDescription>
    </Field>
  )
}

function PermissionPicker({ groups, value, onChange, mine }) {
  const { t } = useTranslation()
  const all = value.includes("all")
  const canGrant = (p) => hasPerm(mine, p)
  const toggle = (p, on) => onChange(on ? [...new Set([...value, p])] : value.filter((x) => x !== p))
  const setGroup = (g, on) => {
    const grantable = g.perms.filter(canGrant)
    onChange(on ? [...new Set([...value, ...grantable])] : value.filter((x) => !g.perms.includes(x)))
  }
  const every = groups.flatMap((g) => g.perms).filter(canGrant)
  const selected = every.filter((p) => value.includes(p)).length

  return (
    <div className="space-y-3">
      <label
        htmlFor="perm-all"
        className={cn("flex items-start gap-3 rounded-lg border p-3 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring", all && "border-foreground/40 bg-muted/50", !canGrant("all") && "opacity-50")}
      >
        <Checkbox id="perm-all" checked={all} disabled={!canGrant("all")} onCheckedChange={(v) => onChange(v === true ? ["all"] : [])} className="mt-0.5" />
        <span>
          <span className="flex items-center gap-1.5 text-sm font-medium">
            <ShieldCheckIcon className="size-4" aria-hidden="true" />
            {t("perm.all")}
          </span>
          <span className="block text-sm text-muted-foreground">{t("perm.allHelp")}</span>
        </span>
      </label>
      {!all && (
        <>
          <div className="flex items-center justify-between gap-2 text-sm">
            <span className="text-muted-foreground">{t("perm.selected", { count: selected, total: every.length })}</span>
            <div className="flex gap-1">
              <Button type="button" variant="ghost" size="sm" onClick={() => onChange(every)}>
                {t("perm.checkAll")}
              </Button>
              <Button type="button" variant="ghost" size="sm" onClick={() => onChange([])}>
                {t("perm.uncheckAll")}
              </Button>
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {groups.map((g) => {
              const on = g.perms.filter((p) => value.includes(p)).length
              const state = on === 0 ? false : on === g.perms.length ? true : "indeterminate"
              return (
                <fieldset key={g.id} className="rounded-lg border">
                  <legend className="sr-only">{t(`perm.groups.${g.id}`)}</legend>
                  <label htmlFor={`grp-${g.id}`} className="flex items-center gap-2.5 border-b bg-muted/40 px-3 py-2">
                    <Checkbox id={`grp-${g.id}`} checked={state} onCheckedChange={(v) => setGroup(g, v === true)} />
                    <span className="text-sm font-medium">{t(`perm.groups.${g.id}`)}</span>
                    <span className="ml-auto text-xs text-muted-foreground tabular-nums">
                      {on}/{g.perms.length}
                    </span>
                  </label>
                  <div className="space-y-2.5 p-3">
                    {g.perms.map((p) => (
                      <label key={p} htmlFor={`perm-${p}`} className={cn("flex items-start gap-2.5", !canGrant(p) && "opacity-50")}>
                        <Checkbox id={`perm-${p}`} checked={value.includes(p)} disabled={!canGrant(p)} onCheckedChange={(v) => toggle(p, v === true)} className="mt-0.5" />
                        <span className="min-w-0">
                          <span className="block text-sm leading-tight">{t(`perm.items.${p}.label`)}</span>
                          <span className="block text-xs text-muted-foreground">{t(`perm.items.${p}.help`)}</span>
                        </span>
                      </label>
                    ))}
                  </div>
                </fieldset>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}

function ServerPicker({ servers, all, list, onChange, disabled }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-3">
      <RadioGroup value={all ? "all" : "some"} onValueChange={(v) => onChange(v === "all", list)} className="gap-2" disabled={disabled}>
        {["all", "some"].map((v) => (
          <label key={v} htmlFor={`srv-${v}`} className="flex items-start gap-3 rounded-lg border p-3 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring">
            <RadioGroupItem id={`srv-${v}`} value={v} className="mt-0.5" />
            <span>
              <span className="block text-sm font-medium">{t(`users.servers.${v}`)}</span>
              <span className="block text-sm text-muted-foreground">{t(`users.servers.${v}Help`)}</span>
            </span>
          </label>
        ))}
      </RadioGroup>
      {!all && (
        <div className="max-h-48 space-y-2 overflow-y-auto rounded-lg border p-3">
          {servers.length ? (
            servers.map((s) => (
              <label key={s.id} htmlFor={`srv-pick-${s.id}`} className="flex items-center gap-2.5 text-sm">
                <Checkbox id={`srv-pick-${s.id}`} checked={list.includes(s.id)} onCheckedChange={(v) => onChange(false, v === true ? [...list, s.id] : list.filter((x) => x !== s.id))} />
                {s.name}
                <span className="text-xs text-muted-foreground">{s.gameName}</span>
              </label>
            ))
          ) : (
            <p className="text-sm text-muted-foreground">{t("users.servers.none")}</p>
          )}
        </div>
      )}
    </div>
  )
}

function UserDialog({ user, groups, servers, onClose, onSaved }) {
  const { t } = useTranslation()
  const { user: me } = useSession()
  const editing = Boolean(user)
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState(() => (editing ? "" : generatePassword()))
  const [permissions, setPermissions] = useState(user?.permissions ?? ["console.view"])
  const [access, setAccess] = useState({ all: user ? user.allServers : false, list: user?.servers ?? [] })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  const allPerms = permissions.includes("all")

  async function submit(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      const body = { Permissions: permissions, AllServers: allPerms || access.all, Servers: access.list }
      if (editing) await api.updateUser(user.id, body)
      else await api.createUser({ ...body, Username: username, Password: password })
      toast.success(t(editing ? "users.saved" : "users.created"))
      onSaved()
      onClose()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-3xl">
        <form onSubmit={submit} className="grid gap-6">
          <DialogHeader>
            <DialogTitle>{editing ? t("users.editTitle", { name: user.username }) : t("users.addTitle")}</DialogTitle>
            <DialogDescription>{t(editing ? "users.editText" : "users.addText")}</DialogDescription>
          </DialogHeader>
          {!editing && (
            <FieldGroup className="grid gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="u-name">{t("auth.username")}</FieldLabel>
                <Input id="u-name" value={username} onChange={(e) => setUsername(e.target.value)} required autoComplete="off" autoFocus />
                <FieldDescription>{t("users.usernameHelp")}</FieldDescription>
              </Field>
              <PasswordField id="u-pass" label={t("auth.password")} value={password} onChange={setPassword} help={t("users.passwordShare")} />
            </FieldGroup>
          )}
          <section className="space-y-2">
            <h3 className="text-sm font-medium">{t("users.permissionsTitle")}</h3>
            <PermissionPicker groups={groups} value={permissions} onChange={setPermissions} mine={me.permissions} />
          </section>
          <section className="space-y-2">
            <h3 className="text-sm font-medium">{t("users.serversTitle")}</h3>
            {allPerms ? (
              <p className="text-sm text-muted-foreground">{t("users.servers.allByPerm")}</p>
            ) : (
              <ServerPicker servers={servers} all={access.all} list={access.list} onChange={(all, list) => setAccess({ all, list })} />
            )}
          </section>
          <ErrorNote error={error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending || (!editing && (!username || password.length < 10))}>
              {pending && <Spinner />}
              {t(editing ? "common.save" : "users.add")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ResetPasswordDialog({ user, onClose }) {
  const { t } = useTranslation()
  const [password, setPassword] = useState(generatePassword)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  async function submit(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      await api.updateUser(user.id, { Password: password })
      toast.success(t("users.passwordSet"))
      onClose()
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle>{t("users.setPasswordTitle", { name: user.username })}</DialogTitle>
            <DialogDescription>{t("users.setPasswordText")}</DialogDescription>
          </DialogHeader>
          <PasswordField id="reset-pass" label={t("users.newPassword")} value={password} onChange={setPassword} help={t("users.passwordShare")} />
          <ErrorNote error={error} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending || password.length < 10}>
              {pending && <Spinner />}
              {t("users.setPassword")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default function UsersPage() {
  const { t } = useTranslation()
  const can = useCan()
  const { user: me } = useSession()
  const { servers } = useOutletContext()
  const users = useApi(api.users)
  const groups = useApi(api.permissions)
  const [dialog, setDialog] = useState(null)
  const [removing, setRemoving] = useState(null)
  const serverName = useMemo(() => Object.fromEntries((servers.data ?? []).map((s) => [s.id, s.name])), [servers.data])
  if (!can("panel.users")) return <Navigate to="/" replace />

  // Someone without "all" can't change a user who has it (the server refuses too).
  const editable = (u) => u.id !== me.id && (!u.permissions.includes("all") || me.permissions.includes("all"))
  const permCount = (u) => (u.permissions.includes("all") ? null : u.permissions.length)

  async function resetTotp(u) {
    try {
      await api.updateUser(u.id, { ResetTOTP: true })
      toast.success(t("users.totpReset"))
      users.reload()
    } catch (err) {
      toast.error(err.message)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("nav.users")}
        description={t("users.text")}
        actions={
          <Button onClick={() => setDialog({})} disabled={!groups.data}>
            <PlusIcon aria-hidden="true" />
            {t("users.add")}
          </Button>
        }
      />
      <Query query={users}>
        {(list) => (
          <div className="rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("auth.username")}</TableHead>
                  <TableHead>{t("users.permissionsTitle")}</TableHead>
                  <TableHead className="hidden md:table-cell">{t("users.serversTitle")}</TableHead>
                  <TableHead className="hidden sm:table-cell">2FA</TableHead>
                  <TableHead className="hidden lg:table-cell">{t("users.created")}</TableHead>
                  <TableHead className="w-10">
                    <span className="sr-only">{t("common.actions")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((u) => (
                  <TableRow key={u.id}>
                    <TableCell className="font-medium">
                      {u.username} {u.id === me.id && <span className="font-normal text-muted-foreground">({t("users.you")})</span>}
                    </TableCell>
                    <TableCell>
                      {permCount(u) === null ? (
                        <Badge variant="secondary" className="gap-1">
                          <ShieldCheckIcon aria-hidden="true" />
                          {t("perm.allShort")}
                        </Badge>
                      ) : (
                        <span className="text-sm">{t("perm.count", { count: permCount(u) })}</span>
                      )}
                    </TableCell>
                    <TableCell className="hidden max-w-64 truncate text-sm md:table-cell">
                      {u.allServers ? t("users.servers.allShort") : u.servers.length ? u.servers.map((id) => serverName[id] ?? "?").join(", ") : <span className="text-muted-foreground">{t("users.servers.noneShort")}</span>}
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">{u.totp ? t("users.on") : <span className="text-muted-foreground">{t("users.off")}</span>}</TableCell>
                    <TableCell className="hidden text-muted-foreground lg:table-cell">{formatDateTime(u.createdAt)}</TableCell>
                    <TableCell>
                      {editable(u) && (
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon-sm" aria-label={t("common.actions")}>
                              <EllipsisIcon aria-hidden="true" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-56">
                            <DropdownMenuItem onSelect={() => setDialog({ user: u })} disabled={!groups.data}>
                              <PencilIcon aria-hidden="true" />
                              {t("users.edit")}
                            </DropdownMenuItem>
                            <DropdownMenuItem onSelect={() => setDialog({ reset: u })}>
                              <KeyRoundIcon aria-hidden="true" />
                              {t("users.setPassword")}
                            </DropdownMenuItem>
                            {u.totp && <DropdownMenuItem onSelect={() => resetTotp(u)}>{t("users.resetTotp")}</DropdownMenuItem>}
                            <DropdownMenuSeparator />
                            <DropdownMenuItem variant="destructive" onSelect={() => setRemoving(u)}>
                              <Trash2Icon aria-hidden="true" />
                              {t("users.remove")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </Query>
      {dialog?.reset && <ResetPasswordDialog user={dialog.reset} onClose={() => setDialog(null)} />}
      {dialog && !dialog.reset && groups.data && (
        <UserDialog user={dialog.user} groups={groups.data} servers={servers.data ?? []} onClose={() => setDialog(null)} onSaved={users.reload} />
      )}
      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("users.removeTitle", { name: removing?.username })}
        description={t("users.removeText")}
        confirmLabel={t("users.remove")}
        destructive
        onConfirm={async () => {
          await api.deleteUser(removing.id)
          users.reload()
        }}
      />
    </div>
  )
}
