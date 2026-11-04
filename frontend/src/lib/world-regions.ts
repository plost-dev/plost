import * as d3 from "d3"
import * as topojson from "topojson-client"
import type { Feature, MultiLineString, MultiPolygon, Polygon, Position } from "geojson"
import regionMapUrl from "../assets/world-regions.bin?url"
import { decodeTopology } from "./world-map-format"
import { subdivisionCode } from "./geography"

/** c: country, i: ISO 3166-2 code ("" where Natural Earth has none), n/z: English and Chinese names. */
export type RegionProps = { c: string, i: string, n: string, z: string }
export type RegionFeature = Feature<Polygon | MultiPolygon, RegionProps>
export type CountryShape = Feature<Polygon | MultiPolygon, { code: string }>

export type RegionMap = {
  regions: RegionFeature[]
  /** Each country as the union of its provinces, so its border is exactly where its provinces end. */
  countries: CountryShape[]
  /** The lines between provinces of the same country. */
  regionLines: MultiLineString
  centroids: Map<string, [number, number]>
  /** Finds the province a report row names: an ISO 3166-2 code, or a name from the online GeoIP service. */
  regionCode: (country: string, name: string) => string
  /** The middle of a province's main part, by its ISO 3166-2 code. */
  regionCenter: (code: string) => [number, number] | undefined
  /** The province of a country that holds a place, or "" when none does. */
  regionAt: (country: string, lon: number, lat: number) => string
}

let mapPromise: Promise<RegionMap> | undefined

// One fetch and one conversion for every map on the page; a failed load is not kept, so retrying can recover.
export function loadRegionMap(): Promise<RegionMap> {
  return mapPromise ??= fetch(regionMapUrl).then(async (response) => {
    if (!response.ok) throw new Error("Unable to load the region map")
    return buildRegionMap(decodeTopology(await response.arrayBuffer()) as never)
  }).catch((error) => {
    mapPromise = undefined
    throw error
  })
}

type Topology = Parameters<typeof topojson.feature>[0] & {
  objects: { regions: { type: "GeometryCollection", geometries: Array<{ properties: RegionProps }> } }
}

function buildRegionMap(topology: Topology): RegionMap {
  const object = topology.objects.regions
  const regions = (topojson.feature(topology, object as never) as unknown as { features: RegionFeature[] }).features
  const byCountry = new Map<string, typeof object.geometries>()
  for (const geometry of object.geometries) {
    const list = byCountry.get(geometry.properties.c)
    if (list) list.push(geometry)
    else byCountry.set(geometry.properties.c, [geometry])
  }
  const countries: CountryShape[] = []
  const centroids = new Map<string, [number, number]>()
  for (const [code, geometries] of byCountry) {
    const shape: CountryShape = { type: "Feature", properties: { code }, geometry: topojson.merge(topology, geometries as never) }
    countries.push(shape)
    if (!code) continue
    // The middle of the main landmass, so a flag is not pulled out to sea by far islands.
    const c = d3.geoCentroid(mainland(shape) as never)
    if (Number.isFinite(c[0]) && Number.isFinite(c[1])) centroids.set(code, c)
  }
  const regionLines = topojson.mesh(topology, object as never, (a, b) => {
    const pa = (a as unknown as { properties: RegionProps }).properties, pb = (b as unknown as { properties: RegionProps }).properties
    return a !== b && !!pa.c && pa.c === pb.c
  })

  const names = new Map<string, string>()
  for (const { properties: p } of regions) {
    if (!p.i) continue
    for (const name of [p.n, p.z]) {
      if (!name) continue
      names.set(nameKey(p.c, name), p.i)
      if (p.c === "CN") names.set(nameKey(p.c, name.replace(CHINESE_SUFFIX, "")), p.i)
    }
  }
  const regionCode = (country: string, name: string) => {
    const value = name.trim().toUpperCase()
    if (/^[A-Z]{2}-[A-Z0-9]{1,3}$/.test(value)) return value
    return names.get(nameKey(country, name)) || names.get(nameKey(country, name.replace(CHINESE_SUFFIX, ""))) || subdivisionCode(country, name)
  }
  const byCode = new Map<string, RegionFeature[]>()
  const ofCountry = new Map<string, RegionFeature[]>()
  for (const region of regions) {
    const { c, i } = region.properties
    if (i) byCode.set(i, [...byCode.get(i) || [], region])
    if (c) ofCountry.set(c, [...ofCountry.get(c) || [], region])
  }
  const centers = new Map<string, [number, number] | undefined>()
  const regionCenter = (code: string) => {
    if (!centers.has(code)) {
      const parts = byCode.get(code) || []
      const largest = parts.flatMap(part => part.geometry.type === "Polygon" ? [part.geometry.coordinates] : part.geometry.coordinates)
        .map(coordinates => ({ coordinates, area: d3.geoArea({ type: "Polygon", coordinates }) }))
        .sort((a, b) => b.area - a.area)[0]
      const c = largest ? d3.geoCentroid({ type: "Polygon", coordinates: largest.coordinates }) : undefined
      centers.set(code, c && Number.isFinite(c[0]) && Number.isFinite(c[1]) ? c : undefined)
    }
    return centers.get(code)
  }
  // Places come rounded to two decimals and repeat on every refresh, so each is looked up once.
  const found = new Map<string, string>()
  const regionAt = (country: string, lon: number, lat: number) => {
    const key = `${country}:${lon}:${lat}`
    let code = found.get(key)
    if (code === undefined) {
      code = (ofCountry.get(country) || []).find(region => region.properties.i && d3.geoContains(region as never, [lon, lat]))?.properties.i || ""
      found.set(key, code)
    }
    return code
  }
  return { regions, countries, regionLines, centroids, regionCode, regionCenter, regionAt }
}

const CHINESE_SUFFIX = /(?:壮族自治区|回族自治区|维吾尔自治区|自治区|特别行政区|省|市)$/
const nameKey = (country: string, name: string) => `${country.toUpperCase()}:${name.normalize("NFKC").trim().toLowerCase()}`

/**
 * The parts of a country that matter for framing it: polygons at least 1.5% the size of its largest, which leaves out
 * remote islands (the Azores, the Canaries) that would otherwise shrink the country to a corner of the map.
 */
export function mainland(shape: CountryShape): CountryShape {
  const polygons: Position[][][] = shape.geometry.type === "Polygon" ? [shape.geometry.coordinates] : shape.geometry.coordinates
  const areas = polygons.map(coordinates => d3.geoArea({ type: "Polygon", coordinates }))
  const largest = Math.max(...areas)
  return { ...shape, geometry: { type: "MultiPolygon", coordinates: polygons.filter((_, i) => areas[i] >= largest * 0.015) } }
}
