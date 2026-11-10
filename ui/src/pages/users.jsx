import { useState } from "react"
import { Navigate } from "react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { EllipsisIcon, PlusIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmDialog, ErrorNote, PageHeader, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { formatDateTime } from "@/lib/format"
import { useCan, useSession } from "@/lib/session"

const ROLES = ["owner", "admin", "viewer"]

function UserDialog({ open, onOpenChange, user, onSaved }) {
  const { t } = useTranslation()
  const [form, setForm] = useState({ username: "", password: "", role: "admin" })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target ? e.target.value : e }))

  async function submit(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      if (user) await api.updateUser(user.id, { Password: form.password })
      else await api.createUser({ Username: form.username, Password: form.password, Role: form.role })
      toast.success(t(user ? "users.passwordSet" : "users.created"))
      onSaved()
      onOpenChange(false)
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle>{user ? t("users.setPasswordTitle", { name: user.username }) : t("users.addTitle")}</DialogTitle>
            <DialogDescription>{t(user ? "users.setPasswordText" : "users.addText")}</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            {!user && (
              <>
                <Field>
                  <FieldLabel htmlFor="u-name">{t("auth.username")}</FieldLabel>
                  <Input id="u-name" value={form.username} onChange={set("username")} required autoComplete="off" />
                </Field>
                <Field>
                  <FieldLabel htmlFor="u-role">{t("users.role")}</FieldLabel>
                  <Select value={form.role} onValueChange={set("role")}>
                    <SelectTrigger id="u-role" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent position="popper">
                      {ROLES.map((r) => (
                        <SelectItem key={r} value={r}>
                          {t(`role.${r}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FieldDescription>{t(`users.roleHelp.${form.role}`)}</FieldDescription>
                </Field>
              </>
            )}
            <Field>
              <FieldLabel htmlFor="u-pass">{t(user ? "users.newPassword" : "auth.password")}</FieldLabel>
              <Input id="u-pass" type="password" value={form.password} onChange={set("password")} required minLength={10} autoComplete="new-password" />
              <FieldDescription>{t("users.passwordShare")}</FieldDescription>
            </Field>
            <ErrorNote error={error} />
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending}>
              {pending && <Spinner />}
              {t(user ? "common.save" : "users.add")}
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
  const users = useApi(api.users)
  const [dialog, setDialog] = useState(null)
  const [removing, setRemoving] = useState(null)
  if (!can("owner")) return <Navigate to="/" replace />

  async function update(u, body, msg) {
    try {
      await api.updateUser(u.id, body)
      toast.success(msg)
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
          <Button onClick={() => setDialog({})}>
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
                  <TableHead>{t("users.role")}</TableHead>
                  <TableHead className="hidden sm:table-cell">2FA</TableHead>
                  <TableHead className="hidden md:table-cell">{t("users.created")}</TableHead>
                  <TableHead className="w-10">
                    <span className="sr-only">{t("common.actions")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((u) => (
                  <TableRow key={u.id}>
                    <TableCell className="font-medium">
                      {u.username} {u.id === me.id && <span className="text-muted-foreground">({t("users.you")})</span>}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{t(`role.${u.role}`)}</Badge>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">{u.totp ? t("users.on") : <span className="text-muted-foreground">{t("users.off")}</span>}</TableCell>
                    <TableCell className="hidden text-muted-foreground md:table-cell">{formatDateTime(u.createdAt)}</TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon-sm" aria-label={t("common.actions")}>
                            <EllipsisIcon aria-hidden="true" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-56">
                          <DropdownMenuLabel>{t("users.role")}</DropdownMenuLabel>
                          <DropdownMenuRadioGroup value={u.role} onValueChange={(r) => update(u, { Role: r }, t("users.roleChanged"))}>
                            {ROLES.map((r) => (
                              <DropdownMenuRadioItem key={r} value={r}>
                                {t(`role.${r}`)}
                              </DropdownMenuRadioItem>
                            ))}
                          </DropdownMenuRadioGroup>
                          <DropdownMenuSeparator />
                          {u.id !== me.id && <DropdownMenuItem onSelect={() => setDialog({ user: u })}>{t("users.setPassword")}</DropdownMenuItem>}
                          {u.totp && u.id !== me.id && <DropdownMenuItem onSelect={() => update(u, { ResetTOTP: true }, t("users.totpReset"))}>{t("users.resetTotp")}</DropdownMenuItem>}
                          {u.id !== me.id && (
                            <DropdownMenuItem variant="destructive" onSelect={() => setRemoving(u)}>
                              {t("users.remove")}
                            </DropdownMenuItem>
                          )}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </Query>
      <div className="grid gap-3 text-sm sm:grid-cols-3">
        {ROLES.map((r) => (
          <div key={r} className="rounded-lg border p-3">
            <p className="font-medium">{t(`role.${r}`)}</p>
            <p className="text-muted-foreground">{t(`users.roleHelp.${r}`)}</p>
          </div>
        ))}
      </div>
      {dialog && <UserDialog open onOpenChange={(o) => !o && setDialog(null)} user={dialog.user} onSaved={users.reload} />}
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
