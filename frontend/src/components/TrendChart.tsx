import { useT } from "~/lib/i18n"
import { line as chartLine, curveMonotoneX } from "d3"
import { useEffect, useId, useMemo, useRef, useState } from "react"
import { formatClock, formatDayLong, formatDayShort, formatMonthYYYY } from "~/lib/format"
import type { Interval } from "~/lib/range"
import { INTERVAL_VIEW } from "~/lib/range"
import type { Point } from "~/lib/ch"
import type { ChartType } from "~/lib/chart-type-preference"
import { axisScale, formatAxisTick } from "~/lib/axis"

export type TrafficMetric = "visitors" | "visits" | "pageviews"
export type TrafficSeries = Record<TrafficMetric, Point[]>
type TrafficPoint = {date: string; values: Record<TrafficMetric, number>}
type HoverPoint = {index: number; x: number; persistent: boolean}
export const TRAFFIC_METRICS: ReadonlyArray<{key: TrafficMetric; label: string}> = [
  {key: "visitors", label: "访客"},
  {key: "visits", label: "访问次数"},
  {key: "pageviews", label: "浏览量"},
]
// Each metric keeps the original soft line and fading area treatment.
// The areas overlap transparently rather than stacking or offsetting values.
// Visitors is drawn last so the headline metric is on top where it equals visits.
const DRAW_ORDER: TrafficMetric[] = ["pageviews", "visits", "visitors"]
const FILL_ORDER: TrafficMetric[] = ["pageviews", "visits", "visitors"]
// Visitors never outnumber visits, nor visits pageviews, so the bars stand in front of each other at the same width,
// the smallest at the front, as Umami draws visitors over views.
const BAR_ORDER: TrafficMetric[] = ["pageviews", "visits", "visitors"]
const BAR_MAX_WIDTH = 40
const NONE: ReadonlyArray<TrafficMetric> = []
// Point rings use their own outer-to-inner order so the surface-filled outer
// circle cannot cover the other two metrics when all values match.
const POINT_ORDER: TrafficMetric[] = ["pageviews", "visitors", "visits"]
const POINT_RADIUS: Record<TrafficMetric, number> = {pageviews: 7, visitors: 5, visits: 3}
const HEIGHT = 264
const MARGIN = { top: 16, bottom: 32 }
const AXIS_FONT_SIZE = 10
const LABEL_LIFT = 4 // value labels sit just above their gridline, inside the plot
const TOOLTIP_ACTION_CLASS = "squircle min-h-8 rounded-md border border-gray-600 bg-gray-700 px-3 text-xs font-medium text-gray-100 hover:bg-gray-600"

/** A bar with rounded top corners that sits on the baseline. */
function barPath(x: number, width: number, top: number, bottom: number) {
  const r = Math.min(3, width / 2, bottom - top)
  return `M${x},${bottom}V${top + r}Q${x},${top} ${x + r},${top}H${x + width - r}Q${x + width},${top} ${x + width},${top + r}V${bottom}Z`
}

function axisLabel(date: string, interval: Interval, withYear: boolean, locale: string) {
  if (interval === "month") return formatMonthYYYY(date, locale)
  if (interval === "hour") return formatClock(date)
  if (interval === "minute") return formatClock(date, true)
  return formatDayShort(date, withYear, locale)
}

function tooltipLabel(date: string, interval: Interval, withYear: boolean, index: number, total: number, realtime: boolean, locale: string) {
  if (interval === "month") return formatMonthYYYY(date, locale)
  if (interval === "week") return locale === "en" ? `Week of ${formatDayShort(date, withYear, locale)}` : `${formatDayShort(date, withYear, locale)} 当周`
  if (interval === "day") return formatDayLong(date, withYear, locale)
  if (interval === "minute" && realtime) {
    const ago = total - index
    if (locale !== "en") return `${ago} 分钟前`
    return ago === 1 ? "1 minute ago" : `${ago} minutes ago`
  }
  if (interval === "hour" || interval === "minute") {
    return `${formatDayLong(date, withYear, locale)}, ${formatClock(date, interval === "minute")}`
  }
  return formatDayLong(date, withYear, locale)
}

