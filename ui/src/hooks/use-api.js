import { useCallback, useEffect, useRef, useState } from "react"

// useApi loads data and reloads it on demand or every `poll` ms (while the tab is visible).
export function useApi(fn, deps = [], { poll } = {}) {
  const [state, setState] = useState({ data: undefined, error: null, loading: true })
  const fnRef = useRef(fn)
  fnRef.current = fn
  const [nonce, setNonce] = useState(0)
  const key = JSON.stringify(deps)

  useEffect(() => {
    let cancelled = false
    fnRef
      .current()
      .then((data) => !cancelled && setState({ data, error: null, loading: false }))
      .catch((error) => !cancelled && setState((s) => ({ data: s.data, error, loading: false })))
    return () => {
      cancelled = true
    }
  }, [key, nonce])

  useEffect(() => {
    if (!poll) return
    const t = setInterval(() => document.visibilityState === "visible" && setNonce((n) => n + 1), poll)
    return () => clearInterval(t)
  }, [poll])

  const reload = useCallback(() => setNonce((n) => n + 1), [])
  return { ...state, reload, setData: (data) => setState((s) => ({ ...s, data })) }
}

// useAction wraps a mutation with a pending flag.
export function useAction(fn) {
  const [pending, setPending] = useState(false)
  const fnRef = useRef(fn)
  fnRef.current = fn
  const run = useCallback(async (...args) => {
    setPending(true)
    try {
      return await fnRef.current(...args)
    } finally {
      setPending(false)
    }
  }, [])
  return [run, pending]
}
