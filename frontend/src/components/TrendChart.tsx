import { useT } from "~/lib/i18n"
import { line as chartLine, curveMonotoneX } from "d3"
import { useEffect, useId, useMemo, useRef, useState } from "react"
import { formatClock, formatDayLong, formatDayShort, formatMonthYYYY } from "~/lib/format"
import type { Interval } from "~/lib/range"
import { INTERVAL_VIEW } from "~/lib/range"
import type { Point } from "~/lib/ch"
import { visitLabel, visitorLabel, type VisitorMode } from "~/lib/visitor-mode-preference"

export type TrafficMetric = "visitors" | "visits" | "pageviews"
export type TrafficSeries = Record<TrafficMetric, Point[]>
type TrafficPoint = {date: string; values: Record<TrafficMetric, number>}
type HoverPoint = {index: number; x: number; persistent: boolean}
export const TRAFFIC_KEYS: ReadonlyArray<TrafficMetric> = ["visitors", "visits", "pageviews"]
export const trafficMetrics = (mode?: VisitorMode): ReadonlyArray<{key: TrafficMetric; label: string}> => [
  {key: "visitors", label: visitorLabel(mode)},
  {key: "visits", label: visitLabel(mode)},
  {key: "pageviews", label: "浏览量"},
]
// Each metric keeps the original soft line and fading area treatment.
// The areas overlap transparently rather than stacking or offsetting values.
const DRAW_ORDER: TrafficMetric[] = ["visitors", "visits", "pageviews"]
const FILL_ORDER: TrafficMetric[] = ["pageviews", "visits", "visitors"]
// Point rings use their own outer-to-inner order so the surface-filled outer
// circle cannot cover the other two metrics when all values match.
const POINT_ORDER: TrafficMetric[] = ["pageviews", "visitors", "visits"]
const POINT_RADIUS: Record<TrafficMetric, number> = {pageviews: 7, visitors: 5, visits: 3}
const HEIGHT = 264
const MARGIN = { top: 16, bottom: 32 }
const AXIS_FONT_SIZE = 10
const TOOLTIP_ACTION_CLASS = "squircle min-h-8 rounded-md border border-gray-600 bg-gray-700 px-3 text-xs font-medium text-gray-100 hover:bg-gray-600"

function niceStep(range: number) {
  if (range <= 0) return 1
  const exp = Math.floor(Math.log10(range))
  const frac = range / 10 ** exp
  const nice = frac <= 1 ? 1 : frac <= 2 ? 2 : frac <= 5 ? 5 : 10
  return nice * 10 ** exp
}

function integerScale(maxValue: number) {
  const raw = Math.max(maxValue, 1)
  const rough = niceStep(raw / 4)
  const step = Math.max(1, Math.round(rough))
  const max = Math.ceil(raw / step) * step
  const ticks: number[] = []
  for (let v = 0; v <= max + 1e-6; v += step) ticks.push(Math.round(v))
  return { max, ticks }
}

