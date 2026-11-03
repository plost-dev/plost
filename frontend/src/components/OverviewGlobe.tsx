import { UIIcon } from "./UIIcon"
import { useEffect, useMemo, useRef, useState } from "react"
import * as d3 from "d3"
import { CountryFlag } from "~/components/icons"
import { countryName } from "~/lib/countries"
import { numberShort } from "~/lib/format"
import { useT } from "~/lib/i18n"
import { COUNTRIES_BY_TWO_LETTER_CODE, countryCentroids, loadWorldMap, type WorldJsonCountryData } from "~/lib/world-countries"

export type OverviewCountry = { code: string, visitors: number, sites: Array<{ name: string, visitors: number }> }
type Mode = "globe" | "map"
type Marker = OverviewCountry & { lonLat: [number, number] }
type Dots = { globe: Array<[number, number]>, map: Array<[number, number]> }

let dotsPromise: Promise<{ dots: Dots, features: WorldJsonCountryData[] }> | undefined

/** Rasterises land once, then samples it: evenly spaced on the sphere for the globe, a plain grid for the flat map. */
function loadDots() {
  return dotsPromise ??= loadWorldMap().then((features) => {
    const width = 720, height = 360
    const canvas = document.createElement("canvas")
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext("2d", { willReadFrequently: true })!
    const projection = d3.geoEquirectangular().scale(width / (2 * Math.PI)).translate([width / 2, height / 2])
    const path = d3.geoPath(projection, ctx)
    ctx.fillStyle = "#000"
    ctx.beginPath()
    path({ type: "FeatureCollection", features } as unknown as d3.GeoPermissibleObjects)
    ctx.fill()
    const pixels = ctx.getImageData(0, 0, width, height).data
    const land = (lon: number, lat: number) => {
      const x = Math.min(width - 1, Math.floor((lon + 180) / 360 * width))
      const y = Math.min(height - 1, Math.floor((90 - lat) / 180 * height))
      return pixels[(y * width + x) * 4 + 3] > 128
    }
    const globe: Array<[number, number]> = []
    const step = 1.6
    for (let lat = -58; lat <= 84; lat += step) {
      const lonStep = step / Math.max(0.2, Math.cos(lat * Math.PI / 180))
      for (let lon = -180; lon < 180; lon += lonStep) if (land(lon, lat)) globe.push([lon, lat])
    }
    const map: Array<[number, number]> = []
    for (let lat = -56; lat <= 82; lat += 2.2) for (let lon = -180; lon < 180; lon += 2.2) if (land(lon, lat)) map.push([lon, lat])
    return { dots: { globe, map }, features }
  }).catch((error) => {
    dotsPromise = undefined
    throw error
  })
}

function cssColor(el: Element, name: string, fallback: string) {
  return getComputedStyle(el).getPropertyValue(name).trim() || fallback
}

