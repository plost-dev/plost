// The Google Maps counterpart of the Mapbox catalog: the same three choices, kept local so a preference never becomes
// arbitrary style JSON. Standard follows light and dark mode; Streets and Dark keep their own colours.
export const GOOGLE_MAP_STYLES = [
  {id: "standard", label: "标准"},
  {id: "roadmap", label: "街道"},
  {id: "dark", label: "深色"},
] as const

export type GoogleMapStyleId = typeof GOOGLE_MAP_STYLES[number]["id"]
// Google's own road map is what people know it by, so it is the default; Mapbox keeps Standard.
export const DEFAULT_GOOGLE_MAP_STYLE: GoogleMapStyleId = "roadmap"

export function normalizeGoogleMapStyle(id: unknown): GoogleMapStyleId {
  return GOOGLE_MAP_STYLES.find(style => style.id === id)?.id || DEFAULT_GOOGLE_MAP_STYLE
}

const QUIET = [
  { featureType: "poi", stylers: [{ visibility: "off" }] }, { featureType: "road", stylers: [{ visibility: "off" }] }, { featureType: "transit", stylers: [{ visibility: "off" }] },
  { featureType: "administrative.province", stylers: [{ visibility: "off" }] }, { featureType: "administrative.locality", stylers: [{ visibility: "off" }] },
]
const LIGHT = [
  ...QUIET,
  { featureType: "landscape", stylers: [{ color: "#f4f4f5" }] }, { featureType: "water", stylers: [{ color: "#e4e4e7" }] },
  { elementType: "labels.text.fill", stylers: [{ color: "#71717a" }] }, { elementType: "labels.text.stroke", stylers: [{ color: "#ffffff" }] },
]
const DARK = [
  ...QUIET,
  { featureType: "landscape", stylers: [{ color: "#1c1c1f" }] }, { featureType: "water", stylers: [{ color: "#0c0c0e" }] },
  { featureType: "administrative", elementType: "geometry.stroke", stylers: [{ color: "#3f3f46" }] },
  { elementType: "labels.text.fill", stylers: [{ color: "#a1a1aa" }] }, { elementType: "labels.text.stroke", stylers: [{ color: "#000000" }] },
]

/** The Map options for a style; Streets is Google's own unstyled road map. */
export function googleMapLook(id: GoogleMapStyleId, dark: boolean) {
  if (id === "roadmap") return { styles: [], backgroundColor: "#90daee" }
  const night = id === "dark" || dark
  return { styles: night ? DARK : LIGHT, backgroundColor: night ? "#0c0c0e" : "#e4e4e7" }
}
