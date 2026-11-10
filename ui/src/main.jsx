import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter, Navigate, Route, Routes } from "react-router"
import "@fontsource-variable/geist"
import "@fontsource-variable/geist-mono"
import "@/index.css"
import "@/lib/i18n"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Loading } from "@/components/common"
import { Layout } from "@/components/layout"
import { ThemeProvider } from "@/components/theme-provider"
import { SessionProvider, useSession } from "@/lib/session"
import { LoginPage, SetupPage } from "@/pages/auth"
import ActivityPage from "@/pages/activity"
import AddServerPage from "@/pages/add-server"
import LatchPage from "@/pages/latch"
import OverviewPage from "@/pages/overview"
import ServerPage from "@/pages/server/server"
import SettingsPage from "@/pages/settings"
import StatisticsPage from "@/pages/statistics"
import UsersPage from "@/pages/users"

function App() {
  const { loading, setup, user } = useSession()
  if (loading)
    return (
      <div className="mx-auto max-w-md p-10">
        <Loading rows={2} />
      </div>
    )
  if (setup) return <SetupPage />
  if (!user) return <LoginPage />
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<OverviewPage />} />
        <Route path="servers/new" element={<AddServerPage />} />
        <Route path="servers/:id/:tab?" element={<ServerPage />} />
        <Route path="statistics" element={<StatisticsPage />} />
        <Route path="latch" element={<LatchPage />} />
        <Route path="activity" element={<ActivityPage />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

createRoot(document.getElementById("root")).render(
  <StrictMode>
    <ThemeProvider>
      <TooltipProvider>
        <BrowserRouter>
          <SessionProvider>
            <App />
          </SessionProvider>
        </BrowserRouter>
        <Toaster position="bottom-right" />
      </TooltipProvider>
    </ThemeProvider>
  </StrictMode>
)
