import { overviewChartsFn, overviewLiveFn, type OverviewCharts } from "./actions"
import { overviewMapFn, type OverviewMapConfig } from "./map-usage-actions"
import { loadWorldMap } from "./world-countries"
import type { OverviewPlace } from "~/components/OverviewGlobe"

// The site overview's own requests start with the page's loader instead of after the list has drawn, so on a slow
// connection they travel together rather than one round trip after another. Each is handed over once and only while
// fresh; anything later asks the server again.

type Started = { at: number, live: Promise<OverviewPlace[]>, charts: Promise<OverviewCharts>, map: Promise<OverviewMapConfig> }
let started: Started | null = null
const FRESH_MS = 10_000
const quiet = <T,>(promise: Promise<T>) => { promise.catch(() => {}); return promise }

export function prefetchOverview() {
  started = { at: Date.now(), live: quiet(overviewLiveFn()), charts: quiet(overviewChartsFn()), map: quiet(overviewMapFn()) }
  // The built-in globe's land shapes, which the overview draws first.
  void loadWorldMap().catch(() => {})
}

/** The prefetched requests if they are still fresh, given out once. */
export function takeOverviewPrefetch() {
  const taken = started && Date.now() - started.at < FRESH_MS ? started : null
  started = null
  return taken
}
