import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { BoxIcon, EllipsisIcon, LockIcon, PlayIcon, PowerIcon, PowerOffIcon, RotateCwIcon, SquareIcon, Trash2Icon, UploadIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ConfirmDialog, EmptyState, Query } from "@/components/common"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { useCan } from "@/lib/session"

export default function ResourcesTab({ server }) {
  const { t } = useTranslation()
  const can = useCan()
  const query = useApi(() => api.resources(server.id), [server.id], { poll: 15000 })
  const [filter, setFilter] = useState("")
  const [uploading, setUploading] = useState(false)
  const [removing, setRemoving] = useState(null)
  const file = useRef(null)
  const running = server.status.state === "running"

  async function act(r, action) {
    try {
      await api.resourceAction(server.id, r.folder, action)
      toast.success(t(`resources.toast.${action}`, { name: r.folder }))
      query.reload()
    } catch (err) {
      toast.error(err.message)
    }
  }

  async function upload(e) {
    const f = e.target.files?.[0]
    e.target.value = ""
    if (!f) return
    setUploading(true)
    try {
      const res = await api.uploadResource(server.id, f)
      toast.success(t("resources.uploaded", { name: res.folder }))
      query.reload()
    } catch (err) {
      toast.error(err.message)
    } finally {
      setUploading(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("resources.search")} className="h-8 max-w-xs" aria-label={t("resources.search")} />
        <p className="text-sm text-muted-foreground">{t(running ? "resources.liveHint" : "resources.offHint")}</p>
        {can("resources.manage") && (
          <>
            <input ref={file} type="file" accept=".zip,application/zip" className="hidden" onChange={upload} />
            <Button size="sm" variant="outline" className="ml-auto" onClick={() => file.current?.click()} disabled={uploading}>
              {uploading ? <Spinner /> : <UploadIcon aria-hidden="true" />}
              {t("resources.upload")}
            </Button>
          </>
        )}
      </div>
      <Query query={query}>
        {(list) => {
          const f = filter.toLowerCase()
          const rows = list.filter((r) => !f || r.folder.toLowerCase().includes(f) || (r.name ?? "").toLowerCase().includes(f))
          if (!list.length) return <EmptyState icon={BoxIcon} title={t("resources.emptyTitle")} description={t("resources.emptyText")} />
          return (
            <div className="rounded-xl border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("resources.name")}</TableHead>
                    <TableHead className="hidden sm:table-cell">{t("resources.version")}</TableHead>
                    <TableHead className="hidden md:table-cell">{t("resources.dependsOn")}</TableHead>
                    <TableHead>{t("resources.status")}</TableHead>
                    <TableHead className="w-10">
                      <span className="sr-only">{t("common.actions")}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((r) => (
                    <TableRow key={r.folder} className={r.enabled ? undefined : "text-muted-foreground"}>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <span className="font-medium">{r.folder}</span>
                          {r.latch && (
                            <Badge variant="outline" className="gap-1 font-normal">
                              <LockIcon aria-hidden="true" />
                              Latch
                            </Badge>
                          )}
                        </div>
                        {r.author && <p className="text-xs text-muted-foreground">{r.author}</p>}
                      </TableCell>
                      <TableCell className="hidden font-mono text-sm sm:table-cell">{r.latch?.version ?? r.version ?? "—"}</TableCell>
                      <TableCell className="hidden max-w-xs truncate text-sm md:table-cell">{r.dependencies?.join(", ") || "—"}</TableCell>
                      <TableCell>
                        {!r.enabled ? (
                          <Badge variant="outline" className="font-normal">
                            {t("resources.disabled")}
                          </Badge>
                        ) : !r.runs ? (
                          <Badge variant="outline" className="font-normal" title={t("resources.noManifestHelp")}>
                            {t("resources.noManifest")}
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="font-normal">
                            {t("resources.enabled")}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell>
                        {can("resources.manage") && (
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="icon-sm" aria-label={t("common.actions")}>
                                <EllipsisIcon aria-hidden="true" />
                              </Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="w-52">
                              {r.enabled && running && (
                                <>
                                  <DropdownMenuItem onSelect={() => act(r, "restart")}>
                                    <RotateCwIcon aria-hidden="true" />
                                    {t("resources.restart")}
                                  </DropdownMenuItem>
                                  <DropdownMenuItem onSelect={() => act(r, "ensure")}>
                                    <PlayIcon aria-hidden="true" />
                                    {t("resources.ensure")}
                                  </DropdownMenuItem>
                                  <DropdownMenuItem onSelect={() => act(r, "stop")}>
                                    <SquareIcon aria-hidden="true" />
                                    {t("resources.stopNow")}
                                  </DropdownMenuItem>
                                  <DropdownMenuSeparator />
                                </>
                              )}
                              {r.enabled ? (
                                <DropdownMenuItem onSelect={() => act(r, "disable")}>
                                  <PowerOffIcon aria-hidden="true" />
                                  {t("resources.disable")}
                                </DropdownMenuItem>
                              ) : (
                                <DropdownMenuItem onSelect={() => act(r, "enable")}>
                                  <PowerIcon aria-hidden="true" />
                                  {t("resources.enable")}
                                </DropdownMenuItem>
                              )}
                              <DropdownMenuSeparator />
                              <DropdownMenuItem variant="destructive" onSelect={() => setRemoving(r)}>
                                <Trash2Icon aria-hidden="true" />
                                {t("resources.remove")}
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
          )
        }}
      </Query>
      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("resources.removeTitle", { name: removing?.folder })}
        description={t("resources.removeText")}
        confirmLabel={t("resources.remove")}
        destructive
        onConfirm={async () => {
          const res = await api.removeResource(server.id, removing.folder)
          toast.success(t("resources.removed", { name: removing.folder, path: res.movedTo }))
          query.reload()
        }}
      />
    </div>
  )
}

