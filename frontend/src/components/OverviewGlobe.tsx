import { useEffect, useMemo, useRef, useState } from "react"
import * as d3 from "d3"
import { UIIcon } from "~/components/UIIcon"
import { useT } from "~/lib/i18n"
import { COUNTRIES_BY_TWO_LETTER_CODE, countryCentroids, loadWorldMap, type WorldJsonCountryData } from "~/lib/world-countries"

/** Visitors online now at one place; a place without coordinates is a whole country. `last` is the latest visit
 * there, Unix seconds: the newest place glows. */
export type OverviewPlace = { lat?: number, lon?: number, country: string, visitors: number, last: number }
type Mode = "globe" | "map"
type Marker = OverviewPlace & { key: string, lonLat: [number, number] }
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

// One breath of the glow around the newest visit.
const PULSE_MS = 1800

function cssColor(el: Element, name: string, fallback: string) {
  return getComputedStyle(el).getPropertyValue(name).trim() || fallback
}

/** Right-clicking the map (or the context-menu key while it has focus) opens a small menu to switch globe and flat map. */
export function OverviewGlobe({ places }: { places: OverviewPlace[] }) {
  const { t } = useT()
  const [mode, setMode] = useState<Mode>("globe")
  const [menu, setMenu] = useState<{ x: number, y: number } | null>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const [data, setData] = useState<{ dots: Dots, features: WorldJsonCountryData[] } | null>(null)
  const [failed, setFailed] = useState(false)
  const frameRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  // The turn survives new data: every push of fresh places redraws, but the globe keeps spinning from where it is.
  const rotationRef = useRef<[number, number] | null>(null)
  const aimedRef = useRef(false)

  useEffect(() => {
    if (!menu) return
    menuRef.current?.querySelector<HTMLButtonElement>("[aria-checked='true']")?.focus()
    const close = (event: Event) => { if (!(event.target instanceof Node && menuRef.current?.contains(event.target))) setMenu(null) }
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return
      setMenu(null)
      frameRef.current?.focus()
    }
    document.addEventListener("pointerdown", close, true)
    document.addEventListener("scroll", close, true)
    document.addEventListener("keydown", onKey)
    window.addEventListener("blur", close)
    return () => {
      document.removeEventListener("pointerdown", close, true)
      document.removeEventListener("scroll", close, true)
      document.removeEventListener("keydown", onKey)
      window.removeEventListener("blur", close)
    }
  }, [menu])

  useEffect(() => {
    let alive = true
    loadDots().then((value) => { if (alive) setData(value) }).catch(() => { if (alive) setFailed(true) })
    return () => { alive = false }
  }, [])

  const markers = useMemo<Marker[]>(() => {
    if (!data) return []
    const centroids = countryCentroids(data.features)
    return places.flatMap((place) => {
      const key = `${place.country}:${place.lat ?? ""}:${place.lon ?? ""}`
      if (place.lat !== undefined && place.lon !== undefined) return [{ ...place, key, lonLat: [place.lon, place.lat] as [number, number] }]
      const a3 = COUNTRIES_BY_TWO_LETTER_CODE[place.country]?.alpha_3
      const lonLat = a3 ? centroids.get(a3) : undefined
      return lonLat ? [{ ...place, key, lonLat }] : []
    })
  }, [places, data])

  useEffect(() => {
    const canvas = canvasRef.current
    const frame = frameRef.current
    if (!canvas || !frame || !data) return
    const ctx = canvas.getContext("2d")!
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches
    const max = Math.max(1, ...markers.map((m) => m.visitors))
    const newest = markers.reduce<Marker | null>((best, m) => !best || m.last > best.last ? m : best, null)?.key
    let width = 0, height = 0, raf = 0, visible = true
    // Tilted towards the viewer, so the globe is seen a little from above, with the north half in front. It turns to
    // the busiest place once, when the first visitors arrive; after that it only ever keeps turning.
    if (!aimedRef.current && markers[0]) {
      rotationRef.current = [-markers[0].lonLat[0], -25]
      aimedRef.current = true
    }
    let rotation: [number, number] = rotationRef.current ?? [-20, -25]
    let drag: { x: number, y: number, rotation: [number, number] } | null = null
    let shown: Array<{ x: number, y: number, r: number, newest: boolean }> = []
    // Small dots; a busier place is only a little larger.
    const dotSize = (m: Marker) => 1.4 + Math.sqrt(m.visitors / max)

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
        // As in the reference design: the top of the globe shows whole and the lower part runs past the bottom edge.
        const radius = Math.min(width * 0.46, height * 0.62)
        const cx = width / 2, cy = height * 0.68
        const projection = d3.geoOrthographic().scale(radius).translate([cx, cy]).rotate(rotation).clipAngle(90)
        const center: [number, number] = [-rotation[0], -rotation[1]]
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
          if (xy) shown.push({ x: xy[0], y: xy[1], r: dotSize(marker), newest: marker.key === newest })
        }
      } else {
        // The whole world is too wide for the card and leaves it half empty, so the map is fitted to 125°W–160°E and
        // 50°S–75°N, from the American west coast to Japan and eastern Australia; the far edges run off the card.
        const projection = d3.geoEquirectangular().fitExtent([[12, 68], [width - 12, height - 16]], { type: "MultiPoint", coordinates: [[-125, 75], [160, -50]] } as d3.GeoPermissibleObjects)
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
          if (xy) shown.push({ x: xy[0], y: xy[1], r: dotSize(marker), newest: marker.key === newest })
        }
      }
      // Plain dots for everyone online; the place of the newest visit breathes a ring and a soft glow.
      ctx.fillStyle = accent
      for (const dot of shown) {
        if (dot.newest) continue
        ctx.beginPath(); ctx.arc(dot.x, dot.y, dot.r, 0, Math.PI * 2); ctx.fill()
      }
      const fresh = shown.find((dot) => dot.newest)
      if (fresh) {
        if (!reduced) {
          const phase = (performance.now() % PULSE_MS) / PULSE_MS
          ctx.globalAlpha = 0.45 * (1 - phase)
          ctx.beginPath(); ctx.arc(fresh.x, fresh.y, fresh.r + fresh.r * 4 * phase, 0, Math.PI * 2); ctx.fill()
          ctx.globalAlpha = 1
        }
        ctx.save()
        ctx.shadowColor = accent
        ctx.shadowBlur = 10
        ctx.beginPath(); ctx.arc(fresh.x, fresh.y, fresh.r + 0.6, 0, Math.PI * 2); ctx.fill()
        ctx.restore()
      }
    }

    const tick = () => {
      raf = 0
      if (mode === "globe" && !drag && !reduced) rotation = [(rotation[0] + 0.08) % 360, rotation[1]]
      rotationRef.current = rotation
      draw()
      // The flat map stands still, but keeps drawing while the newest dot breathes.
      if (!reduced && (mode === "globe" || newest) && visible && document.visibilityState === "visible") raf = requestAnimationFrame(tick)
    }
    const start = () => { if (!raf) raf = requestAnimationFrame(tick) }

    const onDown = (event: PointerEvent) => {
      if (mode !== "globe" || event.button !== 0) return
      drag = { x: event.clientX, y: event.clientY, rotation }
      canvas.setPointerCapture(event.pointerId)
    }
    const onMove = (event: PointerEvent) => {
      if (!drag) return
      rotation = [drag.rotation[0] + (event.clientX - drag.x) * 0.3, Math.max(-60, Math.min(60, drag.rotation[1] - (event.clientY - drag.y) * 0.3))]
      rotationRef.current = rotation
      if (reduced) draw()
    }
    const onUp = () => { drag = null }

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
    return () => {
      cancelAnimationFrame(raf)
      observer.disconnect()
      seen.disconnect()
      document.removeEventListener("visibilitychange", onVisibility)
      canvas.removeEventListener("pointerdown", onDown)
      canvas.removeEventListener("pointermove", onMove)
      canvas.removeEventListener("pointerup", onUp)
      canvas.removeEventListener("pointercancel", onUp)
    }
  }, [data, markers, mode])

  return (
    <div ref={frameRef} className={`overview-globe is-${mode}`} tabIndex={0} title={t("右键切换地球或平面地图")}
      onContextMenu={(event) => {
        event.preventDefault()
        // Kept inside the map, since the card clips anything past its edge.
        const rect = event.currentTarget.getBoundingClientRect()
        setMenu({ x: Math.min(event.clientX - rect.left, rect.width - 136), y: Math.min(event.clientY - rect.top, rect.height - 88) })
      }}
      onKeyDown={(event) => {
        if (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10")) return
        event.preventDefault()
        setMenu({ x: event.currentTarget.clientWidth / 2, y: event.currentTarget.clientHeight / 2 })
      }}>
      <canvas ref={canvasRef} className="overview-globe-canvas" role="img" aria-label={t("当前在线访客位置")} />
      {failed ? <p className="overview-globe-state" role="status">{t("地图加载失败")}</p> : null}
      {menu ? (
        <div ref={menuRef} role="menu" aria-label={t("地图样式")} className="squircle overview-globe-menu" style={{ left: menu.x, top: menu.y }}>
          {(["globe", "map"] as const).map((value) => (
            <button key={value} type="button" role="menuitemradio" aria-checked={mode === value} className="squircle"
              onClick={() => { setMode(value); setMenu(null); frameRef.current?.focus() }}>
              {t(value === "globe" ? "地球" : "平面")}
              {mode === value ? <UIIcon name="check" /> : null}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
