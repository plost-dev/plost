import type { TrafficMetric } from "~/components/TrendChart"

/**
 * Which trend series are left out of the chart. Visits start hidden: within an hour it is nearly always equal to
 * visitors, so it only sits behind that line, while the totals above the chart still show all three. The legend and the
 * options menu turn each one on or off. Kept per browser, like the chart type.
 */
export const DEFAULT_HIDDEN_SERIES: ReadonlyArray<TrafficMetric> = ["visits"]

const ALL: ReadonlyArray<TrafficMetric> = ["visitors", "visits", "pageviews"]
const STORAGE_KEY = "litestats:chart-hidden-series"

export function readHiddenSeries(): ReadonlyArray<TrafficMetric> {
  try {
    const raw = typeof localStorage === "undefined" ? null : localStorage.getItem(STORAGE_KEY)
    if (raw === null) return DEFAULT_HIDDEN_SERIES
    const parsed: unknown = JSON.parse(raw)
    const hidden = Array.isArray(parsed) ? ALL.filter(key => parsed.includes(key)) : []
    return hidden.length < ALL.length ? hidden : DEFAULT_HIDDEN_SERIES
  } catch { return DEFAULT_HIDDEN_SERIES }
}

export function saveHiddenSeries(value: ReadonlyArray<TrafficMetric>) {
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(value)) } catch { /* The choice still applies until the page is closed. */ }
}
