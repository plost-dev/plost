/** Which chart sits beside the globe on the site list. Kept per browser, like the site layout. */
export type OverviewChart = "hourly" | "heatmap" | "sites" | "metrics" | "daily"

export const OVERVIEW_CHARTS: ReadonlyArray<OverviewChart> = ["hourly", "daily", "heatmap", "sites", "metrics"]
export const OVERVIEW_CHART_LABEL: Record<OverviewChart, string> = {
  hourly: "24 小时柱形图",
  daily: "30 天柱形图",
  heatmap: "时段热力图",
  sites: "站点占比",
  metrics: "关键数字",
}

const STORAGE_KEY = "litestats:overview-chart"
const chartValue = (value: unknown): OverviewChart => OVERVIEW_CHARTS.includes(value as OverviewChart) ? value as OverviewChart : "hourly"

export function readOverviewChart(): OverviewChart {
  try { return typeof localStorage === "undefined" ? "hourly" : chartValue(localStorage.getItem(STORAGE_KEY)) }
  catch { return "hourly" }
}

export function saveOverviewChart(value: OverviewChart) {
  try { localStorage.setItem(STORAGE_KEY, chartValue(value)) } catch { /* The choice still applies until the page is closed. */ }
}
