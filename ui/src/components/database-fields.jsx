import { useTranslation } from "react-i18next"
import { ArrowUpRightIcon } from "lucide-react"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"

// The database a game server uses: either LatchAdmin creates it with an administrator login of MySQL/MariaDB
// (XAMPP's root, for example; used once, never saved), or the owner enters an existing database's login.
export const emptyDatabase = () => ({
  mode: "create",
  conn: { host: "127.0.0.1", port: 3306, user: "root", password: "", database: "" },
  database: "arcanemp",
  user: "arcanemp",
})

// toRequest turns the form into what the API takes.
export const databaseRequest = (v) => ({ ...v, conn: { ...v.conn, port: Number(v.conn.port) || 3306, database: v.mode === "existing" ? v.conn.database : "" } })

export function DatabaseFields({ value, onChange, disabled, idPrefix = "db" }) {
  const { t } = useTranslation()
  const set = (k, v) => onChange({ ...value, [k]: v })
  const setConn = (k, v) => onChange({ ...value, conn: { ...value.conn, [k]: v } })
  const create = value.mode === "create"
  const id = (k) => `${idPrefix}-${k}`
  return (
    <div className="space-y-4">
      <RadioGroup
        value={value.mode}
        onValueChange={(mode) => onChange({ ...value, mode, conn: { ...value.conn, user: mode === "create" ? "root" : "", password: "" } })}
        className="grid gap-2 sm:grid-cols-2"
        disabled={disabled}
      >
        {["create", "existing"].map((m) => (
          <label key={m} htmlFor={id(m)} className="flex items-start gap-3 rounded-lg border p-3 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring">
            <RadioGroupItem id={id(m)} value={m} className="mt-0.5" />
            <span>
              <span className="block text-sm font-medium">{t(`db.mode.${m}`)}</span>
              <span className="block text-sm text-muted-foreground">{t(`db.mode.${m}Help`)}</span>
            </span>
          </label>
        ))}
      </RadioGroup>

      <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_7rem]">
        <Field>
          <FieldLabel htmlFor={id("host")}>{t("db.host")}</FieldLabel>
          <Input id={id("host")} value={value.conn.host} onChange={(e) => setConn("host", e.target.value)} disabled={disabled} spellCheck={false} className="font-mono" />
        </Field>
        <Field>
          <FieldLabel htmlFor={id("port")}>{t("db.port")}</FieldLabel>
          <Input id={id("port")} inputMode="numeric" value={value.conn.port} onChange={(e) => setConn("port", e.target.value.replace(/\D/g, ""))} disabled={disabled} className="font-mono" />
        </Field>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor={id("user")}>{create ? t("db.adminUser") : t("db.user")}</FieldLabel>
          <Input id={id("user")} value={value.conn.user} onChange={(e) => setConn("user", e.target.value)} disabled={disabled} autoComplete="off" spellCheck={false} />
        </Field>
        <Field>
          <FieldLabel htmlFor={id("password")}>{create ? t("db.adminPassword") : t("db.password")}</FieldLabel>
          <Input id={id("password")} type="password" value={value.conn.password} onChange={(e) => setConn("password", e.target.value)} disabled={disabled} autoComplete="new-password" />
        </Field>
      </div>
      {create ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor={id("newdb")}>{t("db.newDatabase")}</FieldLabel>
            <Input id={id("newdb")} value={value.database} onChange={(e) => set("database", e.target.value)} disabled={disabled} spellCheck={false} className="font-mono" />
          </Field>
          <Field>
            <FieldLabel htmlFor={id("newuser")}>{t("db.newUser")}</FieldLabel>
            <Input id={id("newuser")} value={value.user} onChange={(e) => set("user", e.target.value)} disabled={disabled} spellCheck={false} className="font-mono" />
          </Field>
          <FieldDescription className="sm:col-span-2">{t("db.createHelp")}</FieldDescription>
        </div>
      ) : (
        <Field>
          <FieldLabel htmlFor={id("database")}>{t("db.database")}</FieldLabel>
          <Input id={id("database")} value={value.conn.database} onChange={(e) => setConn("database", e.target.value)} disabled={disabled} spellCheck={false} className="font-mono" />
        </Field>
      )}
      <p className="text-sm text-muted-foreground">
        {t("db.noServer")}{" "}
        <a href="https://www.apachefriends.org/" target="_blank" rel="noreferrer" className="inline-flex items-center gap-0.5 font-medium text-foreground underline-offset-4 hover:underline">
          XAMPP
          <ArrowUpRightIcon className="size-3" aria-hidden="true" />
        </a>{" "}
        {t("db.or")}{" "}
        <a href="https://mariadb.org/download/" target="_blank" rel="noreferrer" className="inline-flex items-center gap-0.5 font-medium text-foreground underline-offset-4 hover:underline">
          MariaDB
          <ArrowUpRightIcon className="size-3" aria-hidden="true" />
        </a>
      </p>
    </div>
  )
}