export function OverviewGlobe({ countries }: { countries: OverviewCountry[] }) {
  const { t, locale } = useT()
  const [mode, setMode] = useState<Mode>("globe")
  const [data, setData] = useState<{ dots: Dots, features: WorldJsonCountryData[] } | null>(null)
  const [failed, setFailed] = useState(false)
  const [tip, setTip] = useState<Marker | null>(null)
  const tipRef = useRef<HTMLDivElement>(null)
  const tipAt = useRef<[number, number]>([0, 0])
  const [fullscreen, setFullscreen] = useState(false)
  const frameRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const hoverRef = useRef<string | null>(null)

  useEffect(() => {
    let alive = true
    loadDots().then((value) => { if (alive) setData(value) }).catch(() => { if (alive) setFailed(true) })
    return () => { alive = false }
  }, [])

  useEffect(() => {
    const sync = () => setFullscreen(document.fullscreenElement === frameRef.current)
    document.addEventListener("fullscreenchange", sync)
    return () => document.removeEventListener("fullscreenchange", sync)
  }, [])

  const markers = useMemo<Marker[]>(() => {
    if (!data) return []
    const centroids = countryCentroids(data.features)
    return countries.flatMap((country) => {
      const a3 = COUNTRIES_BY_TWO_LETTER_CODE[country.code]?.alpha_3
      const lonLat = a3 ? centroids.get(a3) : undefined
      return lonLat ? [{ ...country, lonLat }] : []
    })
  }, [countries, data])

  useEffect(() => {
    const canvas = canvasRef.current
    const frame = frameRef.current
    if (!canvas || !frame || !data) return
    const ctx = canvas.getContext("2d")!
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches
    const max = Math.max(1, ...markers.map((m) => m.visitors))
    let width = 0, height = 0, raf = 0, visible = true
    // Start facing the busiest country so the first frame already shows a visitor.
    let rotation: [number, number] = markers[0] ? [-markers[0].lonLat[0], -Math.max(-30, Math.min(30, markers[0].lonLat[1]))] : [-20, -20]
    let drag: { x: number, y: number, rotation: [number, number] } | null = null
    let shown: Array<{ marker: Marker, x: number, y: number, r: number }> = []

    const resize = () => {
      const rect = canvas.getBoundingClientRect()
      const ratio = window.devicePixelRatio || 1
      width = rect.width
      height = rect.height
      canvas.width = Math.round(width * ratio)
      canvas.height = Math.round(height * ratio)
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0)
    }

    const draw = () => {
      if (!width || !height) return
      const dotColor = cssColor(frame, "--overview-globe-dot", "#a1a1aa")
      const accent = cssColor(frame, "--analytics-accent", "#2563eb")
      const sphere = cssColor(frame, "--overview-globe-sphere", "#09090b")
      ctx.clearRect(0, 0, width, height)
      shown = []
      if (mode === "globe") {
        const radius = Math.min(width * 0.46, height * 0.62)
        const cx = width / 2, cy = height * 0.6
        const projection = d3.geoOrthographic().scale(radius).translate([cx, cy]).rotate(rotation).clipAngle(90)
        const center: [number, number] = [-rotation[0], -rotation[1]]
        const glow = ctx.createRadialGradient(cx, cy, radius * 0.97, cx, cy, radius * 1.06)
        glow.addColorStop(0, "transparent")
        glow.addColorStop(0.45, cssColor(frame, "--overview-globe-rim", "rgba(161,161,170,.28)"))
        glow.addColorStop(1, "transparent")
        ctx.fillStyle = glow
        ctx.beginPath(); ctx.arc(cx, cy, radius * 1.06, 0, Math.PI * 2); ctx.fill()
        ctx.fillStyle = sphere
        ctx.beginPath(); ctx.arc(cx, cy, radius, 0, Math.PI * 2); ctx.fill()
        ctx.fillStyle = dotColor
        for (const point of data.dots.globe) {
          const distance = d3.geoDistance(point, center)
          if (distance > Math.PI / 2) continue
          const xy = projection(point)
          if (!xy) continue
          const depth = Math.cos(distance)
          ctx.globalAlpha = 0.2 + depth * 0.8
          const size = Math.max(1, radius / 140) * (0.6 + depth * 0.4)
          ctx.fillRect(xy[0] - size / 2, xy[1] - size / 2, size, size)
        }
        ctx.globalAlpha = 1
        for (const marker of markers) {
          if (d3.geoDistance(marker.lonLat, center) > Math.PI / 2 - 0.05) continue
          const xy = projection(marker.lonLat)
          if (xy) shown.push({ marker, x: xy[0], y: xy[1], r: 2.5 + 3.5 * Math.sqrt(marker.visitors / max) })
        }
      } else {
        const projection = d3.geoEquirectangular().fitExtent([[12, 16], [width - 12, height - 12]], { type: "Sphere" } as d3.GeoPermissibleObjects)
        ctx.fillStyle = dotColor
        ctx.globalAlpha = 0.7
        const size = Math.max(1, width / 520)
        for (const point of data.dots.map) {
          const xy = projection(point)
          if (xy) ctx.fillRect(xy[0] - size / 2, xy[1] - size / 2, size, size)
        }
        ctx.globalAlpha = 1
        for (const marker of markers) {
          const xy = projection(marker.lonLat)
          if (xy) shown.push({ marker, x: xy[0], y: xy[1], r: 2.5 + 3.5 * Math.sqrt(marker.visitors / max) })
        }
      }
      for (const dot of shown) {
        const halo = ctx.createRadialGradient(dot.x, dot.y, dot.r, dot.x, dot.y, dot.r * 2.6)
        halo.addColorStop(0, accent)
        halo.addColorStop(1, "transparent")
        ctx.globalAlpha = 0.35
        ctx.fillStyle = halo
        ctx.beginPath(); ctx.arc(dot.x, dot.y, dot.r * 2.6, 0, Math.PI * 2); ctx.fill()
        ctx.globalAlpha = 1
        ctx.fillStyle = accent
        ctx.beginPath(); ctx.arc(dot.x, dot.y, dot.r, 0, Math.PI * 2); ctx.fill()
      }
      // Without a hover, label the busiest visitor in view.
      const label = shown.find((dot) => dot.marker.code === hoverRef.current) || (hoverRef.current ? undefined : shown.slice().sort((a, b) => b.marker.visitors - a.marker.visitors)[0])
      // Position is written straight to the element so rotation never re-renders React.
      if (label) {
        // Sit above-left of the marker, flipping away from the controls and the edges.
        const tipWidth = tipRef.current?.offsetWidth || 150, tipHeight = tipRef.current?.offsetHeight || 44
        const gap = label.r + 8
        let x = label.x - tipWidth - gap, y = label.y - tipHeight - gap
        if (x < 8) x = label.x + gap
        if (y < 60) y = label.y + gap
        tipAt.current = [Math.min(width - tipWidth - 8, x), Math.min(height - tipHeight - 8, y)]
        if (tipRef.current) tipRef.current.style.transform = `translate(${Math.round(tipAt.current[0])}px, ${Math.round(tipAt.current[1])}px)`
      }
      setTip(label?.marker || null)
    }

    const tick = () => {
      raf = 0
      if (mode === "globe" && !drag && !reduced) rotation = [rotation[0] + 0.08, rotation[1]]
      draw()
      if (mode === "globe" && !reduced && visible && document.visibilityState === "visible") raf = requestAnimationFrame(tick)
    }
    const start = () => { if (!raf) raf = requestAnimationFrame(tick) }

    const onDown = (event: PointerEvent) => {
      if (mode !== "globe") return
      drag = { x: event.clientX, y: event.clientY, rotation }
      canvas.setPointerCapture(event.pointerId)
    }
    const onMove = (event: PointerEvent) => {
      const rect = canvas.getBoundingClientRect()
      if (drag) {
        rotation = [drag.rotation[0] + (event.clientX - drag.x) * 0.3, Math.max(-60, Math.min(60, drag.rotation[1] - (event.clientY - drag.y) * 0.3))]
        if (reduced) draw()
        return
      }
      const x = event.clientX - rect.left, y = event.clientY - rect.top
      const hit = shown.find((dot) => Math.hypot(dot.x - x, dot.y - y) <= Math.max(10, dot.r + 4))
      const code = hit?.marker.code || null
      if (code !== hoverRef.current) {
        hoverRef.current = code
        draw()
      }
    }
    const onUp = () => { drag = null }
    const onLeave = () => { if (hoverRef.current) { hoverRef.current = null; draw() } }

    resize()
    draw()
    start()
    const observer = new ResizeObserver(() => { resize(); draw() })
    observer.observe(canvas)
    const seen = new IntersectionObserver(([entry]) => { visible = entry.isIntersecting; if (visible) start() })
    seen.observe(canvas)
    const onVisibility = () => { if (document.visibilityState === "visible") start() }
    document.addEventListener("visibilitychange", onVisibility)
    canvas.addEventListener("pointerdown", onDown)
    canvas.addEventListener("pointermove", onMove)
    canvas.addEventListener("pointerup", onUp)
    canvas.addEventListener("pointercancel", onUp)
    canvas.addEventListener("pointerleave", onLeave)
    return () => {
      cancelAnimationFrame(raf)
      observer.disconnect()
      seen.disconnect()
      document.removeEventListener("visibilitychange", onVisibility)
      canvas.removeEventListener("pointerdown", onDown)
      canvas.removeEventListener("pointermove", onMove)
      canvas.removeEventListener("pointerup", onUp)
      canvas.removeEventListener("pointercancel", onUp)
      canvas.removeEventListener("pointerleave", onLeave)
    }
  }, [data, markers, mode])

  function toggleFullscreen() {
    if (document.fullscreenElement) void document.exitFullscreen()
    else void frameRef.current?.requestFullscreen?.()
  }

  const top = tip?.sites[0]
  return (
    <div ref={frameRef} className={`overview-globe is-${mode}${fullscreen ? " is-fullscreen" : ""}`}>
      <canvas ref={canvasRef} className="overview-globe-canvas" role="img" aria-label={t("访客地区分布（近 24 小时）")} />
      {failed ? <p className="overview-globe-state" role="status">{t("地图加载失败")}</p> : null}
      {tip && top ? (
        <div ref={tipRef} className="squircle overview-globe-tip" style={{ transform: `translate(${Math.round(tipAt.current[0])}px, ${Math.round(tipAt.current[1])}px)` }}>
          <strong><CountryFlag code={tip.code} shape="rect" className="overview-globe-flag" />{countryName(tip.code, locale)}</strong>
          <span>{t("{n} 位访客 · {site}").replace("{n}", numberShort(top.visitors)).replace("{site}", top.name)}{tip.sites.length > 1 ? ` +${tip.sites.length - 1}` : ""}</span>
        </div>
      ) : null}
      <div className="overview-globe-tools">
        <div className="squircle overview-globe-mode" role="group" aria-label={t("地图样式")}>
          <button type="button" className="squircle" aria-pressed={mode === "globe"} onClick={() => setMode("globe")}>{t("地球")}</button>
          <button type="button" className="squircle" aria-pressed={mode === "map"} onClick={() => setMode("map")}>{t("平面")}</button>
        </div>
        <button type="button" className="squircle overview-globe-expand" aria-label={t(fullscreen ? "退出全屏" : "全屏")} title={t(fullscreen ? "退出全屏" : "全屏")} onClick={toggleFullscreen}>
          <UIIcon name="expand" />
        </button>
      </div>
    </div>
  )
}
