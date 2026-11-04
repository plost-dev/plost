import { useEffect, useRef, useState } from "react"
import type { Map as MapboxMap, Marker } from "mapbox-gl"
import { loadGoogleMaps, type Maps } from "~/lib/google-maps-loader"
import { newestPlace, placeMarkers, type OverviewMapMode, type OverviewPlace, type PlaceMarker } from "~/components/OverviewGlobe"
import { googleMapLook } from "~/lib/google-map-styles"
import { useT } from "~/lib/i18n"
import { useGoogleMapStyle, useMapStyle } from "~/lib/map-preferences"
import { getMapboxStyle, mapboxStyleConfig } from "~/lib/mapbox-styles"
import { loadWorldMap, type WorldJsonCountryData } from "~/lib/world-countries"

// Mapbox and Google Maps for the site overview, used when the visitor map settings send the chosen service here.
// Both show only the live dots: small ones for the places online now and a breathing ring around the newest visit.

const dark = () => document.documentElement.dataset.theme === "dark"
const reducedMotion = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches
// The globe fills the card about as the built-in one does; the flat map shows the whole world across.
const ZOOM: Record<OverviewMapMode, number> = { globe: 0.7, map: 0 }

/** A live dot element, a little larger for a busier place; `delay` (ms) makes it grow in after that long. */
function liveDot(marker: PlaceMarker, max: number, newest: boolean, delay: number | null) {
  const dot = document.createElement("span")
  dot.className = `overview-live-dot${newest ? " is-newest" : ""}${delay === null ? "" : " is-appearing"}`
  dot.style.setProperty("--size", `${(1.4 + Math.sqrt(marker.visitors / max)) * 2}px`)
  if (delay !== null) dot.style.setProperty("--delay", `${delay}ms`)
  return dot
}

/**
 * The dots for a list of places. Places not shown before grow in, one after another and newest first; `seen` keeps
 * the places already shown, so a push adds only the new dots and a refresh (an emptied `seen`) brings all back.
 */
function liveDots(list: PlaceMarker[], seen: Set<string>) {
  const newest = newestPlace(list), max = Math.max(1, ...list.map((m) => m.visitors))
  const appearing = list.filter((m) => !seen.has(m.key)).sort((a, b) => b.last - a.last).map((m) => m.key)
  const step = Math.min(80, 1200 / Math.max(1, appearing.length))
  appearing.forEach((key) => seen.add(key))
  return list.map((m) => {
    const order = appearing.indexOf(m.key)
    return { marker: m, element: liveDot(m, max, m.key === newest, order < 0 ? null : order * step) }
  })
}

/** The set of places already shown, emptied whenever `replay` changes. */
function useSeen(replay: number) {
  const seen = useRef({ replay, keys: new Set<string>() })
  if (seen.current.replay !== replay) seen.current = { replay, keys: new Set() }
  return seen.current.keys
}

/** The world's countries, for placing country-only visitors at the country's middle. */
function useCountries() {
  const [features, setFeatures] = useState<WorldJsonCountryData[] | null>(null)
  useEffect(() => {
    let alive = true
    loadWorldMap().then((value) => { if (alive) setFeatures(value) }).catch(() => {})
    return () => { alive = false }
  }, [])
  return features
}

