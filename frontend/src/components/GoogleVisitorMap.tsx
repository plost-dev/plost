import { useEffect, useRef, useState } from "react"
import { UIIcon } from "./UIIcon"
import { CountryFlag } from "./icons"
import { useT } from "~/lib/i18n"
import { numberShort } from "~/lib/format"
import { liveDot, pointKey, useLiveArrivals, usePointArrivals, type LivePoint } from "~/lib/live-arrivals"
import { loadWorldMap, type WorldJsonCountryData } from "~/lib/world-countries"
import { googleMapLook, type GoogleMapStyleId } from "~/lib/google-map-styles"
import { mapboxVisitorData, type MapRows, type VisitorCountry } from "~/lib/mapbox-data"
import { loadGoogleMaps, type Maps } from "~/lib/google-maps-loader"

const HOME = { center: { lat: 25, lng: 110 }, zoom: 2 }

function hexToRgb(hex: string): [number, number, number] {
  const h = hex.replace("#", "").trim()
  const full = h.length === 3 ? h.split("").map(c => c + c).join("") : h
  const n = Number.parseInt(full, 16)
  return Number.isFinite(n) ? [n >> 16 & 255, n >> 8 & 255, n & 255] : [99, 102, 241]
}
function mix(low: string, high: string, weight: number) {
  const a = hexToRgb(low), b = hexToRgb(high)
  return `rgb(${a.map((v, i) => Math.round(v + (b[i] - v) * weight)).join(",")})`
}

