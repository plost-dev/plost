import { useEffect, useMemo, useRef, useState } from "react"

const FLASH_MS = 3200

/** A visitor online now placed by city (the report's livePoints). */
export type LivePoint = { lat: number; lon: number; country: string; count: number; arrived?: number }
export const pointKey = (p: LivePoint) => `${p.lat},${p.lon}`

/**
 * The keys whose value went up since the last refresh, for the map to flash. The first data a map gets is only a
 * baseline, so opening the page does not light up everything at once.
 */
function useRising(counts: ReadonlyMap<string, number>): ReadonlySet<string> {
  const previous = useRef<ReadonlyMap<string, number> | null>(null)
  const [rising, setRising] = useState<ReadonlySet<string>>(() => new Set())
  // A page renders more than once per refresh; only a real change in the counts may start or end a flash.
  const signature = [...counts].sort(([a], [b]) => a < b ? -1 : 1).map(([k, n]) => `${k}=${n}`).join(";")
  useEffect(() => {
    const before = previous.current
    previous.current = counts
    if (!before) return
    const rose = new Set([...counts].filter(([key, n]) => n > (before.get(key) || 0)).map(([key]) => key))
    setRising(rose)
    if (!rose.size) return
    const timer = window.setTimeout(() => setRising(new Set()), FLASH_MS)
    return () => window.clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- counts is read through its signature
  }, [signature])
  return rising
}

/** Countries whose number of visitors online went up since the last refresh. */
export function useLiveArrivals(live: ReadonlyArray<{ country: string }>): ReadonlySet<string> {
  const counts = useMemo(() => {
    const m = new Map<string, number>()
    for (const item of live) { const code = item.country.toUpperCase(); m.set(code, (m.get(code) || 0) + 1) }
    return m
  }, [live])
  return useRising(counts)
}

/**
 * Places (pointKey) where a new visitor came online since the last refresh: the newest arrival there is later than
 * before. Only those flash, and one visitor leaving while another arrives still counts as an arrival.
 */
export function usePointArrivals(points: ReadonlyArray<LivePoint>): ReadonlySet<string> {
  const arrivals = useMemo(() => new Map(points.map(p => [pointKey(p), p.arrived ?? p.count])), [points])
  return useRising(arrivals)
}

/**
 * The visitors online that could not be placed by city (no City GeoIP database, or an address it does not place),
 * as one entry per visitor of their country, so the maps can still show them at the country.
 */
export function unplacedLive(live: ReadonlyArray<{ country: string }>, points: ReadonlyArray<LivePoint>): Array<{ country: string }> {
  if (!points.length) return [...live]
  const placed = new Map<string, number>()
  for (const p of points) placed.set(p.country.toUpperCase(), (placed.get(p.country.toUpperCase()) || 0) + p.count)
  const out: Array<{ country: string }> = []
  for (const item of live) {
    const code = item.country.toUpperCase(), left = placed.get(code) || 0
    if (left > 0) placed.set(code, left - 1)
    else out.push(item)
  }
  return out
}

/** The size of a place's dot: a little larger for more visitors there, without swallowing its neighbours. */
export const pointRadius = (count: number) => 3.4 + Math.min(4, Math.log2(count + 1) * 1.2)

/** The HTML dot Mapbox and Google Maps put at a place; it sends out rings only when a visitor just arrived there. */
export function liveDot(point: LivePoint, arriving: boolean, t: (key: string, params?: Record<string, string | number>) => string): HTMLElement {
  const dot = document.createElement("span")
  dot.className = `map-live-point${arriving ? " is-arrival" : ""}`
  dot.style.setProperty("--point-size", `${pointRadius(point.count) * 2}px`)
  dot.title = t("{0} 位访客在线", { 0: point.count })
  dot.setAttribute("aria-label", dot.title)
  return dot
}
