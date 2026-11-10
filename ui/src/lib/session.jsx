import { createContext, useCallback, useContext, useEffect, useState } from "react"
import { api, setUnauthorizedHandler } from "@/lib/api"

const SessionContext = createContext(null)

export function SessionProvider({ children }) {
  const [state, setState] = useState({ loading: true, setup: false, user: null, version: "" })

  const refresh = useCallback(async () => {
    try {
      const s = await api.session()
      setState({ loading: false, setup: s.setup, user: s.user ?? null, version: s.version })
    } catch (error) {
      setState((p) => ({ ...p, loading: false, error }))
    }
  }, [])

  useEffect(() => {
    refresh()
    setUnauthorizedHandler(() => setState((p) => ({ ...p, user: null })))
  }, [refresh])

  return <SessionContext.Provider value={{ ...state, refresh, setUser: (user) => setState((p) => ({ ...p, user, setup: false })) }}>{children}</SessionContext.Provider>
}

export const useSession = () => useContext(SessionContext)

// Having the first also grants the second.
const implied = { "console.write": "console.view", "config.edit": "config.view" }

export const hasPerm = (list, perm) => Boolean(list?.some((p) => p === "all" || p === perm || implied[p] === perm))

// useCan reports whether the signed-in user holds a permission.
export function useCan() {
  const { user } = useSession()
  return (perm) => hasPerm(user?.permissions, perm)
}
