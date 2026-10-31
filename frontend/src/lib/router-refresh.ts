import { useEffect, useRef } from "react"
import { useRouter } from "@tanstack/react-router"

type RefreshOptions = {
  intervalMs?: number
  onError?: () => void
  scheduler?: Pick<Window, "setInterval" | "clearInterval">
  visibility?: Pick<Document, "visibilityState">
}

export function startRouterRefresh(refresh: () => Promise<unknown>, {
  intervalMs = 30_000,
  onError,
  scheduler = window,
  visibility = document,
}: RefreshOptions = {}) {
  let disposed = false
  let refreshing = false
  const timer = scheduler.setInterval(() => {
    if (disposed || refreshing || visibility.visibilityState !== "visible") return
    refreshing = true
    void refresh().catch(() => onError?.()).finally(() => { refreshing = false })
  }, intervalMs)
  return () => {
    disposed = true
    scheduler.clearInterval(timer)
  }
}

/** `skip` holds a tick back while data a loader handed over is still loading, so slow refreshes never pile up. */
export function useRouterAutoRefresh({enabled = true, intervalMs, onError, skip}: {enabled?: boolean; intervalMs?: number; onError?: () => void; skip?: () => boolean} = {}) {
  const router = useRouter()
  const errorRef = useRef(onError)
  errorRef.current = onError
  const skipRef = useRef(skip)
  skipRef.current = skip
  useEffect(() => {
    if (!enabled) return
    return startRouterRefresh(async () => { if (!skipRef.current?.()) await router.invalidate() }, {intervalMs, onError: () => errorRef.current?.()})
  }, [router, enabled, intervalMs])
}
