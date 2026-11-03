import * as d3 from "d3"
import { useEffect, useState } from "react"
import { loadWorldMap } from "~/lib/world-countries"
import type { GoogleMapStyleId } from "~/lib/google-map-styles"

const WIDTH = 360, HEIGHT = 220
let outlines: Promise<string> | undefined

/** The country outlines framed on Asia and Europe, close to the opening Google view, drawn once for all three previews. */
function worldOutlines() {
  return outlines ??= loadWorldMap().then(features => {
    const projection = d3.geoMercator().center([85, 25]).scale(WIDTH / (2 * Math.PI) * 2).translate([WIDTH / 2, HEIGHT / 2])
    return d3.geoPath(projection)({type: "FeatureCollection", features}) || ""
  }).catch(error => { outlines = undefined; throw error })
}

/**
 * A preview drawn in each style's own colours from the built-in world map, not a Google screenshot: Google's tiles can
 * only be shown through the Maps API with a key. Standard switches with light and dark mode in CSS.
 */
export function GoogleStylePreview({styleId, label}: {styleId: GoogleMapStyleId; label: string}) {
  const [path, setPath] = useState("")
  useEffect(() => { let live = true; void worldOutlines().then(d => { if (live) setPath(d) }, () => {}); return () => { live = false } }, [])
  return <svg className="squircle map-style-preview google-style-preview" data-style={styleId} viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" aria-label={label}>
    <rect width={WIDTH} height={HEIGHT} className="google-style-water"/>
    {path ? <path d={path} className="google-style-land"/> : null}
  </svg>
}
