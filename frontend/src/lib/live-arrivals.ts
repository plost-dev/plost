import { useEffect, useRef, useState } from "react"

const FLASH_MS = 3200

/**
 * Countries whose number of visitors online went up since the last refresh, for the map to flash. The first data a
 * map gets is only a baseline, so opening the page does not light up every country at once.
 */
export function useLiveArrivals(live: ReadonlyArray<{ country: string }>): ReadonlySet<string> {
  const previous = useRef<Map<string, number> | null>(null)
  const [arrivals, setArrivals] = useState<ReadonlySet<string>>(() => new Set())
  useEffect(() => {
    const counts = new Map<string, number>()
    for (const item of live) {
      const code = item.country.toUpperCase()
      counts.set(code, (counts.get(code) || 0) + 1)
    }
    const before = previous.current
    previous.current = counts
    if (!before) return
    const rose = new Set([...counts].filter(([code, n]) => n > (before.get(code) || 0)).map(([code]) => code))
    setArrivals(rose)
    if (!rose.size) return
    const timer = window.setTimeout(() => setArrivals(new Set()), FLASH_MS)
    return () => window.clearTimeout(timer)
  }, [live])
  return arrivals
}