function formatAxisTick(value: number) {
  const n = Math.round(value)
  if (n >= 1_000_000 && n % 1_000_000 === 0) return `${n / 1_000_000}M`
  if (n >= 1000 && n % 1000 === 0) return `${n / 1000}k`
  return String(n)
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
  visitorMode,
  onZoom,
}: {
  series: TrafficSeries
  interval?: Interval
  realtime?: boolean
  visitorMode?: VisitorMode
  onZoom?: (date: string) => void
}) {
  const { t, locale } = useT()
  const metrics = useMemo(() => trafficMetrics(visitorMode), [visitorMode])
  const gradientId = useId()
  const box = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(320)
  const [hover, setHover] = useState<HoverPoint | null>(null)
  const hoverRef = useRef<HoverPoint | null>(null)
  hoverRef.current = hover
  const pointerIndex = useRef<number | null>(null)
  const dismissedIndex = useRef<number | null>(null)
  const points = useMemo<TrafficPoint[]>(() => {
    const dates = [...new Set(TRAFFIC_KEYS.flatMap(key => series[key].map(point => point.date)))].sort()
    const values = Object.fromEntries(TRAFFIC_KEYS.map(key => [key, new Map(series[key].map(point => [point.date, point.value]))])) as Record<TrafficMetric, Map<string, number>>
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
  const label = metrics.map(metric => t(metric.label)).join(" / ")

  const layout = useMemo(() => {
    const scale = integerScale(Math.max(0, ...points.flatMap(point => TRAFFIC_KEYS.map(key => point.values[key]))))
    const yMax = scale.max
    const yTickVals = scale.ticks
    // Keep the original axis inset, chart height, tick typography and date spacing.
    const labelSpace = Math.max(...yTickVals.map(v => formatAxisTick(v).length), 1) * 6 + 8
    const left = 0
    const right = 0
    const innerW = Math.max(1, width - left - right)
    const innerH = HEIGHT - MARGIN.top - MARGIN.bottom
    const x = (i: number) => left + (points.length <= 1 ? innerW / 2 : (i / (points.length - 1)) * innerW)
    const y = (v: number) => MARGIN.top + innerH - (v / yMax) * innerH
    const lines = DRAW_ORDER.map(key => ({key, path: chartLine<TrafficPoint>().x((_, i) => x(i)).y(point => y(point.values[key])).curve(curveMonotoneX)(points) || ""}))
    const areas = FILL_ORDER.map(key => {
      const path = lines.find(line => line.key === key)!.path
      return {key, path: points.length ? `${path} L${x(points.length - 1)},${MARGIN.top + innerH} L${x(0)},${MARGIN.top + innerH} Z` : ""}
    })
    const xTickCount = Math.min(Math.max(2, Math.floor(innerW / (withYear ? 110 : 76))), 8, points.length)
    const xTicks = points.length ? Array.from({ length: xTickCount }, (_, i) => {
      const index = xTickCount === 1 ? 0 : Math.round((i * (points.length - 1)) / (xTickCount - 1))
      return { index, x: x(index), label: axisLabel(points[index].date, interval, withYear, locale) }
    }) : []
    return { labelSpace, left, right, innerW, x, y, lines, areas, xTicks, yTickVals }
  }, [points, width, interval, withYear, locale])

  function indexFromClientX(clientX: number) {
    const rect = box.current?.getBoundingClientRect()
    if (!rect || points.length === 0) return 0
    const px = clientX - rect.left
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
      style={{ height: HEIGHT, marginInline: `max(0px, calc(${layout.labelSpace + 12}px - var(--trend-inset, 22px)))` }}
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
            {TRAFFIC_KEYS.map(key => <linearGradient key={key} id={`${gradientId}-${key}`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0%" stopColor={`var(--traffic-${key})`} stopOpacity="0.14" />
              <stop offset="100%" stopColor={`var(--traffic-${key})`} stopOpacity="0.01" />
            </linearGradient>)}
          </defs>
          {layout.yTickVals.map((value, i) => (
            <g key={value}>
              <line x1={layout.left} x2={width - layout.right} y1={layout.y(value)} y2={layout.y(value)} stroke={i === 0 ? "var(--color-gray-300)" : "var(--color-gray-150)"} />
              <text x={layout.left - 8} y={layout.y(value)} dominantBaseline="middle" textAnchor="end" className="trend-axis-label fill-gray-500" fontSize={AXIS_FONT_SIZE} fontWeight="400">{formatAxisTick(value)}</text>
            </g>
          ))}
          <line x1={layout.left} x2={width - layout.right} y1={HEIGHT - MARGIN.bottom} y2={HEIGHT - MARGIN.bottom} stroke="var(--color-gray-300)" />
          {layout.xTicks.map(tick => (
            <g key={tick.index}>
              <line x1={tick.x} x2={tick.x} y1={HEIGHT - MARGIN.bottom} y2={HEIGHT - MARGIN.bottom + 4} stroke="var(--color-gray-300)" />
              <text x={tick.x} y={HEIGHT - 10} textAnchor={tick.index === 0 ? "start" : tick.index === points.length - 1 ? "end" : "middle"} className="trend-axis-label fill-gray-500" fontSize={AXIS_FONT_SIZE} fontWeight="400">{t(tick.label)}</text>
            </g>
          ))}
          {layout.areas.map(({key, path}) => <path key={key} data-metric={key} className="traffic-series-area" d={path} fill={`url(#${gradientId}-${key})`} />)}
          {layout.lines.map(({key, path}) => <path key={key} data-metric={key} className="traffic-series-path" d={path} fill="none" stroke={`var(--traffic-${key})`} strokeWidth="2.5" strokeLinejoin="round" strokeLinecap="round" />)}
          {points.length === 1 ? POINT_ORDER.map(key => <circle key={key} data-metric={key} cx={layout.x(0)} cy={layout.y(points[0].values[key])} r={POINT_RADIUS[key]} fill={key === "pageviews" ? "var(--surface)" : "none"} stroke={`var(--traffic-${key})`} strokeWidth="2" />) : null}
          {hover && selected ? <>
            <line x1={hover.x} x2={hover.x} y1={MARGIN.top - 4} y2={HEIGHT - MARGIN.bottom + 4} stroke="var(--analytics-guide)" strokeWidth="1" />
            {POINT_ORDER.map(key => <circle key={key} data-metric={key} cx={hover.x} cy={layout.y(selected.values[key])} r={POINT_RADIUS[key]} fill={key === "pageviews" ? "var(--surface)" : "none"} stroke={`var(--traffic-${key})`} strokeWidth="2" />)}
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
            <dl className="traffic-tooltip-metrics">{metrics.map(({key, label}) => <div key={key} data-metric={key}>
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
