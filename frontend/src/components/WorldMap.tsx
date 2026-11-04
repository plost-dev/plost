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
import { loadCountryMap, loadRegionMap, mainland, type CountryShape, type RegionFeature, type RegionMap } from "~/lib/world-regions"
import { regionName } from "~/lib/geography"

const MAP_ROTATE: [number, number] = [-150, 0]
const WIDE_WIDTH = 960
const WIDE_HEIGHT = 420
const FOCUS_PAD = 28
const OCEAN = "var(--map-ocean)"
const EMPTY_FILL = "var(--map-empty)"
const BORDER = "var(--map-border)"
const COLOR_STOPS = [1, 2, 3, 4].map(index => `--map-accent-${index}`)
/** How close two places may come on screen, in map units at zoom 1, before their dots become one. */
const CLUSTER_RADIUS = 22
/** Below this zoom the world map shows one dot per country. */
const COUNTRY_DOTS_BELOW = 2

/** A place visitors are online at: a city the server placed them in (point), or a province's or country's middle. */
type Place = { lon: number, lat: number, count: number, country: string, point?: string }

// The outlines of a view are worked out once: the world, or one opened country. Going back and forth between them, or
// any refresh of the numbers, reuses them instead of projecting thousands of shapes again.
const pathCache = new Map<string, WeakMap<object, string>>()
function cachedPath(view: string, path: d3.GeoPath) {
  let known = pathCache.get(view)
  if (!known) pathCache.set(view, known = new WeakMap())
  return (feature: object) => {
    let d = known.get(feature)
    if (d === undefined) known.set(feature, d = path(feature as never) || "")
    return d
  }
}

export type MapOutline = "regions" | "countries"

/** A report row of the "regions" kind: code is the country, name the province (an ISO 3166-2 code, or a name). */
export type RegionRow = { name: string, value: number, code?: string }
/** Visitors online now in a province (name: an ISO 3166-2 code or a name, "" when none was recorded). */
export type LiveRegion = { country: string, name: string, value: number }

type CountryData = {
  name: string
  visitors: number
  code: string
}

function iso2(code: string) {
  return (code || "").replace(/\0/g, "").trim().toUpperCase()
}

function worldProjection() {
  return d3.geoMercator()
    .rotate(MAP_ROTATE)
    .scale(WIDE_WIDTH * 0.158)
    .translate([WIDE_WIDTH / 2, WIDE_HEIGHT / 1.5])
}

// A country opened from the map fills the frame: the United States with Alaska and Hawaii moved in beside it, as maps
// of it usually are, and other countries in Mercator centred on themselves, so none is cut by the world map's seam.
function countryProjection(shape: CountryShape, regions: RegionFeature[]) {
  const extent: [[number, number], [number, number]] = [[FOCUS_PAD, FOCUS_PAD], [WIDE_WIDTH - FOCUS_PAD, WIDE_HEIGHT - FOCUS_PAD]]
  // Framed by the lower 48: Alaska and Hawaii are drawn in their usual corner and would only shrink the rest.
  if (shape.properties.code === "US") return d3.geoAlbersUsa().fitExtent(extent, {
    type: "FeatureCollection", features: regions.filter(r => r.properties.c === "US" && r.properties.i !== "US-AK" && r.properties.i !== "US-HI"),
  } as never)
  const main = mainland(shape)
  const [lon] = d3.geoCentroid(main as never)
  return d3.geoMercator().rotate([-lon, 0]).fitExtent(extent, main as never)
}

// One empty list for absent props, so a re-render (a tooltip moving) does not look like new data.
const NONE: never[] = []

