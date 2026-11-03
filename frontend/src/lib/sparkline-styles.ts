import { rpc } from "./rpc"

// The small trend on each site card. Saved for the account; the server keeps the same list (ui.go).
export const SPARKLINE_STYLES = [
  { id: "area-line", label: "折线 + 面积" },
  { id: "bars", label: "柱形" },
  { id: "bars-today", label: "柱形，最新一根高亮" },
  { id: "bars-recent", label: "柱形，越近越深" },
  { id: "barcode", label: "细竖线" },
  { id: "bars-average", label: "柱形 + 均线" },
  { id: "bars-cap", label: "淡柱，顶部描色" },
  { id: "line", label: "细折线 + 末点" },
  { id: "line-faint", label: "折线 + 淡面积" },
  { id: "area", label: "只有面积" },
  { id: "step", label: "阶梯线" },
  { id: "dots", label: "圆点" },
  { id: "heat", label: "热力格" },
] as const
export const SPARKLINE_RANGES = [
  { id: "24h", label: "24 小时" },
  { id: "30d", label: "30 天" },
] as const

export type SparklineStyle = typeof SPARKLINE_STYLES[number]["id"]
export type SparklineRange = typeof SPARKLINE_RANGES[number]["id"]
export type SparklinePreference = { style: SparklineStyle; range: SparklineRange }
export const DEFAULT_SPARKLINE: SparklinePreference = { style: "area-line", range: "24h" }

export function normalizeSparkline(style: unknown, range: unknown): SparklinePreference {
  return {
    style: SPARKLINE_STYLES.some(s => s.id === style) ? style as SparklineStyle : DEFAULT_SPARKLINE.style,
    range: range === "30d" ? "30d" : "24h",
  }
}

export const saveSparklineFn = (data: SparklinePreference) => rpc<SparklinePreference>("/api/sparkline", "PUT", data)
