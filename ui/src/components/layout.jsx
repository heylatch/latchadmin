import { Link, NavLink, Outlet, useLocation } from "react-router"
import { useTranslation } from "react-i18next"
import {
  ActivityIcon,
  ArrowUpCircleIcon,
  BellIcon,
  ChartLineIcon,
  ChevronsUpDownIcon,
  KeyRoundIcon,
  LanguagesIcon,
  LogOutIcon,
  MonitorIcon,
  MoonIcon,
  PlusIcon,
  ServerIcon,
  SettingsIcon,
  SirenIcon,
  SunIcon,
  UsersIcon,
} from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Separator } from "@/components/ui/separator"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { Logo } from "@/components/common"
import { useTheme } from "@/components/theme-provider"
import { useApi } from "@/hooks/use-api"
import { api } from "@/lib/api"
import { LANGUAGES } from "@/lib/i18n"
import { useCan, useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const dot = { running: "bg-success", starting: "bg-info", stopping: "bg-warning", updating: "bg-info", crashed: "bg-destructive", stopped: "bg-muted-foreground/50" }

function Nav({ servers }) {
  const { t } = useTranslation()
  const can = useCan()
  const { pathname } = useLocation()
  const item = (to, label, Icon, exact) => (
    <SidebarMenuItem key={to}>
      <SidebarMenuButton asChild isActive={exact ? pathname === to : pathname.startsWith(to)} tooltip={label}>
        <NavLink to={to}>
          <Icon aria-hidden="true" />
          <span>{label}</span>
        </NavLink>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
  return (
    <SidebarContent>
      <SidebarGroup>
        <SidebarGroupLabel>{t("nav.servers")}</SidebarGroupLabel>
        <SidebarMenu>
          {item("/", t("nav.overview"), ServerIcon, true)}
          {item("/statistics", t("nav.statistics"), ChartLineIcon)}
          {(servers ?? []).map((s) => (
            <SidebarMenuItem key={s.id}>
              <SidebarMenuButton asChild isActive={pathname.startsWith(`/servers/${s.id}`)} tooltip={s.name}>
                <NavLink to={`/servers/${s.id}`}>
                  <span className="flex size-4 items-center justify-center" aria-hidden="true">
                    <span className={cn("size-2 rounded-full", dot[s.status.state] ?? dot.stopped)} />
                  </span>
                  <span className="truncate">{s.name}</span>
                  <span className="sr-only">{t(`state.${s.status.state}`)}</span>
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
          {can("server.add") && item("/servers/new", t("nav.addServer"), PlusIcon)}
        </SidebarMenu>
      </SidebarGroup>
      <SidebarGroup>
        <SidebarGroupLabel>{t("nav.panel")}</SidebarGroupLabel>
        <SidebarMenu>
          {item("/latch", "Latch", KeyRoundIcon)}
          {can("panel.activity") && item("/activity", t("nav.activity"), ActivityIcon)}
          {can("panel.settings") && item("/notifications", t("nav.notifications"), BellIcon)}
          {can("panel.users") && item("/users", t("nav.users"), UsersIcon)}
          {item("/settings", t("nav.settings"), SettingsIcon)}
        </SidebarMenu>
      </SidebarGroup>
    </SidebarContent>
  )
}

function UserMenu() {
  const { t } = useTranslation()
  const { user, refresh } = useSession()
  async function logout() {
    await api.logout().catch(() => {})
    refresh()
  }
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg">
              <div className="flex size-8 items-center justify-center rounded-lg bg-muted text-xs font-medium uppercase">{user.username.slice(0, 2)}</div>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">{user.username}</span>
                <span className="truncate text-xs text-muted-foreground">{user.permissions.includes("all") ? t("perm.allShort") : t("perm.count", { count: user.permissions.length })}</span>
              </div>
              <ChevronsUpDownIcon className="ml-auto size-4" aria-hidden="true" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" align="start" className="w-56">
            <DropdownMenuItem asChild>
              <Link to="/settings">
                <SettingsIcon aria-hidden="true" />
                {t("nav.myAccount")}
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={logout}>
              <LogOutIcon aria-hidden="true" />
              {t("nav.logout")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}

export function Prefs() {
  const { t, i18n } = useTranslation()
  const { theme, setTheme } = useTheme()
  const ThemeIcon = theme === "dark" ? MoonIcon : theme === "light" ? SunIcon : MonitorIcon
  return (
    <div className="flex items-center gap-1">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon-sm" aria-label={t("prefs.language")}>
            <LanguagesIcon aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuRadioGroup value={i18n.resolvedLanguage} onValueChange={(v) => i18n.changeLanguage(v)}>
            {LANGUAGES.map((l) => (
              <DropdownMenuRadioItem key={l.id} value={l.id}>
                {l.label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon-sm" aria-label={t("prefs.theme")}>
            <ThemeIcon aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel>{t("prefs.theme")}</DropdownMenuLabel>
          <DropdownMenuRadioGroup value={theme} onValueChange={setTheme}>
            {["system", "light", "dark"].map((v) => (
              <DropdownMenuRadioItem key={v} value={v}>
                {t(`prefs.${v}`)}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

// newer reports whether version a is higher than b ("1.12.3" > "1.12.2").
function newer(a, b) {
  const pa = String(a ?? "").split(".").map(Number)
  const pb = String(b ?? "").split(".").map(Number)
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] || 0) - (pb[i] || 0)
    if (d) return d > 0
  }
  return false
}

// UpdateBanners: a card for every available update. Urgent ones (marked by the Latch team) are red and shown to
// everyone; the rest are calm and shown to those who can install them. The card for a page's own update is left
// out on that page (the panel's on Settings, a server's on its Updates tab).
function UpdateBanners({ panel, servers, pathname }) {
  const { t } = useTranslation()
  const can = useCan()
  const items = []

  const pu = panel?.update
  const latestPanel = pu?.info?.latest
  if (pu && !pu.dev && latestPanel && newer(latestPanel, pu.current) && pathname !== "/settings") {
    const urgent = pu.info.urgent
    if (urgent || can("panel.update")) {
      items.push({
        key: "panel",
        urgent: Boolean(urgent),
        title: urgent ? t("banner.panelTitle", { version: urgent.version }) : t("banner.panelAvailable", { version: latestPanel }),
        note: urgent ? urgent.note || t("banner.noNote") : pu.pending ? t("banner.panelPending", { version: pu.pending }) : t("banner.panelCurrent", { version: pu.current }),
        to: "/settings",
        action: can("panel.update") ? t("banner.panelAction") : null,
      })
    }
  }

  // Game updates: one card per game and version, however many servers it concerns.
  const groups = new Map()
  for (const s of servers ?? []) {
    const latest = s.update?.latest
    if (!latest || !s.version || !newer(latest, s.version) || pathname.startsWith(`/servers/${s.id}/updates`)) continue
    const urgent = s.update.urgent
    if (!urgent && !can("updates.game")) continue
    const key = `${s.game}:${latest}`
    const g = groups.get(key) ?? { key, game: s.gameName, version: latest, urgent: null, servers: [] }
    g.urgent ??= urgent
    g.servers.push(s)
    groups.set(key, g)
  }
  for (const g of groups.values()) {
    const one = g.servers.length === 1 ? g.servers[0] : null
    items.push({
      key: g.key,
      urgent: Boolean(g.urgent),
      title: g.urgent
        ? one
          ? t("banner.gameTitle", { server: one.name, game: g.game, version: g.version })
          : t("banner.gameTitleMany", { game: g.game, version: g.version, count: g.servers.length })
        : t("banner.gameAvailable", { game: g.game, version: g.version }),
      note: g.urgent?.note || (g.urgent ? t("banner.noNote") : t("banner.gameOn", { servers: g.servers.map((s) => s.name).join(", "), count: g.servers.length })),
      to: one ? `/servers/${one.id}/updates` : "/",
      action: can("updates.game") ? (one ? t("banner.gameAction") : t("banner.gameActionMany")) : null,
    })
  }

  if (!items.length) return null
  items.sort((a, b) => Number(b.urgent) - Number(a.urgent))
  return (
    <div className="space-y-3">
      {items.map((i) => (
        <Alert key={i.key} variant={i.urgent ? "destructive" : "default"} className={i.urgent ? undefined : "bg-muted/30"}>
          {i.urgent ? <SirenIcon aria-hidden="true" /> : <ArrowUpCircleIcon aria-hidden="true" />}
          <AlertTitle>{i.title}</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{i.note}</span>
            {i.action && (
              <Button size="sm" variant={i.urgent ? "outline" : "secondary"} className="no-underline!" asChild>
                <Link to={i.to}>{i.action}</Link>
              </Button>
            )}
          </AlertDescription>
        </Alert>
      ))}
    </div>
  )
}

export function Layout() {
  const servers = useApi(api.servers, [], { poll: 5000 })
  const panel = useApi(api.panel, [], { poll: 60000 })
  const { pathname } = useLocation()
  return (
    <SidebarProvider>
      <Sidebar variant="inset" collapsible="icon">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" asChild>
                <Link to="/">
                  {/* The sidebar button shrinks every svg inside it to 16 px; the logo overrides that. */}
                  <span className="flex size-8 shrink-0">
                    <Logo className="size-full!" />
                  </span>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-semibold">LatchAdmin</span>
                    <span className="truncate text-xs text-muted-foreground">{panel.data?.settings?.panelName ?? ""}</span>
                  </div>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <Nav servers={servers.data} />
        <SidebarFooter>
          <UserMenu />
        </SidebarFooter>
      </Sidebar>
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 data-vertical:h-4 data-vertical:self-center" />
          <span className="truncate text-sm text-muted-foreground">{panel.data ? `v${panel.data.version}` : ""}</span>
          <div className="ml-auto">
            <Prefs />
          </div>
        </header>
        <main className="flex-1 p-4 md:p-6 lg:p-8">
          <div className="mx-auto w-full max-w-6xl space-y-6 pb-16">
            <UpdateBanners panel={panel.data} servers={servers.data} pathname={pathname} />
            <Outlet context={{ servers, panel }} />
          </div>
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
