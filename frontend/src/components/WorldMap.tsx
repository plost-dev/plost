import { UIIcon } from "./UIIcon"
import { GoogleVisitorMap } from "./GoogleVisitorMap"
import { pointKey, pointRadius, unplacedLive, useLiveArrivals, usePointArrivals, type LivePoint } from "~/lib/live-arrivals"
import { Surface } from "~/components/Surface"
import { useT } from "~/lib/i18n"
import { useEffect, useMemo, useRef, useState } from "react"
import * as d3 from "d3"
import { CountryFlag } from "~/components/icons"
import { countryName } from "~/lib/countries"
import { numberShort } from "~/lib/format"
import { MapboxVisitorMap } from "./MapboxVisitorMap"
import { HelpHint } from "./HelpHint"
import { readMapFlags, saveMapFlags, useGoogleMapStyle, useMapStyle } from "~/lib/map-preferences"
import {
  COUNTRIES_BY_TWO_LETTER_CODE,
  countryCentroids,
  loadWorldMap,
  type WorldJsonCountryData,
} from "~/lib/world-countries"

const MAP_ROTATE: [number, number] = [-150, 0]
const WIDE_WIDTH = 960
const WIDE_HEIGHT = 420
const OCEAN = "var(--map-ocean)"
const EMPTY_FILL = "var(--map-empty)"
const BORDER = "var(--map-border)"
const COLOR_STOPS = [1, 2, 3, 4].map(index => `--map-accent-${index}`)

type CountryData = {
  alpha_3: string
  name: string
  visitors: number
  code: string
}

function iso2(code: string) {
  return (code || "").replace(/\0/g, "").trim().toUpperCase()
}

function setupProjection(width: number, height: number) {
  const projection = d3.geoMercator()
    .rotate(MAP_ROTATE)
    .scale(width * 0.158)
    .translate([width / 2, height / 1.5])
  return { projection, path: d3.geoPath().projection(projection) }
}