export function OverviewMapboxMap({ token, places, mode, replay, onFailure }: { token: string, places: OverviewPlace[], mode: OverviewMapMode, replay: number, onFailure: () => void }) {
  const { t } = useT()
  const styleId = useMapStyle()
  const container = useRef<HTMLDivElement>(null)
  const mapRef = useRef<MapboxMap | null>(null)
  const markerClass = useRef<(typeof import("mapbox-gl"))["Marker"] | null>(null)
  const markers = useRef<Marker[]>([])
  const aimed = useRef(false)
  const failRef = useRef(onFailure)
  failRef.current = onFailure
  const modeRef = useRef(mode)
  modeRef.current = mode
  const [ready, setReady] = useState(false)
  const features = useCountries()
  const seen = useSeen(replay)

  useEffect(() => {
    const element = container.current
    if (!element) return
    let disposed = false, raf = 0, idleUntil = 0
    let map: MapboxMap | undefined
    let theme: MutationObserver | undefined
    const timer = setTimeout(() => failRef.current(), 20_000)
    const style = getMapboxStyle(styleId)
    void (async () => {
      try {
        const [{ default: gl }] = await Promise.all([import("mapbox-gl"), import("mapbox-gl/dist/mapbox-gl.css")])
        if (disposed) return
        if (!gl.supported()) { failRef.current(); return }
        const globe = modeRef.current === "globe"
        map = new gl.Map({
          container: element, accessToken: token, style: style.url, config: mapboxStyleConfig(style.id, dark()),
          projection: globe ? "globe" : "mercator", center: [110, 20], zoom: ZOOM[modeRef.current], minZoom: 0, maxZoom: 4,
          attributionControl: false, scrollZoom: false, doubleClickZoom: false, dragRotate: false, touchPitch: false, keyboard: false,
          performanceMetricsCollection: false,
        })
        map.addControl(new gl.AttributionControl({ compact: true }), "bottom-left")
        map.on("error", (event) => {
          const status = (event.error as { status?: number } | undefined)?.status
          if (status === 401 || status === 403) failRef.current()
        })
        // No starry space or haze around the globe: it sits on the card like the built-in one.
        map.on("style.load", () => map?.setFog({ color: "rgba(0,0,0,0)", "high-color": "rgba(0,0,0,0)", "space-color": "rgba(0,0,0,0)", "star-intensity": 0, "horizon-blend": 0 }))
        map.on("load", () => {
          clearTimeout(timer)
          if (disposed) return
          markerClass.current = gl.Marker
          mapRef.current = map!
          setReady(true)
        })
        // The globe turns slowly on its own and waits a few seconds after someone drags it.
        const hold = () => { idleUntil = performance.now() + 3000 }
        map.on("mousedown", hold)
        map.on("touchstart", hold)
        map.on("drag", hold)
        const spin = () => {
          raf = requestAnimationFrame(spin)
          if (!map || modeRef.current !== "globe" || performance.now() < idleUntil || map.isMoving()) return
          const center = map.getCenter()
          map.setCenter([center.lng + 0.06, center.lat])
        }
        if (!reducedMotion()) raf = requestAnimationFrame(spin)
        theme = new MutationObserver(() => {
          if (style.standard && map?.isStyleLoaded()) map.setConfigProperty("basemap", "lightPreset", dark() ? "night" : "day")
        })
        theme.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] })
      } catch { failRef.current() }
    })()
    return () => {
      disposed = true
      clearTimeout(timer)
      cancelAnimationFrame(raf)
      theme?.disconnect()
      markers.current.forEach((marker) => marker.remove())
      markers.current = []
      map?.remove()
      mapRef.current = null
      setReady(false)
    }
  }, [token, styleId])

  useEffect(() => {
    const map = mapRef.current
    if (!ready || !map) return
    map.setProjection(mode === "globe" ? "globe" : "mercator")
    map.jumpTo({ zoom: ZOOM[mode], center: [map.getCenter().lng, mode === "globe" ? 20 : 25] })
  }, [mode, ready])

  useEffect(() => {
    const map = mapRef.current, Marker = markerClass.current
    if (!ready || !map || !Marker || !features) return
    const list = placeMarkers(places, features)
    // Face the busiest place once, when the first visitors arrive; after that the map is left where it is.
    if (!aimed.current && list[0]) {
      aimed.current = true
      map.setCenter([list[0].lonLat[0], map.getCenter().lat])
    }
    markers.current.forEach((marker) => marker.remove())
    markers.current = liveDots(list, seen).map((dot) => new Marker({ element: dot.element }).setLngLat(dot.marker.lonLat).addTo(map))
  }, [places, features, ready, seen])

  return <div ref={container} className="overview-map-stage" role="img" aria-label={t("当前在线访客位置")} />
}

export function OverviewGoogleMap({ apiKey, places, replay, onFailure }: { apiKey: string, places: OverviewPlace[], replay: number, onFailure: () => void }) {
  const { t, locale } = useT()
  const styleId = useGoogleMapStyle()
  const container = useRef<HTMLDivElement>(null)
  const overlay = useRef<{ setDots: (list: PlaceMarker[], seen: Set<string>) => void } | null>(null)
  const failRef = useRef(onFailure)
  failRef.current = onFailure
  const [ready, setReady] = useState(false)
  const features = useCountries()
  const seen = useSeen(replay)

  useEffect(() => {
    const element = container.current
    if (!element) return
    let disposed = false
    let theme: MutationObserver | undefined
    let map: Maps | undefined
    const timer = setTimeout(() => failRef.current(), 20_000)
    window.gm_authFailure = () => failRef.current()
    void (async () => {
      try {
        const maps = await loadGoogleMaps(apiKey, locale)
        if (disposed) return
        clearTimeout(timer)
        map = new maps.Map(element, {
          center: { lat: 25, lng: 110 }, zoom: 1, minZoom: 1, disableDefaultUI: true, gestureHandling: "cooperative",
          keyboardShortcuts: false, clickableIcons: false, ...googleMapLook(styleId, dark()),
        })
        // The dots sit in an overlay of their own, so they are plain elements the CSS can make glow.
        const layer = document.createElement("div")
        let dots: Array<{ marker: PlaceMarker, element: HTMLElement }> = []
        const Overlay = class extends maps.OverlayView {
          onAdd() { this.getPanes().overlayLayer.append(layer) }
          onRemove() { layer.remove() }
          draw() {
            const projection = this.getProjection()
            if (!projection) return
            for (const dot of dots) {
              const point = projection.fromLatLngToDivPixel(new maps.LatLng(dot.marker.lonLat[1], dot.marker.lonLat[0]))
              dot.element.style.left = `${point.x}px`
              dot.element.style.top = `${point.y}px`
            }
          }
        }
        const view = new Overlay()
        view.setMap(map)
        overlay.current = {
          setDots(list, seen) {
            dots = liveDots(list, seen)
            layer.replaceChildren(...dots.map((dot) => dot.element))
            view.draw()
          },
        }
        setReady(true)
        theme = new MutationObserver(() => map?.setOptions(googleMapLook(styleId, dark())))
        theme.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] })
      } catch { failRef.current() }
    })()
    return () => {
      disposed = true
      clearTimeout(timer)
      theme?.disconnect()
      overlay.current = null
      setReady(false)
    }
  }, [apiKey, styleId, locale])

  useEffect(() => {
    if (ready && features) overlay.current?.setDots(placeMarkers(places, features), seen)
  }, [places, features, ready, seen])

  return <div ref={container} className="overview-map-stage" role="img" aria-label={t("当前在线访客位置")} />
}
