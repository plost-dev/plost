import { useEffect, useRef } from "react"

/**
 * Listens to the server's change stream and calls onChange whenever new visits were stored, at most every few
 * seconds. A change heard while the tab is hidden is applied once it is shown again. EventSource reconnects by itself.
 */
export function useLiveChanges(onChange: () => void) {
  const ref = useRef(onChange)
  ref.current = onChange
  useEffect(() => {
    if (typeof EventSource === "undefined") return
    const source = new EventSource("/api/changes")
    let missed = false
    const onServerChange = () => {
      if (document.visibilityState === "visible") ref.current()
      else missed = true
    }
    const onVisible = () => {
      if (!missed || document.visibilityState !== "visible") return
      missed = false
      ref.current()
    }
    source.addEventListener("change", onServerChange)
    document.addEventListener("visibilitychange", onVisible)
    return () => {
      source.close()
      document.removeEventListener("visibilitychange", onVisible)
    }
  }, [])
}
