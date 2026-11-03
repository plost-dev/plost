import * as d3 from "d3"
import { useEffect, useId, useRef, useState } from "react"
import { loadGoogleMaps } from "./GoogleVisitorMap"
import { useT } from "~/lib/i18n"
import { loadWorldMap } from "~/lib/world-countries"
import { googleMapLook, type GoogleMapStyleId } from "~/lib/google-map-styles"

const WIDTH = 360, HEIGHT = 220
// Streets only: rough patches of green where Google's road map shows forest (Siberia, Southeast Asia, Europe, central
// Africa), kept inside the land outlines.
const projection = d3.geoMercator().center([85, 25]).scale(WIDTH / (2 * Math.PI) * 2).translate([WIDTH / 2, HEIGHT / 2])
const GREEN = ([[100, 60, 70, 9], [30, 56, 22, 8], [104, 16, 16, 12], [113, 27, 12, 7], [22, 0, 16, 12], [80, 22, 8, 8]] as const)
  .map(([lon, lat, rx, ry]) => { const [cx, cy] = projection([lon, lat])!; return {cx, cy, rx, ry} })
let outlines: Promise<string> | undefined

/** The country outlines framed on Asia and Europe, close to the opening Google view, drawn once for all three previews. */
function worldOutlines() {
  return outlines ??= loadWorldMap().then(features => {
    return d3.geoPath(projection)({type: "FeatureCollection", features}) || ""
  }).catch(error => { outlines = undefined; throw error })
}

// The Mapbox previews' camera (public/map-previews/SOURCES.md): a 720px-wide capture at zoom 11.3, shown shrunk to
// the card. Mapbox zoom levels use 512px tiles and Google's 256px, so the same view is one level higher in Google Maps,
// less however much the card shrinks the capture.
const PREVIEW_CENTER = { lat: 37.77, lng: -122.44 }
const previewZoom = (width: number) => 12.3 - Math.log2(720 / Math.max(width, 1))

/**
 * The real Google map for a style, live through the Maps JavaScript API with the saved key, at the same place as the
 * Mapbox previews. Google's terms do not allow keeping screenshots, so it is drawn each time; when it cannot load
 * (no key yet, a rejected key, or a page opened before Google Maps was turned on), the drawn preview is shown.
 */
export function GoogleStylePreview({styleId, label, apiKey}: {styleId: GoogleMapStyleId; label: string; apiKey?: string}) {
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [apiKey])
  return apiKey && !failed ? <GoogleLivePreview styleId={styleId} label={label} apiKey={apiKey} onFailure={() => setFailed(true)}/> : <DrawnPreview styleId={styleId} label={label}/>
}

function GoogleLivePreview({styleId, label, apiKey, onFailure}: {styleId: GoogleMapStyleId; label: string; apiKey: string; onFailure: () => void}) {
  const {locale} = useT()
  const container = useRef<HTMLDivElement>(null)
  const fail = useRef(onFailure)
  fail.current = onFailure
  useEffect(() => {
    const element = container.current
    if (!element) return
    let disposed = false
    let theme: MutationObserver | undefined
    let resize: ResizeObserver | undefined
    const failure = () => { if (!disposed) fail.current() }
    const timer = setTimeout(failure, 15_000)
    const previous = window.gm_authFailure
    const onAuthFailure = () => { previous?.(); failure() }
    window.gm_authFailure = onAuthFailure
    const dark = () => document.documentElement.dataset.theme === "dark"
    loadGoogleMaps(apiKey, locale === "en" ? "en" : "zh-CN").then(maps => {
      if (disposed) return
      clearTimeout(timer)
      const map = new maps.Map(element, {
        center: PREVIEW_CENTER, zoom: previewZoom(element.clientWidth), ...googleMapLook(styleId, dark()), isFractionalZoomEnabled: true,
        disableDefaultUI: true, gestureHandling: "none", keyboardShortcuts: false, clickableIcons: false,
      })
      theme = new MutationObserver(() => map.setOptions(googleMapLook(styleId, dark())))
      resize = new ResizeObserver(() => map.setOptions({center: PREVIEW_CENTER, zoom: previewZoom(element.clientWidth)}))
      resize.observe(element)
      theme.observe(document.documentElement, {attributes: true, attributeFilter: ["data-theme"]})
    }, failure)
    return () => {
      disposed = true; clearTimeout(timer); theme?.disconnect(); resize?.disconnect()
      if (window.gm_authFailure === onAuthFailure) window.gm_authFailure = previous
    }
  }, [apiKey, styleId, locale])
  return <div ref={container} className="squircle map-style-preview google-live-preview" role="img" aria-label={label}/>
}

/**
 * A preview drawn in each style's own colours from the built-in world map, for when the real map cannot be shown.
 * Standard switches with light and dark mode in CSS.
 */
function DrawnPreview({styleId, label}: {styleId: GoogleMapStyleId; label: string}) {  const clip = useId()
  const [path, setPath] = useState("")
  useEffect(() => { let live = true; void worldOutlines().then(d => { if (live) setPath(d) }, () => {}); return () => { live = false } }, [])
  return <svg className="squircle map-style-preview google-style-preview" data-style={styleId} viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" aria-label={label}>
    <rect width={WIDTH} height={HEIGHT} className="google-style-water"/>
    {path ? <path d={path} className="google-style-land"/> : null}
    {path && styleId === "roadmap" ? <>
      <clipPath id={clip}><path d={path}/></clipPath>
      <g clipPath={`url(#${CSS.escape(clip)})`} className="google-style-green">{GREEN.map(({cx, cy, rx, ry}) => <ellipse key={cx} cx={cx} cy={cy} rx={rx} ry={ry}/>)}</g>
      <path d={path} className="google-style-border"/>
    </> : null}
  </svg>
}
