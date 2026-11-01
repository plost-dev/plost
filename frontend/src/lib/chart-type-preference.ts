/** How the visit trend is drawn: soft lines with areas, or nested bars. Kept per browser, like the chart interval. */
export type ChartType = "line" | "bar"

export const CHART_TYPES: ReadonlyArray<ChartType> = ["line", "bar"]
export const CHART_TYPE_LABEL: Record<ChartType, string> = { line: "曲线", bar: "柱形" }

const STORAGE_KEY = "litestats:chart-type"

export function readChartType(): ChartType {
  try { return typeof localStorage !== "undefined" && localStorage.getItem(STORAGE_KEY) === "bar" ? "bar" : "line" }
  catch { return "line" }
}

export function saveChartType(value: ChartType) {
  try { localStorage.setItem(STORAGE_KEY, value) } catch { /* The choice still applies until the page is closed. */ }
}
