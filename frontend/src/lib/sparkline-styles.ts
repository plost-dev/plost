import { rpc } from "./rpc"

// The small trend on each site card. Saved for the account; the server keeps the same list (ui.go).
export const SPARKLINE_STYLES = [
  { id: "area-line", label: "折线 + 面积" },
  { id: "smooth", label: "平滑曲线 + 渐变" },
  { id: "peak", label: "峰值标注" },
  { id: "bars", label: "柱形" },
  { id: "change", label: "涨跌柱" },
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
