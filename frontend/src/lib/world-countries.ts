import * as d3 from "d3"
import * as topojson from "topojson-client"
import worldMapUrl from "../assets/world-50m.bin?url"
import { decodeWorldMap } from "./world-map-format"
import countriesMeta from "./countries_meta.json"
import type { Feature, MultiPolygon, Polygon } from "geojson"

export type WorldJsonCountryData = Feature<Polygon | MultiPolygon, {a3: string}>

let featuresPromise: Promise<WorldJsonCountryData[]> | undefined

// Concurrent maps and subsequent report updates share one fetch and conversion.
// A failed request is not cached, so the map's retry button can recover.
export function loadWorldMap(): Promise<WorldJsonCountryData[]> {
  return featuresPromise ??= fetch(worldMapUrl).then(async (response) => {
    if (!response.ok) throw new Error("Unable to load world map")
    const topology = decodeWorldMap(await response.arrayBuffer())
    const collection = topojson.feature(topology, topology.objects.countries)
    return collection.features as unknown as WorldJsonCountryData[]
  }).catch((error) => {
    featuresPromise = undefined
    throw error
  })
}

type CountryEntry = {
  alpha_3: string | null
  flag: string
}

const lookup: Record<string, CountryEntry> = {}
for (const [alpha2, pair] of Object.entries(countriesMeta as Record<string, string[]>)) {
  const [alpha3, flag] = pair
  lookup[alpha2] = { alpha_3: alpha3, flag }
}

export const COUNTRIES_BY_TWO_LETTER_CODE = lookup

let centroidCache: Map<string, [number, number]> | null = null
export function countryCentroids(features: WorldJsonCountryData[]) {
  if (centroidCache) return centroidCache
  const out = new Map<string, [number, number]>()
  for (const feature of features) {
    const c = d3.geoCentroid(feature as unknown as d3.GeoPermissibleObjects)
    if (Number.isFinite(c[0]) && Number.isFinite(c[1])) out.set(feature.properties.a3, c)
  }
  centroidCache = out
  return out
}