function WorldMap({
  rows,
  live = [],
  points = [],
  showFlags = true,
  onCountryClick,
}: {
  rows: Array<{ name: string, value: number, label?: string }>
  live?: Array<{ country: string }>
  points?: LivePoint[]
  showFlags?: boolean
  onCountryClick?: (code: string) => void
}) {
  const { t, locale } = useT()
  const svgRef = useRef<SVGSVGElement | null>(null)
  const zoomRef = useRef<d3.ZoomBehavior<SVGSVGElement, unknown> | null>(null)
  const [features, setFeatures] = useState<WorldJsonCountryData[] | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [tooltip, setTooltip] = useState<{ x: number, y: number, hovered: string | null }>({
    x: 0,
    y: 0,
    hovered: null,
  })

  const { maxValue, dataByAlpha3Code } = useMemo(() => {
    const dataByAlpha3Code = new Map<string, CountryData>()
    let maxValue = 0
    for (const row of rows) {
      const code = iso2(row.name)
      if (!code || code === "(NONE)") continue
      const entry = COUNTRIES_BY_TWO_LETTER_CODE[code]
      if (!entry?.alpha_3) continue
      const visitors = Number(row.value) || 0
      if (visitors > maxValue) maxValue = visitors
      dataByAlpha3Code.set(entry.alpha_3, {
        alpha_3: entry.alpha_3,
        visitors,
        name: row.label || countryName(code, locale),
        code,
      })
    }
    return { maxValue, dataByAlpha3Code }
  }, [rows, locale])

  const liveByAlpha3 = useMemo(() => {
    const counts = new Map<string, number>()
    for (const item of live) {
      const a3 = COUNTRIES_BY_TWO_LETTER_CODE[iso2(item.country)]?.alpha_3
      if (!a3) continue
      counts.set(a3, (counts.get(a3) || 0) + 1)
    }
    return counts
  }, [live])
  const arrivals = useLiveArrivals(live)
  const pointArrivals = usePointArrivals(points)

  useEffect(() => {
    let active = true
    let observer: IntersectionObserver | undefined
    setLoadFailed(false)
    function load() {
      observer?.disconnect()
      loadWorldMap().then((data) => {
        if (active) setFeatures(data)
      }).catch(() => {
        if (active) setLoadFailed(true)
      })
    }
    // Keep the overview usable without downloading geography below the viewport.
    if (svgRef.current && typeof IntersectionObserver !== "undefined") {
      observer = new IntersectionObserver((entries) => {
        if (entries.some((entry) => entry.isIntersecting)) load()
      }, { rootMargin: "200px" })
      observer.observe(svgRef.current)
    } else load()
    return () => { active = false; observer?.disconnect() }
  }, [attempt])

  useEffect(() => {
    const el = svgRef.current
    if (!el || !features) return
    const svg = d3.select(el)
    svg.selectAll("*").remove()

    const root = svg.append("g").attr("class", "map-root").attr("transform", d3.zoomTransform(el).toString())
    const { path, projection } = setupProjection(WIDE_WIDTH, WIDE_HEIGHT)

    root.append("rect")
      .attr("width", WIDE_WIDTH)
      .attr("height", WIDE_HEIGHT)
      .attr("fill", OCEAN)
      .attr("pointer-events", "none")

    const countries = root.append("g").attr("class", "countries")
      .selectAll("path.country")
      .data(features)
      .enter()
      .append("path")
      .attr("class", "country")
      .attr("stroke", BORDER)
      .attr("stroke-width", 0.55)
      .attr("stroke-linejoin", "round")
      .attr("d", path as never)
      .attr("cursor", (d) => dataByAlpha3Code.get(d.properties.a3)?.visitors ? "pointer" : "default")
      .attr("tabindex", d => dataByAlpha3Code.get(d.properties.a3)?.visitors && onCountryClick ? 0 : null)
      .attr("role", d => dataByAlpha3Code.get(d.properties.a3)?.visitors && onCountryClick ? "button" : null)
      .attr("aria-label", d => {const row=dataByAlpha3Code.get(d.properties.a3);return row ? `${row.name} · ${numberShort(row.visitors)} ${t("访客")}` : null})

    // D3 interpolates actual colors, not CSS variables. Repaint on preference
    // changes without rebuilding geography or resetting the user's zoom.
    const colorScale = d3.scaleSqrt().domain([0, maxValue || 1]).range([0.12, 1]).clamp(true)
    let ramp = (_: number) => EMPTY_FILL
    let hoveredA3: string | null = null
    const baseFill = (a3: string) => {
      const row = dataByAlpha3Code.get(a3)
      return row?.visitors ? ramp(colorScale(row.visitors)) : EMPTY_FILL
    }
    // The country under the pointer is shown a shade darker rather than outlined: the map is already coloured, and a
    // thick outline only crowds the borders around it.
    const fillOf = (a3: string) => {
      const base = baseFill(a3)
      const color = a3 === hoveredA3 ? d3.color(base) : null
      return color ? color.darker(0.45).formatRgb() : base
    }
    const applyColors = () => {
      const styles = getComputedStyle(el)
      ramp = d3.interpolateRgbBasis(COLOR_STOPS.map(token => styles.getPropertyValue(token).trim()))
      countries.attr("fill", country => fillOf(country.properties.a3))
    }
    applyColors()
    const paletteObserver = new MutationObserver(applyColors)
    paletteObserver.observe(document.documentElement, {attributes: true, attributeFilter: ["data-accent", "data-theme"]})

    countries
      .on("keydown", (event, country) => {
        if (event.key !== "Enter" && event.key !== " ") return
        event.preventDefault()
        const row = dataByAlpha3Code.get(country.properties.a3)
        if (row?.visitors) onCountryClick?.(row.code)
      })
      .on("mouseover", function (event, country) {
        const [x, y] = d3.pointer(event, svg.node()?.parentNode)
        setTooltip({ x, y, hovered: country.properties.a3 })
        hoveredA3 = country.properties.a3
        d3.select(this).attr("fill", fillOf(hoveredA3))
      })
      .on("mousemove", function (event) {
        const [x, y] = d3.pointer(event, svg.node()?.parentNode)
        setTooltip((cur) => ({ ...cur, x, y }))
      })
      .on("mouseout", function (_, country) {
        setTooltip({ x: 0, y: 0, hovered: null })
        hoveredA3 = null
        d3.select(this).attr("fill", fillOf(country.properties.a3))
      })
      .on("click", (event, country) => {
        if (event.defaultPrevented) return
        const row = dataByAlpha3Code.get((country as WorldJsonCountryData).properties.a3)
        if (row?.code) onCountryClick?.(row.code)
      })

    const flags = root.append("g").attr("class", "country-flags")
    for (const row of dataByAlpha3Code.values()) {
      if (!row.visitors) continue
      const center = countryCentroids(features).get(row.alpha_3)
      const xy = center ? projection(center) : null
      if (!xy) continue
      const marker = flags.append("g").attr("transform", `translate(${xy[0]},${xy[1]})`).attr("cursor", "pointer")
      marker.attr("aria-hidden", "true")
      marker.append("image").attr("href", `/images/flags/1x1/${row.code.toLowerCase()}.svg`).attr("x", -12).attr("y", -12).attr("width", 24).attr("height", 24)
      marker.append("title").text(`${row.name} · ${row.visitors}`)
      marker.on("click", () => onCountryClick?.(row.code))
    }

    const dots = root.append("g").attr("class", "live-dots").attr("pointer-events", "none")
    const centroids = countryCentroids(features)
    for (const [a3, count] of liveByAlpha3) {
      const ll = centroids.get(a3)
      if (!ll) continue
      const xy = projection(ll)
      if (!xy) continue
      const n = Math.min(count, 8)
      for (let i = 0; i < n; i++) {
        const angle = (i / n) * Math.PI * 2
        const dist = n === 1 ? 0 : 3.2 + i * 0.7
        const x = xy[0] + Math.cos(angle) * dist
        const y = xy[1] + Math.sin(angle) * dist
        dots.append("circle").attr("class", "live-dot-pulse").attr("cx", x).attr("cy", y).attr("r", 9)
        dots.append("circle").attr("class", "live-dot").attr("cx", x).attr("cy", y).attr("r", 3.6)
      }
    }
    // Visitors placed by city: one dot per place, a little larger for more visitors there.
    for (const point of points) {
      const xy = projection([point.lon, point.lat])
      if (!xy) continue
      const r = pointRadius(point.count)
      dots.append("circle").attr("class", "live-dot-pulse").attr("cx", xy[0]).attr("cy", xy[1]).attr("r", r * 2.4)
      dots.append("circle").attr("class", "live-dot").attr("cx", xy[0]).attr("cy", xy[1]).attr("r", r)
      if (pointArrivals.has(pointKey(point))) for (const delay of [0, 0.6, 1.2]) dots.append("circle").attr("class", "live-dot-arrival").attr("cx", xy[0]).attr("cy", xy[1]).attr("r", 16).style("animation-delay", `${delay}s`)
    }
    // A country that just gained a visitor sends out a few wide rings.
    for (const code of arrivals) {
      const a3 = COUNTRIES_BY_TWO_LETTER_CODE[iso2(code)]?.alpha_3
      const ll = a3 ? centroids.get(a3) : undefined
      const xy = ll ? projection(ll) : null
      if (!xy) continue
      for (const delay of [0, 0.6, 1.2]) dots.append("circle").attr("class", "live-dot-arrival").attr("cx", xy[0]).attr("cy", xy[1]).attr("r", 16).style("animation-delay", `${delay}s`)
    }

    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.45, 8])
      .on("zoom", (ev) => {
        root.attr("transform", ev.transform.toString())
      })
    zoomRef.current = zoom
    svg.call(zoom)

    return () => {
      paletteObserver.disconnect()
      zoomRef.current = null
      svg.on(".zoom", null)
      svg.selectAll("*").remove()
    }
  }, [features, maxValue, dataByAlpha3Code, liveByAlpha3, arrivals, points, pointArrivals, onCountryClick])

  function zoomBy(k: number) {
    const el = svgRef.current
    if (!el || !zoomRef.current) return
    d3.select(el).transition().duration(180).call(zoomRef.current.scaleBy, k)
  }

  function resetZoom() {
    const el = svgRef.current
    if (!el || !zoomRef.current) return
    d3.select(el).transition().duration(180).call(zoomRef.current.transform, d3.zoomIdentity)
  }

  const hovered = tooltip.hovered ? dataByAlpha3Code.get(tooltip.hovered) : undefined
  const hoveredLive = tooltip.hovered ? liveByAlpha3.get(tooltip.hovered) || 0 : 0

  return (
    <div className="relative w-full">
      <div className="squircle map-stage relative overflow-hidden" data-show-flags={showFlags}>
        <div className="squircle map-controls absolute top-2.5 right-2.5 z-10 flex overflow-hidden rounded-sm border border-white/70 bg-white/90 shadow-sm backdrop-blur-sm">
          <button type="button" disabled={!features} className="squircle px-2.5 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40" onClick={() => zoomBy(1.35)} aria-label={t("放大")}><UIIcon name="plus" className="size-3"/></button>
          <button type="button" disabled={!features} className="squircle border-l border-gray-200 px-2.5 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40" onClick={() => zoomBy(1 / 1.35)} aria-label={t("缩小")}><UIIcon name="minus" className="size-3"/></button>
          <button type="button" disabled={!features} className="squircle border-l border-gray-200 px-2 py-1 text-xs text-gray-500 hover:bg-gray-50 disabled:opacity-40" onClick={resetZoom}>{t("重置")}</button>
        </div>
        <svg
          ref={svgRef}
          viewBox={`0 0 ${WIDE_WIDTH} ${WIDE_HEIGHT}`}
          className="map-viewport h-auto w-full cursor-grab active:cursor-grabbing"
        />
        {!features ? (
          <div className="absolute inset-0 flex items-center justify-center gap-2 text-xs text-gray-500" role="status">
            {loadFailed ? <>{t("地图加载失败")}<button type="button" className="squircle control-compact underline" onClick={() => setAttempt((value) => value + 1)}>{t("重试")}</button></> : t("正在加载地图…")}
          </div>
        ) : null}
      </div>
      <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-[11px] text-gray-400">
        <span>{t("滚轮缩放 · 拖动移动")}</span>
        <span className="inline-flex items-center gap-1">{t("少")}{COLOR_STOPS.map((c) => (
            <span key={c} className="squircle inline-block h-2 w-3.5 rounded-sm" style={{ background: `var(${c})` }} />
          ))}{t("多")}</span>
      </div>
      {hovered ? (
        <div
          className="visitor-map-tooltip squircle pointer-events-none absolute z-50 rounded-md bg-white p-2 shadow-lg ring-1 ring-black/5"
          style={{ left: `clamp(8px, ${tooltip.x + 8}px, calc(100% - 176px))`, top: `clamp(8px, ${tooltip.y + 8}px, calc(100% - 112px))` }}
        >
          <div className="flex items-center gap-1.5 text-sm font-semibold">
            <CountryFlag code={hovered.code} />
            {hovered.name}
          </div>
          <div className="flex items-center gap-x-1 text-sm">
            <strong>{numberShort(hovered.visitors)}</strong>{t("访客")}</div>
          {hoveredLive ? (
            <div className="mt-0.5 flex items-center gap-x-1 text-sm text-green-600">
              <strong>{hoveredLive}</strong>{t("当前在线")}</div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function VisitorMap({token, googleKey = "", ...props}: {token: string; googleKey?: string} & Parameters<typeof WorldMap>[0]) {
  const {t} = useT()
  const styleId = useMapStyle()
  const googleStyleId = useGoogleMapStyle()
  const [failed, setFailed] = useState(false)
  if (googleKey && !failed) return <GoogleVisitorMap apiKey={googleKey} styleId={googleStyleId} rows={props.rows} live={props.live || []} points={props.points || []} showFlags={props.showFlags} onCountryClick={props.onCountryClick} onFailure={() => setFailed(true)}/>
  if (token && !failed) return <MapboxVisitorMap token={token} styleId={styleId} rows={props.rows} live={props.live || []} points={props.points || []} showFlags={props.showFlags} onCountryClick={props.onCountryClick} onFailure={() => setFailed(true)}/>
  return <>
    {failed ? <p role="status" className="map-fallback-note">{t(googleKey ? "Google 地图暂时无法加载，已显示备用地图。请检查 API Key 和它的域名限制。" : "Mapbox 暂时无法加载，已显示备用地图。")}
      <button type="button" className="squircle control-compact underline" onClick={() => setFailed(false)}>{t("重试")}</button>
    </p> : null}
    <WorldMap {...props}/>
  </>
}

export function MapCard({
  mapboxToken = "",
  googleMapsKey = "",
  rows,
  live = [],
  livePoints = [],
  liveCount = 0,
  onViewVisitors,
  onCountryClick,
}: {
  mapboxToken?: string
  googleMapsKey?: string
  rows: Array<{ name: string, value: number, label?: string }>
  live?: Array<{ country: string }>
  livePoints?: LivePoint[]
  liveCount?: number
  onViewVisitors?: () => void
  onCountryClick?: (code: string) => void
}) {
  const { t } = useT()
  const [showFlags, setShowFlags] = useState(true)
  useEffect(() => { setShowFlags(readMapFlags()) }, [])

  function changeFlags(show: boolean) {
    setShowFlags(show)
    saveMapFlags(show)
  }

  const liveStatus = <><span className="map-live-dot" aria-hidden="true"/><strong>{liveCount}</strong><span>{t("当前访客")}</span></>

  return (
    <section className="geography-section col-span-full">
      <div className="geography-grid geography-with-ranking">
        <Surface className="geography-map analytics-panel">
          <div className="analytics-heading geography-map-heading">
            <div className="map-heading-title">
              <h3>{t("访客分布")}</h3>
              <HelpHint>
                <p>{t("查看访客来自哪里，点击国家可筛选报表。")}</p>
                <p>{t("按国家/地区汇总，标记不是访客的精确位置。占比按已记录的地区访客数计算。")}</p>
              </HelpHint>
            </div>
            {onViewVisitors ? <button type="button" className="map-live-count" aria-label={t("查看最近访客")} title={t("近 5 分钟活跃访客")} onClick={onViewVisitors}>{liveStatus}</button>
              : <div className="map-live-count" title={t("近 5 分钟活跃访客")}>{liveStatus}</div>}
              <label className="map-flags-toggle">
                <span>{t("国旗")}</span>
                <input className="sr-only" type="checkbox" role="switch" checked={showFlags} onChange={event => changeFlags(event.target.checked)}/>
                <span className="squircle map-flags-track" aria-hidden="true"><span className="squircle map-flags-thumb"/></span>
              </label>
          </div>
          {/* Visitors placed by city get their own dots; the rest stay at their country. */}
          <VisitorMap key={googleMapsKey || mapboxToken} token={mapboxToken} googleKey={googleMapsKey} rows={rows} live={unplacedLive(live, livePoints)} points={livePoints} showFlags={showFlags} onCountryClick={onCountryClick} />

        </Surface>
      </div>
    </section>
  )
}
