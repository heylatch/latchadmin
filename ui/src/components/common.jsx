import { useState } from "react"
import { useTranslation } from "react-i18next"
import { CircleAlertIcon } from "lucide-react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"

export function Logo({ className }) {
  return (
    <svg viewBox="0 0 32 32" className={cn("size-8", className)} aria-hidden="true">
      <rect width="32" height="32" rx="8" className="fill-foreground" />
      <g transform="translate(5.12 5.12) scale(0.9067)" className="fill-background">
        <path d="M5.25 4h4.75v11.25h5.5V20H5.25z" />
        <path d="M16.75 15.25h3.5V20h-3.5z" />
      </g>
    </svg>
  )
}

export function PageHeader({ title, description, actions, className }) {
  return (
    <div className={cn("flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between", className)}>
      <div className="min-w-0 space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight break-words">{title}</h1>
        {description && <p className="max-w-2xl text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

export function EmptyState({ icon: Icon, title, description, children, className }) {
  return (
    <div className={cn("flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed px-6 py-12 text-center", className)}>
      {Icon && (
        <div className="flex size-10 items-center justify-center rounded-lg bg-muted">
          <Icon className="size-5 text-muted-foreground" aria-hidden="true" />
        </div>
      )}
      <div className="space-y-1">
        <p className="font-medium">{title}</p>
        {description && <p className="mx-auto max-w-md text-sm text-muted-foreground">{description}</p>}
      </div>
      {children && <div className="mt-2 flex flex-wrap justify-center gap-2">{children}</div>}
    </div>
  )
}

export function ErrorNote({ error, className }) {
  if (!error) return null
  return (
    <Alert variant="destructive" className={className}>
      <CircleAlertIcon aria-hidden="true" />
      <AlertDescription>{typeof error === "string" ? error : error.message}</AlertDescription>
    </Alert>
  )
}

export function Loading({ rows = 3 }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-16 w-full" />
      ))}
    </div>
  )
}

// Query renders loading, error (with retry) or the data.
export function Query({ query, rows, children }) {
  const { t } = useTranslation()
  if (query.data === undefined && query.loading) return <Loading rows={rows} />
  if (query.data === undefined && query.error)
    return (
      <div className="space-y-3">
        <ErrorNote error={query.error} />
        <Button variant="outline" size="sm" onClick={query.reload}>
          {t("common.retry")}
        </Button>
      </div>
    )
  return children(query.data)
}

const tones = {
  running: "bg-success",
  starting: "bg-info animate-pulse",
  stopping: "bg-warning animate-pulse",
  updating: "bg-info animate-pulse",
  stopped: "bg-muted-foreground/60",
  crashed: "bg-destructive",
}

// Color never carries meaning alone: every badge has a label.
export function StateBadge({ state, className }) {
  const { t } = useTranslation()
  return (
    <Badge variant="outline" className={cn("gap-1.5 font-normal", className)}>
      <span aria-hidden="true" className={cn("size-1.5 rounded-full", tones[state] ?? tones.stopped)} />
      {t(`state.${state}`)}
    </Badge>
  )
}

// ConfirmDialog asks before running a destructive or disruptive action. Controlled: open/onOpenChange.
export function ConfirmDialog({ open, onOpenChange, title, description, confirmLabel, destructive, onConfirm, children }) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState(null)
  async function run(e) {
    e.preventDefault()
    setPending(true)
    setError(null)
    try {
      await onConfirm()
      onOpenChange(false)
    } catch (err) {
      setError(err)
    } finally {
      setPending(false)
    }
  }
  return (
    <AlertDialog open={open} onOpenChange={(o) => !pending && (setError(null), onOpenChange(o))}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && <AlertDialogDescription>{description}</AlertDialogDescription>}
        </AlertDialogHeader>
        {children}
        <ErrorNote error={error} />
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={run} disabled={pending} variant={destructive ? "destructive" : "default"}>
            {pending && <Spinner />}
            {confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

// ConfirmButton is a button that opens a ConfirmDialog.
export function ConfirmButton({ title, description, confirmLabel, destructive, onConfirm, children, ...props }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button {...props} onClick={() => setOpen(true)}>
        {children}
      </Button>
      <ConfirmDialog open={open} onOpenChange={setOpen} title={title} description={description} confirmLabel={confirmLabel} destructive={destructive} onConfirm={onConfirm} />
    </>
  )
}

export function Section({ title, description, action, children, className }) {
  return (
    <section className={cn("space-y-3", className)}>
      <div className="flex items-end justify-between gap-3">
        <div>
          <h2 className="text-sm font-medium">{title}</h2>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}
