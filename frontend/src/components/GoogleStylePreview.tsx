import * as d3 from "d3"
import { useEffect, useId, useState } from "react"
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

// The Mapbox previews' place and framing (public/map-previews/SOURCES.md): 720 × 440 pixels at Mapbox zoom 11.3,
// which is Google zoom 11 at 360 × 220 doubled for sharp screens.
const STATIC_MAP = "https://maps.googleapis.com/maps/api/staticmap?center=37.77,-122.44&zoom=11&size=360x220&scale=2"

type Rule = { featureType?: string; elementType?: string; stylers: Array<Record<string, string>> }
/** A Maps JavaScript style array as the Static Maps API's style parameters. */
function staticStyle(rules: Rule[]) {
  return rules.map(rule => "&style=" + encodeURIComponent([
    rule.featureType ? `feature:${rule.featureType}` : "", rule.elementType ? `element:${rule.elementType}` : "",
    ...rule.stylers.flatMap(Object.entries).map(([k, v]) => `${k}:${v.replace(/^#/, "0x")}`),
  ].filter(Boolean).join("|"))).join("")
}

/**
 * The real Google map for a style as a Static Maps API image with the saved key, at the same place as the Mapbox
 * previews. Google's terms do not allow keeping its images, so the page asks Google for them each time; when one
 * cannot load (no key yet, or the Static Maps API not allowed for the key), the drawn preview is shown instead.
 * Standard has a light and a dark image, and CSS shows the one for the current mode.
 */
export function GoogleStylePreview({styleId, label, apiKey}: {styleId: GoogleMapStyleId; label: string; apiKey?: string}) {
  const {locale} = useT()
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [apiKey])
  if (!apiKey || failed) return <DrawnPreview styleId={styleId} label={label}/>
  const url = (dark: boolean) => `${STATIC_MAP}&language=${locale === "en" ? "en" : "zh-CN"}${staticStyle(googleMapLook(styleId, dark).styles)}&key=${encodeURIComponent(apiKey)}`
  const image = (dark: boolean, mode?: string) => <img className="squircle map-style-preview google-static-preview" data-mode={mode} src={url(dark)} alt={label} width="360" height="220" loading="lazy" decoding="async" referrerPolicy="strict-origin-when-cross-origin" onError={() => setFailed(true)}/>
  return styleId === "standard" ? <>{image(false, "light")}{image(true, "dark")}</> : image(styleId === "dark")
}

/**
 * A preview drawn in each style's own colours from the built-in world map, for when the real map cannot be shown.
 * Standard switches with light and dark mode in CSS.
 */
function DrawnPreview({styleId, label}: {styleId: GoogleMapStyleId; label: string}) {
  const clip = useId()
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
