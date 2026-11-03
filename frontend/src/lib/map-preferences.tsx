import { createContext, useContext, type ReactNode } from "react"
import { DEFAULT_MAPBOX_STYLE, normalizeMapboxStyle, type MapboxStyleId } from "./mapbox-styles"
import { DEFAULT_GOOGLE_MAP_STYLE, normalizeGoogleMapStyle, type GoogleMapStyleId } from "./google-map-styles"

const MapStyleContext = createContext<MapboxStyleId>(DEFAULT_MAPBOX_STYLE)
const GoogleMapStyleContext = createContext<GoogleMapStyleId>(DEFAULT_GOOGLE_MAP_STYLE)
const FLAGS_STORAGE_KEY = "litestats:map-flags"

export function MapStyleProvider({styleId, googleStyleId, children}: {styleId: MapboxStyleId; googleStyleId: GoogleMapStyleId; children: ReactNode}) {
  return <MapStyleContext.Provider value={normalizeMapboxStyle(styleId)}>
    <GoogleMapStyleContext.Provider value={normalizeGoogleMapStyle(googleStyleId)}>{children}</GoogleMapStyleContext.Provider>
  </MapStyleContext.Provider>
}

export function useMapStyle(): MapboxStyleId {
  return useContext(MapStyleContext)
}

export function useGoogleMapStyle(): GoogleMapStyleId {
  return useContext(GoogleMapStyleContext)
}

// Geography is loaded after hydration, so restore the preference before any
// flag markers are drawn. Missing, invalid or unavailable storage shows flags.
export function readMapFlags(): boolean {
  try { return localStorage.getItem(FLAGS_STORAGE_KEY) !== "hidden" } catch { return true }
}

export function saveMapFlags(show: boolean): void {
  try { localStorage.setItem(FLAGS_STORAGE_KEY, show ? "visible" : "hidden") } catch { /* Keep the current page usable when storage is blocked. */ }
}
