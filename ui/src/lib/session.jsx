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

const rank = { viewer: 1, admin: 2, owner: 3 }
// useCan reports whether the signed-in user has at least role.
export function useCan() {
  const { user } = useSession()
  return (role) => Boolean(user && rank[user.role] >= rank[role])
}
