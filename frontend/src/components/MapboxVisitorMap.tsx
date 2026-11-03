import { UIIcon } from "./UIIcon"
import { useLiveArrivals } from "~/lib/live-arrivals"
import { useEffect, useRef, useState } from "react"
import type { GeoJSONSource, Map as MapboxMap, Marker } from "mapbox-gl"
import { useT } from "~/lib/i18n"
import { loadWorldMap, type WorldJsonCountryData } from "~/lib/world-countries"
import { mapboxVisitorData, type MapRows, type VisitorCountry } from "~/lib/mapbox-data"
import { CountryFlag } from "./icons"
import { numberShort } from "~/lib/format"
import { DEFAULT_MAPBOX_STYLE, getMapboxStyle, mapboxStyleConfig, type MapboxStyleId } from "~/lib/mapbox-styles"
import { createMapboxStyleLifecycle } from "~/lib/mapbox-style-lifecycle"

export function MapboxVisitorMap({token, rows, live, showFlags = true, styleId = DEFAULT_MAPBOX_STYLE, onCountryClick, onFailure}: {
  token: string; rows: MapRows; live: Array<{country: string}>;
  showFlags?: boolean; styleId?: MapboxStyleId;
  onCountryClick?: (code: string) => void; onFailure: () => void
}) {
  const {t} = useT()
  const container = useRef<HTMLDivElement>(null)
  const mapRef = useRef<MapboxMap | null>(null)
  const styleLifecycle = useRef<ReturnType<typeof createMapboxStyleLifecycle> | null>(null)
  const update = useRef<(() => void) | null>(null)
  const arrivals = useLiveArrivals(live)
  const props = useRef({rows, live, arrivals, styleId, onCountryClick, onFailure, t})
  props.current = {rows, live, arrivals, styleId, onCountryClick, onFailure, t}
  const [loaded, setLoaded] = useState(false)
  const [hovered, setHovered] = useState<{country: VisitorCountry; x: number; y: number} | null>(null)

  useEffect(() => {
    const element = container.current
    if (!element) return
    let disposed = false, started = false
    let map: MapboxMap | undefined, features: WorldJsonCountryData[]
    let activeStyle = getMapboxStyle(props.current.styleId)
    let markers: Marker[] = []
    let timer: ReturnType<typeof setTimeout> | undefined
    let resize: ResizeObserver | undefined, theme: MutationObserver | undefined
    const dark = () => document.documentElement.dataset.theme === "dark"
    const colors = () => {
      const styles = getComputedStyle(element)
      return {
        low: styles.getPropertyValue("--mapbox-accent-low").trim(),
        high: styles.getPropertyValue("--mapbox-accent-high").trim(),
        border: styles.getPropertyValue("--mapbox-accent-border").trim(),
      }
    }
    const fail = () => { if (!disposed) props.current.onFailure() }
    const show = (country: VisitorCountry, x: number, y: number) => {
      setHovered({country, x: Math.max(8, Math.min(x + 12, element.clientWidth - 184)), y: Math.max(8, Math.min(y + 12, element.clientHeight - 100))})
    }
    const start = async () => {
      if (started) return
      started = true
      observer?.disconnect()
      timer = setTimeout(fail, 20_000)
      try {
        const [{default: gl}, geography] = await Promise.all([import("mapbox-gl"), loadWorldMap(), import("mapbox-gl/dist/mapbox-gl.css")])
        if (disposed) return
        if (!gl.supported()) { fail(); return }
        features = geography
        activeStyle = getMapboxStyle(props.current.styleId)
        map = new gl.Map({
          container: element, accessToken: token,
          style: activeStyle.url, projection: "mercator",
          center: [110, 25], zoom: 1.2, minZoom: 0, maxZoom: 6,
          attributionControl: false, cooperativeGestures: true,
          performanceMetricsCollection: false,
          config: mapboxStyleConfig(activeStyle.id, dark()),
        })
        clearTimeout(timer)
        mapRef.current = map
        map.addControl(new gl.AttributionControl({compact: true}), "bottom-right")
        element.querySelectorAll(".mapboxgl-ctrl-attrib,.mapboxgl-ctrl-attrib-button").forEach(control => control.classList.add("squircle"))
        let byCode = new Map<string, VisitorCountry>()
        update.current = () => {
          if (!map?.getSource("litestats-visitors")) return
          const data = mapboxVisitorData(features, props.current.rows, props.current.live)
          byCode = new Map(data.countries.map(country => [country.code, country]))
          ;(map.getSource("litestats-visitors") as GeoJSONSource).setData(data.polygons)
          markers.forEach(marker => marker.remove()); markers = []
          for (const country of data.countries) {
            const button = document.createElement("button")
            button.type = "button"; button.className = `mapbox-country-marker${country.live ? " has-live" : ""}${props.current.arrivals.has(country.code.toUpperCase()) ? " is-arrival" : ""}`
            button.setAttribute("aria-label", `${country.name} · ${country.visitors} ${props.current.t("访客")}`)
            button.title = `${country.name} · ${country.visitors}`
            const flag = document.createElement("img")
            flag.src = `/images/flags/1x1/${country.code.toLowerCase()}.svg`; flag.alt = ""; flag.draggable = false
            button.append(flag)
            if (country.live) {const dot = document.createElement("span"); dot.className = "mapbox-live-dot"; button.append(dot)}
            const reveal = () => {if (map) {const point = map.project(country.center); show(country, point.x, point.y)}}
            button.addEventListener("mouseenter", reveal); button.addEventListener("focus", reveal)
            button.addEventListener("mouseleave", () => setHovered(null)); button.addEventListener("blur", () => setHovered(null))
            button.addEventListener("click", () => props.current.onCountryClick?.(country.code))
            const marker = new gl.Marker({element: button}).setLngLat(country.center).addTo(map)
            // Mapbox gives custom markers role="img"; these markers remain interactive buttons.
            button.setAttribute("role", "button")
            markers.push(marker)
          }
          setHovered(null)
        }
        // Interactions belong to the map, rather than to an individual basemap load.
        map.on("mousemove", "litestats-countries", event => {
          const country = byCode.get(String(event.features?.[0]?.properties?.code))
          if (country) {show(country, event.point.x, event.point.y); map!.getCanvas().style.cursor = "pointer"}
        })
        map.on("mouseleave", "litestats-countries", () => {setHovered(null); if (map) map.getCanvas().style.cursor = ""})
        map.on("click", "litestats-countries", event => {
          const country = byCode.get(String(event.features?.[0]?.properties?.code))
          if (country) props.current.onCountryClick?.(country.code)
        })
        map.on("movestart", () => setHovered(null))
        const applyTheme = (mutations?: MutationRecord[]) => {
          if (!map?.getLayer("litestats-countries")) return
          if (activeStyle.standard && (!mutations || mutations.some(mutation => mutation.attributeName === "data-theme"))) {
            map.setConfigProperty("basemap", "lightPreset", dark() ? "night" : "day")
          }
          const palette = colors()
          map.setPaintProperty("litestats-countries", "fill-color", ["interpolate", ["linear"], ["get", "weight"], 0, palette.low, 1, palette.high])
          map.setPaintProperty("litestats-countries", "fill-outline-color", palette.border)
        }
        styleLifecycle.current = createMapboxStyleLifecycle(map, {
          initialStyleId: activeStyle.id,
          isDark: dark,
          onFailure: fail,
          onLoading: () => {
            setLoaded(false); setHovered(null)
            if (map) map.getCanvas().style.cursor = ""
          },
          onReady: style => {
            if (disposed || !map) return
            activeStyle = style
            if (!map.getSource("litestats-visitors")) map.addSource("litestats-visitors", {type: "geojson", data: {type: "FeatureCollection", features: []}})
            if (!map.getLayer("litestats-countries")) {
              const palette = colors()
              const beforeId = style.standard ? undefined : map.getStyle().layers?.find(layer => layer.type === "symbol")?.id
              map.addLayer({id: "litestats-countries", type: "fill", ...(style.standard ? {slot: "middle"} : {}), source: "litestats-visitors", paint: {
                "fill-color": ["interpolate", ["linear"], ["get", "weight"], 0, palette.low, 1, palette.high],
                "fill-opacity": .55, "fill-outline-color": palette.border,
                "fill-emissive-strength": style.standard ? 1 : 0,
              }}, beforeId)
            }
            update.current?.(); applyTheme(); setLoaded(true)
          },
        })
        theme = new MutationObserver(applyTheme)
        theme.observe(document.documentElement, {attributes: true, attributeFilter: ["data-theme", "data-accent"]})
        resize = new ResizeObserver(() => map?.resize()); resize.observe(element)
      } catch { fail() }
    }
    const observer = typeof IntersectionObserver === "undefined" ? undefined : new IntersectionObserver(entries => {
      if (entries.some(entry => entry.isIntersecting)) void start()
    }, {rootMargin: "200px"})
    if (observer) observer.observe(element)
    else void start()
    return () => {
      disposed = true; clearTimeout(timer); observer?.disconnect(); resize?.disconnect(); theme?.disconnect()
      styleLifecycle.current?.dispose(); styleLifecycle.current = null
      markers.forEach(marker => marker.remove()); map?.remove(); mapRef.current = null; update.current = null
    }
  }, [token])
  useEffect(() => {update.current?.()}, [rows, live, arrivals, t])
  useEffect(() => {styleLifecycle.current?.setStyle(styleId)}, [styleId])

  return <div className="relative">
    <div className="squircle map-stage mapbox-stage" data-show-flags={showFlags}>
      <div ref={container} className="mapbox-viewport" aria-label={t("访客分布")}/>
      <div className="squircle map-controls absolute top-2.5 right-2.5 z-10 flex overflow-hidden border border-gray-200 bg-white/90 shadow-sm backdrop-blur-sm">
        <button type="button" className="squircle px-2.5 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40" disabled={!loaded} aria-label={t("放大")} onClick={() => mapRef.current?.zoomIn()}><UIIcon name="plus" className="size-3"/></button>
        <button type="button" className="squircle border-l border-gray-200 px-2.5 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40" disabled={!loaded} aria-label={t("缩小")} onClick={() => mapRef.current?.zoomOut()}><UIIcon name="minus" className="size-3"/></button>
        <button type="button" className="squircle border-l border-gray-200 px-2 py-1 text-xs text-gray-500 hover:bg-gray-50 disabled:opacity-40" disabled={!loaded} onClick={() => mapRef.current?.easeTo({center: [110, 25], zoom: 1.2, bearing: 0, pitch: 0})}>{t("重置")}</button>
      </div>
      {!loaded ? <div className="mapbox-loading" role="status">{t("正在加载地图…")}</div> : null}
      {hovered ? <div className="squircle mapbox-visitor-tooltip" style={{left: hovered.x, top: hovered.y}}>
        <div className="flex items-center gap-1.5 font-semibold"><CountryFlag code={hovered.country.code}/>{hovered.country.name}</div>
        <div>{numberShort(hovered.country.visitors)} {t("访客")}</div>
        {hovered.country.live ? <div className="text-green-600">{hovered.country.live} {t("当前在线")}</div> : null}
      </div> : null}
    </div>
    <p className="mt-2 text-[11px] text-gray-500">{t("按住 Ctrl / ⌘ 滚轮缩放 · 双指移动地图")}</p>
  </div>
}