function WorldMap({
  rows,
  regions = NONE,
  focus = "",
  live = NONE,
  liveRegions = NONE,
  points = NONE,
  showFlags = true,
  outline = "regions",
  onCountryClick,
  onBack,
}: {
  rows: Array<{ name: string, value: number, label?: string }>
  /** "regions": countries with their provinces; "countries": country outlines only, lighter to load and draw. */
  outline?: MapOutline
  /** The visitors online now by country and province, for placing those without a city. */
  liveRegions?: LiveRegion[]
  /** Visitors by province, shown when a country is open. */
  regions?: RegionRow[]
  /** The country the report is filtered to: the map opens it and colours its provinces. */
  focus?: string
  live?: Array<{ country: string }>
  points?: LivePoint[]
  showFlags?: boolean
  onCountryClick?: (code: string) => void
  onBack?: () => void
}) {
  const { t, locale } = useT()
  const svgRef = useRef<SVGSVGElement | null>(null)
  const zoomRef = useRef<d3.ZoomBehavior<SVGSVGElement, unknown> | null>(null)
  const [map, setMap] = useState<RegionMap | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const [tooltip, setTooltip] = useState<{ x: number, y: number, hovered: string | null }>({
    x: 0,
    y: 0,
    hovered: null,
  })

  const { maxValue, dataByCode } = useMemo(() => {
    const dataByCode = new Map<string, CountryData>()
    let maxValue = 0
    for (const row of rows) {
      const code = iso2(row.name)
      if (!code || code === "(NONE)") continue
      const visitors = Number(row.value) || 0
      if (visitors > maxValue) maxValue = visitors
      dataByCode.set(code, { visitors, name: row.label || countryName(code, locale), code })
    }
    return { maxValue, dataByCode }
  }, [rows, locale])

  // Only a map with provinces opens a country; with country outlines only, a click just filters the report.
  const shape = useMemo(() => focus && map?.regions.length ? map.countries.find(c => c.properties.code === iso2(focus)) : undefined, [map, focus])
  // The open country's provinces with their visitors. Rows the map cannot place (a name it does not know) are left out.
  const { regionMax, regionValues } = useMemo(() => {
    const regionValues = new Map<string, number>()
    if (!shape || !map) return { regionMax: 0, regionValues }
    const code = shape.properties.code
    for (const row of regions) {
      if (iso2(row.code || "") !== code) continue
      const key = map.regionCode(code, row.name)
      if (key) regionValues.set(key, (regionValues.get(key) || 0) + (Number(row.value) || 0))
    }
    return { regionMax: Math.max(0, ...regionValues.values()), regionValues }
  }, [shape, map, regions])

  // Visitors online that have no city place, by country.
  const liveByCode = useMemo(() => {
    const counts = new Map<string, number>()
    for (const item of live) {
      const code = iso2(item.country)
      if (code) counts.set(code, (counts.get(code) || 0) + 1)
    }
    return counts
  }, [live])
  // Everyone online in each country, those placed at a city included, for the tooltip.
  const liveTotalByCode = useMemo(() => {
    const counts = new Map(liveByCode)
    for (const p of points) {
      const code = iso2(p.country)
      if (code) counts.set(code, (counts.get(code) || 0) + p.count)
    }
    return counts
  }, [liveByCode, points])
  const arrivals = useLiveArrivals(live)
  const pointArrivals = usePointArrivals(points)

  // Where the visitors online now are: at their city when the server placed them, otherwise at their province when one
  // was recorded, and only failing both at the middle of their country.
  const places = useMemo<Place[]>(() => {
    if (!map) return []
    const out: Place[] = []
    const placedIn = new Map<string, number>()
    for (const p of points) {
      const country = iso2(p.country)
      out.push({ lon: p.lon, lat: p.lat, count: p.count, country, point: pointKey(p) })
      const region = map.regionAt(country, p.lon, p.lat)
      if (region) placedIn.set(region, (placedIn.get(region) || 0) + p.count)
    }
    const left = new Map(liveByCode)
    for (const row of liveRegions) {
      const country = iso2(row.country), want = left.get(country) || 0
      if (!want || !row.name) continue
      const code = map.regionCode(country, row.name)
      const center = code ? map.regionCenter(code) : undefined
      if (!center) continue
      // Those already shown at a city of this province are not counted twice.
      const placed = placedIn.get(code) || 0
      placedIn.set(code, Math.max(0, placed - row.value))
      const n = Math.min(want, Math.max(0, row.value - placed))
      if (!n) continue
      out.push({ lon: center[0], lat: center[1], count: n, country })
      left.set(country, want - n)
    }
    for (const [country, n] of left) {
      const center = map.centroids.get(country)
      if (n > 0 && center) out.push({ lon: center[0], lat: center[1], count: n, country })
    }
    return out
  }, [map, points, liveByCode, liveRegions])

  // The layers read what they show from here, so new numbers repaint them without drawing the map again.
  const latest = useRef({ dataByCode, maxValue, regionValues, regionMax, places, arrivals, pointArrivals, onCountryClick, locale, t })
  latest.current = { dataByCode, maxValue, regionValues, regionMax, places, arrivals, pointArrivals, onCountryClick, locale, t }
  const layers = useRef<{ repaint: () => void, drawDots: () => void } | null>(null)

  useEffect(() => {
    let active = true
    let observer: IntersectionObserver | undefined
    setLoadFailed(false)
    function load() {
      observer?.disconnect()
      ;(outline === "countries" ? loadCountryMap : loadRegionMap)().then((data) => {
        if (active) setMap(data)
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
  }, [attempt, outline])

  // The geography: drawn when the map loads and when a country opens or closes, never for a refresh of the numbers.
  useEffect(() => {
    const el = svgRef.current
    if (!el || !map) return
    const svg = d3.select(el)
    svg.selectAll("*").remove()
    const view = shape ? shape.properties.code : ""
    setTooltip({ x: 0, y: 0, hovered: null })

    const root = svg.append("g").attr("class", "map-root")
    const projection = shape ? countryProjection(shape, map.regions) : worldProjection()
    const path = cachedPath(view, d3.geoPath().projection(projection))

    root.append("rect")
      .attr("width", WIDE_WIDTH)
      .attr("height", WIDE_HEIGHT)
      .attr("fill", OCEAN)
      .attr("pointer-events", "none")

    let ramp = (_: number) => EMPTY_FILL
    let hovered: string | null = null
    const valueOf = (key: string) => shape ? latest.current.regionValues.get(key) || 0 : latest.current.dataByCode.get(key)?.visitors || 0
    let colorScale = d3.scaleSqrt().domain([0, 1]).range([0.12, 1]).clamp(true)
    // The area under the pointer is shown a shade darker rather than outlined: the map is already coloured, and a
    // thick outline only crowds the borders around it.
    const fillOf = (key: string) => {
      const value = valueOf(key)
      const base = value ? ramp(colorScale(value)) : EMPTY_FILL
      const color = key === hovered ? d3.color(base) : null
      return color ? color.darker(0.45).formatRgb() : base
    }

    // Countries: the whole map, or around an open country the neighbours in a paler shade for orientation.
    const countries = root.append("g").attr("class", "countries")
      .selectAll("path.country")
      // The United States' projection moves Alaska and Hawaii, which would leave Canada and Mexico in pieces under them.
      .data(shape ? (shape.properties.code === "US" ? [] : map.countries.filter(c => c !== shape)) : map.countries)
      .enter()
      .append("path")
      .attr("class", "country")
      .attr("stroke", BORDER)
      .attr("stroke-width", 0.7)
      .attr("vector-effect", "non-scaling-stroke")
      .attr("stroke-linejoin", "round")
      .attr("d", path)
      .attr("opacity", shape ? 0.55 : null)
    // Provinces: the open country's own areas, each coloured by its visitors.
    const provinces = shape ? root.append("g").attr("class", "regions")
      .selectAll("path.region")
      .data(map.regions.filter(r => r.properties.c === shape.properties.code))
      .enter()
      .append("path")
      .attr("class", "region")
      .attr("stroke", BORDER)
      .attr("stroke-width", 0.6)
      .attr("vector-effect", "non-scaling-stroke")
      .attr("stroke-linejoin", "round")
      .attr("d", path) : null
    // On the world map the province lines are drawn finer than the borders, for detail without clutter.
    if (!shape && map.regions.length) root.append("path").datum(map.regionLines).attr("class", "region-lines").attr("d", path)
      .attr("fill", "none").attr("stroke", BORDER).attr("stroke-width", 0.35).attr("stroke-opacity", 0.8)
      .attr("vector-effect", "non-scaling-stroke").attr("pointer-events", "none")
    const flags = root.append("g").attr("class", "country-flags")
    const dots = root.append("g").attr("class", "live-dots").attr("pointer-events", "none")

    // key colours the area; tip names what the tooltip shows (a country code, or "region:" and the province's names).
    const enter = (key: string, tip: string) => function (this: SVGPathElement, event: MouseEvent) {
      const [x, y] = d3.pointer(event, svg.node()?.parentNode)
      setTooltip({ x, y, hovered: tip })
      hovered = key
      d3.select(this).attr("fill", fillOf(key))
    }
    const move = (event: MouseEvent) => {
      const [x, y] = d3.pointer(event, svg.node()?.parentNode)
      setTooltip((cur) => ({ ...cur, x, y }))
    }
    const leave = (key: string) => function (this: SVGPathElement) {
      setTooltip({ x: 0, y: 0, hovered: null })
      hovered = null
      d3.select(this).attr("fill", fillOf(key))
    }
    const clickable = (code: string) => !!latest.current.dataByCode.get(code)?.visitors && !!latest.current.onCountryClick
    if (provinces) {
      provinces
        .on("mouseover", function (event, r) { enter(r.properties.i, `region:${r.properties.i}|${r.properties.n}|${r.properties.z}`).call(this, event) })
        .on("mousemove", move)
        .on("mouseout", function (_, r) { leave(r.properties.i).call(this) })
    } else {
      countries
        .on("keydown", (event, c) => {
          if (event.key !== "Enter" && event.key !== " ") return
          event.preventDefault()
          if (clickable(c.properties.code)) latest.current.onCountryClick?.(c.properties.code)
        })
        // A click opens the country without focusing its shape, which would leave a focus frame round it.
        .on("mousedown.focus", (event: MouseEvent) => event.preventDefault())
        .on("mouseover", function (event, c) { enter(c.properties.code, c.properties.code).call(this, event) })
        .on("mousemove", move)
        .on("mouseout", function (_, c) { leave(c.properties.code).call(this) })
        .on("click", (event, c) => {
          if (event.defaultPrevented) return
          if (clickable(c.properties.code)) latest.current.onCountryClick?.(c.properties.code)
        })
    }

    // The zoom the dots and flags were last drawn for; they keep their size on screen.
    let scale = 1
    const sizeFlags = () => flags.selectAll<SVGGElement, [number, number]>("g").attr("transform", xy => `translate(${xy[0]},${xy[1]}) scale(${1 / scale})`)

    // Colours, labels and flags follow the numbers.
    const repaint = () => {
      const { dataByCode, maxValue, regionValues, regionMax, locale, t } = latest.current
      ramp = d3.interpolateRgbBasis(COLOR_STOPS.map(token => getComputedStyle(el).getPropertyValue(token).trim()))
      colorScale = d3.scaleSqrt().domain([0, (shape ? regionMax : maxValue) || 1]).range([0.12, 1]).clamp(true)
      if (provinces) {
        countries.attr("fill", EMPTY_FILL)
        provinces.attr("fill", r => fillOf(r.properties.i))
          .attr("aria-label", r => regionValues.get(r.properties.i) ? `${regionName(r.properties.i, r.properties.n, r.properties.z, locale)} · ${numberShort(regionValues.get(r.properties.i) || 0)} ${t("访客")}` : null)
        return
      }
      countries.attr("fill", c => fillOf(c.properties.code))
        .attr("cursor", c => clickable(c.properties.code) ? "pointer" : "default")
        .attr("tabindex", c => clickable(c.properties.code) ? 0 : null)
        .attr("role", c => clickable(c.properties.code) ? "button" : null)
        .attr("aria-label", c => {const row=dataByCode.get(c.properties.code);return row ? `${row.name} · ${numberShort(row.visitors)} ${t("访客")}` : null})
      flags.selectAll("*").remove()
      for (const row of dataByCode.values()) {
        const center = row.visitors ? map.centroids.get(row.code) : undefined
        const xy = center ? projection(center) : null
        if (!xy) continue
        const marker = flags.append("g").datum(xy).attr("transform", `translate(${xy[0]},${xy[1]}) scale(${1 / scale})`).attr("cursor", "pointer").attr("aria-hidden", "true")
        marker.append("image").attr("href", `/images/flags/1x1/${row.code.toLowerCase()}.svg`).attr("x", -12).attr("y", -12).attr("width", 24).attr("height", 24)
        marker.append("title").text(`${row.name} · ${row.visitors}`)
        marker.on("click", () => latest.current.onCountryClick?.(row.code))
      }
    }

    // Visitors online. On the whole world each country is one dot, at its busiest place and sized by everyone online
    // there; zoomed in, or with a country open, each city or province has its own, and places that would still overlap
    // on screen share one. Dots keep their size on screen at any zoom.
    const drawDots = () => {
      const { places, arrivals, pointArrivals } = latest.current
      dots.selectAll("*").remove()
      const reach = CLUSTER_RADIUS / scale
      const clusters: Array<{ x: number, y: number, count: number, arriving: boolean, country: string }> = []
      for (const place of [...places].sort((a, b) => b.count - a.count)) {
        if (shape && place.country !== view) continue
        const xy = projection([place.lon, place.lat])
        if (!xy) continue
        const arriving = place.point ? pointArrivals.has(place.point) : arrivals.has(place.country)
        // Joined to the busiest dot nearby (or of the country, on the whole world), which stays where it is.
        const byCountry = !shape && scale < COUNTRY_DOTS_BELOW
        const near = clusters.find(c => byCountry ? c.country === place.country : Math.hypot(c.x - xy[0], c.y - xy[1]) < reach)
        if (near) {
          near.count += place.count
          near.arriving ||= arriving
        } else clusters.push({ x: xy[0], y: xy[1], count: place.count, arriving, country: place.country })
      }
      for (const c of clusters) {
        const r = pointRadius(c.count) / scale
        dots.append("circle").attr("class", c.arriving ? "live-dot is-arrival" : "live-dot").attr("cx", c.x).attr("cy", c.y).attr("r", r)
        if (c.arriving) for (const delay of [0, 0.6, 1.2]) dots.append("circle").attr("class", "live-dot-arrival").attr("cx", c.x).attr("cy", c.y).attr("r", 16 / scale).style("animation-delay", `${delay}s`)
      }
    }

    repaint()
    drawDots()
    layers.current = { repaint, drawDots }
    const paletteObserver = new MutationObserver(repaint)
    paletteObserver.observe(document.documentElement, {attributes: true, attributeFilter: ["data-accent", "data-theme"]})

    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent(shape ? [0.6, 12] : [0.45, 12])
      .on("zoom", (ev) => {
        root.attr("transform", ev.transform.toString())
        // Regroup the dots once the zoom has changed enough to matter, not on every frame of it.
        if (Math.abs(Math.log(ev.transform.k / scale)) > 0.15 || (ev.transform.k < COUNTRY_DOTS_BELOW) !== (scale < COUNTRY_DOTS_BELOW)) {
          scale = ev.transform.k
          drawDots()
          sizeFlags()
        }
      })
      .on("end", (ev) => {
        if (ev.transform.k !== scale) {
          scale = ev.transform.k
          drawDots()
          sizeFlags()
        }
      })
    zoomRef.current = zoom
    svg.call(zoom)
    // Opening or leaving a country starts from its whole frame.
    svg.call(zoom.transform, d3.zoomIdentity)

    return () => {
      paletteObserver.disconnect()
      layers.current = null
      zoomRef.current = null
      svg.on(".zoom", null)
      svg.selectAll("*").remove()
    }
  }, [map, shape])

  useEffect(() => { layers.current?.repaint() }, [dataByCode, maxValue, regionValues, regionMax, locale, t])
  useEffect(() => { layers.current?.drawDots() }, [places, arrivals, pointArrivals])

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

  const hoveredRegion = tooltip.hovered?.startsWith("region:") ? tooltip.hovered.slice(7).split("|") : null
  const hovered = !hoveredRegion && tooltip.hovered ? dataByCode.get(tooltip.hovered) : undefined
  const hoveredLive = hovered ? liveTotalByCode.get(hovered.code) || 0 : 0
  const focusCode = shape?.properties.code || ""
  const legendMax = focusCode ? regionMax : maxValue

  return (
    <div className="relative w-full">
      <div className="squircle map-stage relative overflow-hidden" data-show-flags={showFlags}>
        {focusCode ? (
          <button type="button" className="squircle map-back absolute top-2.5 left-2.5 z-10" onClick={onBack} title={t("返回世界地图")}>
            <UIIcon name="globe" className="size-3.5"/><span>{t("全球")}</span><span aria-hidden="true">/</span>
            <CountryFlag code={focusCode}/><strong>{countryName(focusCode, locale)}</strong>
          </button>
        ) : null}
        <div className="squircle map-controls absolute top-2.5 right-2.5 z-10 flex overflow-hidden border border-gray-200 bg-white/90 shadow-sm backdrop-blur-sm">
          <button type="button" disabled={!map} className="squircle px-2.5 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40" onClick={() => zoomBy(1.35)} aria-label={t("放大")}><UIIcon name="plus" className="size-3"/></button>
          <button type="button" disabled={!map} className="squircle border-l border-gray-200 px-2.5 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40" onClick={() => zoomBy(1 / 1.35)} aria-label={t("缩小")}><UIIcon name="minus" className="size-3"/></button>
          <button type="button" disabled={!map} className="squircle border-l border-gray-200 px-2 py-1 text-xs text-gray-500 hover:bg-gray-50 disabled:opacity-40" onClick={resetZoom}>{t("重置")}</button>
        </div>
        <svg
          ref={svgRef}
          viewBox={`0 0 ${WIDE_WIDTH} ${WIDE_HEIGHT}`}
          className="map-viewport h-auto w-full cursor-grab active:cursor-grabbing"
        />
        {!map ? (
          <div className="absolute inset-0 flex items-center justify-center gap-2 text-xs text-gray-500" role="status">
            {loadFailed ? <>{t("地图加载失败")}<button type="button" className="squircle control-compact underline" onClick={() => setAttempt((value) => value + 1)}>{t("重试")}</button></> : t("正在加载地图…")}
          </div>
        ) : focusCode && !regionValues.size ? (
          <div className="map-empty-note absolute bottom-2.5 left-2.5 z-10" role="status">{t("这段时间还没有记录到省/州")}</div>
        ) : null}
      </div>
      <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-[11px] text-gray-400">
        <span>{t(focusCode ? "滚轮缩放 · 拖动移动" : map && !map.regions.length ? "滚轮缩放 · 拖动移动 · 点击国家筛选" : "滚轮缩放 · 拖动移动 · 点击国家查看省/州")}</span>
        <span className="map-legend inline-flex items-center gap-1.5">
          <span>0</span>
          <span className="squircle map-legend-bar" style={{ background: `linear-gradient(to right, ${COLOR_STOPS.map(c => `var(${c})`).join(", ")})` }} aria-hidden="true"/>
          <span>{numberShort(legendMax)} {t("访客")}</span>
        </span>
      </div>
      {hovered || hoveredRegion ? (
        <div
          className="visitor-map-tooltip squircle pointer-events-none absolute z-50 rounded-md bg-white p-2 shadow-lg ring-1 ring-black/5"
          style={{ left: `clamp(8px, ${tooltip.x + 8}px, calc(100% - 176px))`, top: `clamp(8px, ${tooltip.y + 8}px, calc(100% - 112px))` }}
        >
          {hoveredRegion ? <>
            <div className="text-sm font-semibold">{regionName(hoveredRegion[0], hoveredRegion[1], hoveredRegion[2], locale)}</div>
            <div className="flex items-center gap-x-1 text-sm">
              <strong>{numberShort(regionValues.get(hoveredRegion[0]) || 0)}</strong>{t("访客")}</div>
          </> : hovered ? <>
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
          </> : null}
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
  regions = [],
  focus = "",
  live = [],
  livePoints = [],
  liveRegions = [],
  liveCount = 0,
  outline = "regions",
  onViewVisitors,
  onCountryClick,
  onBack,
}: {
  outline?: MapOutline
  mapboxToken?: string
  googleMapsKey?: string
  rows: Array<{ name: string, value: number, label?: string }>
  regions?: RegionRow[]
  focus?: string
  onBack?: () => void
  live?: Array<{ country: string }>
  livePoints?: LivePoint[]
  liveRegions?: LiveRegion[]
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
              <h3><UIIcon name="globe" className="analytics-heading-icon" />{t("访客分布")}</h3>
              <HelpHint>
                <p>{t("查看访客来自哪里，点击国家可筛选报表，并按省/州查看分布。")}</p>
                <p>{t("按国家/地区和省/州汇总，标记不是访客的精确位置。省/州来自 GeoIP 城市库，按 IP 推算，可能有偏差。")}</p>
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
          <VisitorMap key={googleMapsKey || mapboxToken} token={mapboxToken} googleKey={googleMapsKey} rows={rows} regions={regions} focus={focus} live={unplacedLive(live, livePoints)} points={livePoints} liveRegions={liveRegions} showFlags={showFlags} outline={outline} onCountryClick={onCountryClick} onBack={onBack} />

        </Surface>
      </div>
    </section>
  )
}