export function GoogleVisitorMap({ apiKey, styleId, rows, live, points = [], showFlags = true, onCountryClick, onFailure }: {
  apiKey: string; styleId: GoogleMapStyleId; rows: MapRows; live: Array<{ country: string }>; points?: LivePoint[]; showFlags?: boolean
  onCountryClick?: (code: string) => void; onFailure: () => void
}) {
  const { t, locale } = useT()
  const container = useRef<HTMLDivElement>(null)
  const mapRef = useRef<Maps | null>(null)
  const update = useRef<(() => void) | null>(null)
  const arrivals = useLiveArrivals(live)
  const pointArrivals = usePointArrivals(points)
  const props = useRef({ rows, live, arrivals, points, pointArrivals, onCountryClick, onFailure, t })
  props.current = { rows, live, arrivals, points, pointArrivals, onCountryClick, onFailure, t }
  const [loaded, setLoaded] = useState(false)
  const [hovered, setHovered] = useState<{ country: VisitorCountry; x: number; y: number } | null>(null)

  useEffect(() => {
    const element = container.current
    if (!element) return
    const stage: HTMLDivElement = element
    let disposed = false
    let theme: MutationObserver | undefined
    let overlay: Maps | undefined
    const timer = setTimeout(() => fail(), 20_000)
    const fail = () => { if (!disposed) props.current.onFailure() }
    window.gm_authFailure = fail
    const dark = () => document.documentElement.dataset.theme === "dark"
    const palette = () => {
      const styles = getComputedStyle(element)
      return { low: styles.getPropertyValue("--mapbox-accent-low").trim(), high: styles.getPropertyValue("--mapbox-accent-high").trim(), border: styles.getPropertyValue("--mapbox-accent-border").trim() }
    }
    const show = (country: VisitorCountry, x: number, y: number) =>
      setHovered({ country, x: Math.max(8, Math.min(x + 12, element.clientWidth - 184)), y: Math.max(8, Math.min(y + 12, element.clientHeight - 100)) })
    void (async () => {
      try {
        const [maps, features] = await Promise.all([loadGoogleMaps(apiKey, locale === "en" ? "en" : "zh-CN"), loadWorldMap()]) as [Maps, WorldJsonCountryData[]]
        if (disposed) return
        clearTimeout(timer)
        const map = new maps.Map(element, {
          ...HOME, minZoom: 1, maxZoom: 6, disableDefaultUI: true, clickableIcons: false, gestureHandling: "cooperative",
          ...googleMapLook(styleId, dark()),
        })
        mapRef.current = map
        let byCode = new Map<string, VisitorCountry>()
        const restyle = () => {
          const colors = palette()
          map.data.setStyle((feature: Maps) => ({
            fillColor: mix(colors.low, colors.high, Number(feature.getProperty("weight")) || 0), fillOpacity: 0.6,
            strokeColor: colors.border, strokeWeight: 0.6, cursor: "pointer",
          }))
        }
        // Flags, live dots and arrival rings are plain HTML laid over the map, with the Mapbox markers' styles.
        class Markers extends maps.OverlayView {
          layer = document.createElement("div")
          items: Array<{ el: HTMLElement; center: [number, number] }> = []
          onAdd() { this.layer.className = "google-map-markers"; this.getPanes().overlayMouseTarget.append(this.layer) }
          onRemove() { this.layer.remove() }
          draw() {
            const projection = this.getProjection()
            if (!projection) return
            for (const item of this.items) {
              const point = projection.fromLatLngToDivPixel(new maps.LatLng(item.center[1], item.center[0]))
              if (point) { item.el.style.left = `${point.x}px`; item.el.style.top = `${point.y}px` }
            }
          }
          setCountries(countries: VisitorCountry[]) {
            this.layer.replaceChildren()
            this.items = countries.map(country => {
              const button = document.createElement("button")
              button.type = "button"
              button.className = `mapbox-country-marker google-country-marker${country.live ? " has-live" : ""}${props.current.arrivals.has(country.code.toUpperCase()) ? " is-arrival" : ""}`
              button.setAttribute("aria-label", `${country.name} · ${country.visitors} ${props.current.t("访客")}`)
              const flag = document.createElement("img")
              flag.src = `/images/flags/1x1/${country.code.toLowerCase()}.svg`; flag.alt = ""; flag.draggable = false
              button.append(flag)
              if (country.live) { const dot = document.createElement("span"); dot.className = "mapbox-live-dot"; button.append(dot) }
              button.addEventListener("mouseenter", () => { const r = button.getBoundingClientRect(), box = stage.getBoundingClientRect(); show(country, r.left - box.left, r.top - box.top) })
              // A click selects the country without focusing the marker, so no focus ring is left round the flag.
              button.addEventListener("mousedown", event => event.preventDefault())
              button.addEventListener("mouseleave", () => setHovered(null))
              button.addEventListener("click", event => { event.stopPropagation(); props.current.onCountryClick?.(country.code) })
              this.layer.append(button)
              return { el: button, center: country.center }
            })
            for (const point of props.current.points) {
              const dot = liveDot(point, props.current.pointArrivals.has(pointKey(point)), props.current.t)
              dot.classList.add("google-country-marker")
              this.layer.append(dot)
              this.items.push({ el: dot, center: [point.lon, point.lat] })
            }
            this.draw()
          }
        }
        overlay = new Markers()
        overlay.setMap(map)
        update.current = () => {
          const data = mapboxVisitorData(features, props.current.rows, props.current.live)
          byCode = new Map(data.countries.map(country => [country.code, country]))
          map.data.forEach((feature: Maps) => map.data.remove(feature))
          map.data.addGeoJson(data.polygons, { idPropertyName: "code" })
          overlay.setCountries(data.countries)
          setHovered(null)
        }
        map.data.addListener("mouseover", (event: Maps) => {
          const country = byCode.get(String(event.feature.getProperty("code")))
          const box = element.getBoundingClientRect()
          if (country && event.domEvent) show(country, event.domEvent.clientX - box.left, event.domEvent.clientY - box.top)
        })
        map.data.addListener("mouseout", () => setHovered(null))
        map.data.addListener("click", (event: Maps) => { const code = event.feature.getProperty("code"); if (code) props.current.onCountryClick?.(String(code)) })
        map.addListener("dragstart", () => setHovered(null))
        restyle(); update.current(); setLoaded(true)
        theme = new MutationObserver(() => { map.setOptions(googleMapLook(styleId, dark())); restyle() })
        theme.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme", "data-accent"] })
      } catch { fail() }
    })()
    return () => {
      disposed = true; clearTimeout(timer); theme?.disconnect(); overlay?.setMap(null)
      if (window.gm_authFailure === fail) window.gm_authFailure = undefined
      mapRef.current = null; update.current = null
    }
  }, [apiKey, styleId, locale])
  useEffect(() => { update.current?.() }, [rows, live, arrivals, points, pointArrivals, t])

  return <div className="relative">
    <div className="squircle map-stage mapbox-stage google-map-stage" data-show-flags={showFlags}>
      <div ref={container} className="mapbox-viewport" aria-label={t("访客分布")} />
      <div className="squircle map-controls absolute top-2.5 right-2.5 z-10 flex overflow-hidden border border-gray-200 bg-white/90 shadow-sm backdrop-blur-sm">
        <button type="button" className="squircle px-2.5 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40" disabled={!loaded} aria-label={t("放大")} onClick={() => mapRef.current?.setZoom((mapRef.current.getZoom() || 2) + 1)}><UIIcon name="plus" className="size-3" /></button>
        <button type="button" className="squircle border-l border-gray-200 px-2.5 py-1 text-sm text-gray-700 hover:bg-gray-50 disabled:opacity-40" disabled={!loaded} aria-label={t("缩小")} onClick={() => mapRef.current?.setZoom((mapRef.current.getZoom() || 2) - 1)}><UIIcon name="minus" className="size-3" /></button>
        <button type="button" className="squircle border-l border-gray-200 px-2 py-1 text-xs text-gray-500 hover:bg-gray-50 disabled:opacity-40" disabled={!loaded} onClick={() => { mapRef.current?.setCenter(HOME.center); mapRef.current?.setZoom(HOME.zoom) }}>{t("重置")}</button>
      </div>
      {!loaded ? <div className="mapbox-loading" role="status">{t("正在加载地图…")}</div> : null}
      {hovered ? <div className="squircle mapbox-visitor-tooltip" style={{ left: hovered.x, top: hovered.y }}>
        <div className="flex items-center gap-1.5 font-semibold"><CountryFlag code={hovered.country.code} />{hovered.country.name}</div>
        <div>{numberShort(hovered.country.visitors)} {t("访客")}</div>
        {hovered.country.live ? <div className="text-green-600">{hovered.country.live} {t("当前在线")}</div> : null}
      </div> : null}
    </div>
    <p className="mt-2 text-[11px] text-gray-500">{t("按住 Ctrl / ⌘ 滚轮缩放 · 双指移动地图")}</p>
  </div>
}
