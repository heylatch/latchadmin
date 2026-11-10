import { createContext, useCallback, useContext, useLayoutEffect, useMemo, useState, useSyncExternalStore } from "react"

const STORAGE_KEY = "latchadmin-theme"
const query = "(prefers-color-scheme: dark)"
const ThemeContext = createContext({ theme: "system", resolvedTheme: "light", setTheme: () => {} })

function subscribe(callback) {
  const mql = window.matchMedia(query)
  mql.addEventListener("change", callback)
  return () => mql.removeEventListener("change", callback)
}

function readStored() {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? "system"
  } catch {
    return "system"
  }
}

// public/theme.js sets the class before the first paint; this keeps it in sync afterwards.
export function ThemeProvider({ children }) {
  const [theme, setThemeState] = useState(readStored)
  const systemDark = useSyncExternalStore(subscribe, () => window.matchMedia(query).matches)
  const resolvedTheme = theme === "system" ? (systemDark ? "dark" : "light") : theme

  useLayoutEffect(() => {
    const root = document.documentElement
    root.classList.toggle("dark", resolvedTheme === "dark")
    root.style.colorScheme = resolvedTheme
  }, [resolvedTheme])

  const setTheme = useCallback((next) => {
    setThemeState(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // storage blocked: this tab only
    }
  }, [])

  const value = useMemo(() => ({ theme, resolvedTheme, setTheme }), [theme, resolvedTheme, setTheme])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme() {
  return useContext(ThemeContext)
}