export function TrendChart({
  series,
  interval = "day",
  realtime = false,
  type = "line",
  hidden = NONE,
  onZoom,
}: {
  series: TrafficSeries
  interval?: Interval
  realtime?: boolean
  type?: ChartType
  /** Series left out of the drawing, the tooltip and the scale; the data itself is untouched. */
  hidden?: ReadonlyArray<TrafficMetric>
  onZoom?: (date: string) => void
}) {
  const { t, locale } = useT()
  const gradientId = useId()
  const box = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(320)
  const [hover, setHover] = useState<HoverPoint | null>(null)
  const hoverRef = useRef<HoverPoint | null>(null)
  hoverRef.current = hover
  const pointerIndex = useRef<number | null>(null)
  const dismissedIndex = useRef<number | null>(null)
  const points = useMemo<TrafficPoint[]>(() => {
    const dates = [...new Set(TRAFFIC_METRICS.flatMap(({key}) => series[key].map(point => point.date)))].sort()
    const values = Object.fromEntries(TRAFFIC_METRICS.map(({key}) => [key, new Map(series[key].map(point => [point.date, point.value]))])) as Record<TrafficMetric, Map<string, number>>
    return dates.map(date => ({date, values: {
      visitors: values.visitors.get(date) ?? 0,
      visits: values.visits.get(date) ?? 0,
      pageviews: values.pageviews.get(date) ?? 0,
    }}))
  }, [series])
  const firstDate = points[0]?.date
  const lastDate = points[points.length - 1]?.date

  useEffect(() => {
    // Retain a pinned point during value-only refreshes, but never carry an
    // old index into a different date window or bucket interval.
    pointerIndex.current = null
    dismissedIndex.current = null
    setHover(null)
  }, [interval, realtime, firstDate, lastDate, points.length])

  useEffect(() => {
    const el = box.current
    if (!el) return
    const measure = () => setWidth(el.clientWidth || 320)
    measure()
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", measure)
      return () => window.removeEventListener("resize", measure)
    }
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  useEffect(() => {
    if (!hover?.persistent) return
    const closeOutside = (event: PointerEvent) => {
      if (!box.current?.contains(event.target as Node)) dismissTooltip()
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") dismissTooltip()
    }
    document.addEventListener("pointerdown", closeOutside)
    document.addEventListener("keydown", closeOnEscape)
    return () => {
      document.removeEventListener("pointerdown", closeOutside)
      document.removeEventListener("keydown", closeOnEscape)
    }
  }, [hover?.persistent])

  const withYear = new Set(points.map((p) => p.date.slice(0, 4))).size > 1
  const canZoom = (interval === "day" || interval === "month") && points.length > 1 && !!onZoom
  const visible = useMemo(() => TRAFFIC_METRICS.map(metric => metric.key).filter(key => !hidden.includes(key)), [hidden])
  const shown = TRAFFIC_METRICS.filter(metric => visible.includes(metric.key))
  const label = shown.map(metric => t(metric.label)).join(" / ")

  const layout = useMemo(() => {
    const scale = axisScale(Math.max(0, ...points.flatMap(point => visible.map(key => point.values[key]))))
    const yMax = scale.max
    const yTickVals = scale.ticks
    // Value labels sit inside the plot on their gridlines, so the plot spans the card's padding evenly on both sides.
    const left = 0
    const right = 0
    const innerW = Math.max(1, width - left - right)
    const innerH = HEIGHT - MARGIN.top - MARGIN.bottom
    const baseline = MARGIN.top + innerH
    const bars = type === "bar"
    // Bars own a slot each and are centred in it; a line starts and ends on the plot's edges.
    const slot = innerW / Math.max(points.length, 1)
    const x = (i: number) => bars ? left + slot * (i + 0.5) : left + (points.length <= 1 ? innerW / 2 : (i / (points.length - 1)) * innerW)
    const y = (v: number) => MARGIN.top + innerH - (v / yMax) * innerH
    const lines = bars ? [] : DRAW_ORDER.filter(key => visible.includes(key)).map(key => ({key, path: chartLine<TrafficPoint>().x((_, i) => x(i)).y(point => y(point.values[key])).curve(curveMonotoneX)(points) || ""}))
    const areas = bars ? [] : FILL_ORDER.filter(key => visible.includes(key)).map(key => {
      const path = lines.find(line => line.key === key)!.path
      return {key, path: points.length ? `${path} L${x(points.length - 1)},${baseline} L${x(0)},${baseline} Z` : ""}
    })
    const barWidth = Math.max(2, Math.min(slot * 0.72, BAR_MAX_WIDTH))
    const columns = !bars ? [] : points.map((point, i) => BAR_ORDER.filter(key => visible.includes(key)).map(key => {
      const value = point.values[key]
      const top = value > 0 ? Math.min(y(value), baseline - 1) : baseline
      return {key, i, path: value > 0 ? barPath(x(i) - barWidth / 2, barWidth, top, baseline) : ""}
    })).flat()
    const xTickCount = Math.min(Math.max(2, Math.floor(innerW / (withYear ? 110 : 76))), 8, points.length)
    const xTicks = points.length ? Array.from({ length: xTickCount }, (_, i) => {
      const index = xTickCount === 1 ? 0 : Math.round((i * (points.length - 1)) / (xTickCount - 1))
      return { index, x: x(index), label: axisLabel(points[index].date, interval, withYear, locale) }
    }) : []
    return { left, right, innerW, slot, bars, baseline, x, y, lines, areas, columns, xTicks, yTickVals }
  }, [points, width, interval, withYear, locale, type, visible])

  function indexFromClientX(clientX: number) {
    const rect = box.current?.getBoundingClientRect()
    if (!rect || points.length === 0) return 0
    const px = clientX - rect.left
    if (layout.bars) return Math.max(0, Math.min(points.length - 1, Math.floor((px - layout.left) / layout.slot)))
    const position = (px - layout.left) / layout.innerW
    return Math.max(0, Math.min(points.length - 1, Math.round(position * (points.length - 1))))
  }

  function dismissTooltip(index = pointerIndex.current ?? hoverRef.current?.index ?? null) {
    // Keep the just-dismissed point quiet until the pointer reaches a new
    // point or leaves the chart; a following pointermove must not reopen it.
    dismissedIndex.current = index
    setHover(null)
  }

  function pinTooltip(index: number) {
    dismissedIndex.current = null
    setHover({index, x: layout.x(index), persistent: true})
  }

  const selected = hover ? points[hover.index] : null
  const tooltipWidth = Math.min(300, Math.max(0, width - 16))
  const tooltipX = hover ? Math.max(8, Math.min(hover.x + 12, width - tooltipWidth - 8)) : 0

  return (
    <div
      ref={box}
      className="trend-chart traffic-trend relative mt-4 mb-3"
      tabIndex={points.length ? 0 : undefined}
      role="group"
      aria-label={label}
      style={{ height: HEIGHT }}
      onKeyDown={event => {
        if (event.target !== event.currentTarget || !points.length) return
        if (["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) {
          event.preventDefault()
          const index = event.key === "Home" ? 0 : event.key === "End" ? points.length - 1 : Math.max(0, Math.min(points.length - 1, (hover?.index ?? 0) + (event.key === "ArrowLeft" ? -1 : 1)))
          pinTooltip(index)
        } else if (event.key === "Escape") dismissTooltip()
      }}
      onPointerMove={event => {
        if (event.pointerType === "touch") return
        const index = indexFromClientX(event.clientX)
        pointerIndex.current = index
        if (hoverRef.current?.persistent || dismissedIndex.current === index) return
        dismissedIndex.current = null
        setHover({ index, x: layout.x(index), persistent: false })
      }}
      onPointerLeave={() => {
        pointerIndex.current = null
        dismissedIndex.current = null
        if (!hoverRef.current?.persistent) setHover(null)
      }}
      onClick={event => {
        if (!points.length) return
        const index = indexFromClientX(event.clientX)
        pointerIndex.current = index
        if (hoverRef.current?.persistent) {
          dismissTooltip(index)
          return
        }
        // One click/tap pins a point; the next plot click closes it. Touch
        // uses this same event, avoiding a pointerdown+click double toggle.
        pinTooltip(index)
      }}
      onContextMenu={event => {
        event.preventDefault()
        if (!points.length) return
        const index = indexFromClientX(event.clientX)
        pointerIndex.current = index
        pinTooltip(index)
      }}
    >
      {points.length === 0 ? <p className="flex h-full items-center justify-center text-sm font-medium text-gray-500">{t("暂无数据")}</p> : (
        <svg role="img" aria-label={label} width="100%" height={HEIGHT} viewBox={`0 0 ${width} ${HEIGHT}`} className="overflow-visible">
          <defs>
            {shown.map(({key}) => <linearGradient key={key} id={`${gradientId}-${key}`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor={`var(--traffic-${key})`} stopOpacity="0.14" />
              <stop offset="100%" stopColor={`var(--traffic-${key})`} stopOpacity="0.01" />
            </linearGradient>)}
          </defs>
          {layout.yTickVals.map((value, i) => (
            <g key={value}>
              <line x1={layout.left} x2={width - layout.right} y1={layout.y(value)} y2={layout.y(value)} stroke={i === 0 ? "var(--color-gray-300)" : "var(--color-gray-150)"} />
            </g>
          ))}
          <line x1={layout.left} x2={width - layout.right} y1={HEIGHT - MARGIN.bottom} y2={HEIGHT - MARGIN.bottom} stroke="var(--color-gray-300)" />
          {layout.xTicks.map(tick => (
            <g key={tick.index}>
              <line x1={tick.x} x2={tick.x} y1={HEIGHT - MARGIN.bottom} y2={HEIGHT - MARGIN.bottom + 4} stroke="var(--color-gray-300)" />
              <text x={tick.x} y={HEIGHT - 10} textAnchor={layout.bars ? "middle" : tick.index === 0 ? "start" : tick.index === points.length - 1 ? "end" : "middle"} className="trend-axis-label fill-gray-500" fontSize={AXIS_FONT_SIZE} fontWeight="400">{t(tick.label)}</text>
            </g>
          ))}
          {layout.bars && hover ? <rect className="traffic-hover-band" x={layout.left + layout.slot * hover.index} y={MARGIN.top - 4} width={layout.slot} height={layout.baseline - MARGIN.top + 4} /> : null}
          {layout.areas.map(({key, path}) => <path key={key} data-metric={key} className="traffic-series-area" d={path} fill={`url(#${gradientId}-${key})`} />)}
          {layout.lines.map(({key, path}) => <g key={key}>
            <path data-metric={key} className="traffic-series-path" d={path} fill="none" stroke={`var(--traffic-${key})`} strokeWidth="2.5" strokeLinejoin="round" strokeLinecap="round" />
          </g>)}
          {layout.columns.map(({key, i, path}) => path ? <path key={`${i}-${key}`} data-metric={key} className="traffic-bar" d={path} /> : null)}
          {/* Drawn over the series with a surface-coloured halo so a curve passing through never cuts a label. */}
          {layout.yTickVals.slice(1).map(value => <text key={value} x={layout.left} y={layout.y(value) - LABEL_LIFT} textAnchor="start" className="trend-axis-label trend-value-label fill-gray-400" fontSize={AXIS_FONT_SIZE} fontWeight="400">{formatAxisTick(value)}</text>)}
          {!layout.bars && points.length === 1 ? POINT_ORDER.filter(key => visible.includes(key)).map(key => <circle key={key} data-metric={key} cx={layout.x(0)} cy={layout.y(points[0].values[key])} r={POINT_RADIUS[key]} fill={key === "pageviews" ? "var(--surface)" : "none"} stroke={`var(--traffic-${key})`} strokeWidth="2" />) : null}
          {hover && selected && !layout.bars ? <>
            <line x1={hover.x} x2={hover.x} y1={MARGIN.top - 4} y2={HEIGHT - MARGIN.bottom + 4} stroke="var(--analytics-guide)" strokeWidth="1" />
            {POINT_ORDER.filter(key => visible.includes(key)).map(key => <circle key={key} data-metric={key} cx={hover.x} cy={layout.y(selected.values[key])} r={POINT_RADIUS[key]} fill={key === "pageviews" ? "var(--surface)" : "none"} stroke={`var(--traffic-${key})`} strokeWidth="2" />)}
          </> : null}
        </svg>
      )}
      {selected && hover ? (
        <div className={`trend-tooltip traffic-tooltip squircle absolute bg-gray-800 py-3 px-4 rounded-md shadow shadow-gray-200 w-max ${hover.persistent ? "" : "pointer-events-none"}`} style={{ left: tooltipX, top: 8, maxWidth: tooltipWidth }}
          onPointerDown={event => event.stopPropagation()}
          onClick={event => event.stopPropagation()}
          onContextMenu={event => event.stopPropagation()}>
          <aside className="flex flex-col gap-2 text-sm font-normal text-gray-100">
            <div className="traffic-tooltip-date">{tooltipLabel(selected.date, interval, withYear, hover.index, points.length, realtime, locale)}</div>
            <dl className="traffic-tooltip-metrics">{shown.map(({key, label}) => <div key={key} data-metric={key}>
              <dt><span className="traffic-series-dot" aria-hidden="true" />{t(label)}</dt>
              <dd>{selected.values[key].toLocaleString(locale)}</dd>
            </div>)}</dl>
            {canZoom ? <>
              <hr className="my-1 border-gray-600" />
              <div className="flex items-center justify-between gap-4">
                <button type="button" className={TOOLTIP_ACTION_CLASS} onClick={event=>{event.stopPropagation();onZoom?.(selected.date)}}>{t("点击查看")} {t(INTERVAL_VIEW[interval])}</button>
              </div>
            </> : null}
            {!hover.persistent ? <div className="text-xs text-gray-300">{t("右键固定提示")}</div> : null}
          </aside>
        </div>
      ) : null}
    </div>
  )
}
