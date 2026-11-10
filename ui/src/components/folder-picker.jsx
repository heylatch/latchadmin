import { useState } from "react"
import { useTranslation } from "react-i18next"
import { ArrowUpIcon, FolderIcon, HardDriveIcon, ServerIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { ErrorNote, Loading } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"

// FolderPicker browses folders on the machine LatchAdmin runs on (the browser can't see that machine's disks).
export function FolderPicker({ open, onOpenChange, start, onPick, pickLabel }) {
  const { t } = useTranslation()
  const [path, setPath] = useState(start ?? "")
  const listing = useApi(() => (open ? api.browse(path) : Promise.resolve(null)), [path, open])
  const l = listing.data

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("picker.title")}</DialogTitle>
          <DialogDescription>{t("picker.text")}</DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="icon-sm" disabled={!l?.path} onClick={() => setPath(l?.parent ?? "")} aria-label={t("picker.up")}>
            <ArrowUpIcon aria-hidden="true" />
          </Button>
          <code className="min-w-0 flex-1 truncate rounded-md bg-muted px-2 py-1 text-xs">{l?.path || t("picker.drives")}</code>
        </div>
        <div className="h-80 overflow-y-auto rounded-lg border">
          {listing.error ? (
            <div className="p-3">
              <ErrorNote error={listing.error} />
            </div>
          ) : !l ? (
            <div className="p-3">
              <Loading rows={4} />
            </div>
          ) : l.entries?.length ? (
            <ul className="divide-y">
              {l.entries.map((e) => (
                <li key={e.path}>
                  <button type="button" onClick={() => setPath(e.path)} className="flex w-full items-center gap-2.5 px-3 py-2 text-left text-sm hover:bg-muted/60 focus-visible:bg-muted focus-visible:outline-none">
                    {e.server ? (
                      <ServerIcon className="size-4 text-foreground" aria-hidden="true" />
                    ) : l.path ? (
                      <FolderIcon className="size-4 text-muted-foreground" aria-hidden="true" />
                    ) : (
                      <HardDriveIcon className="size-4 text-muted-foreground" aria-hidden="true" />
                    )}
                    <span className="min-w-0 flex-1 truncate">{e.name}</span>
                    {e.server && <Badge variant="outline">{t("picker.serverHere")}</Badge>}
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="p-4 text-sm text-muted-foreground">{t("picker.empty")}</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={!l?.path} onClick={() => (onPick(l.path), onOpenChange(false))}>
            {pickLabel ?? t("picker.pick")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
