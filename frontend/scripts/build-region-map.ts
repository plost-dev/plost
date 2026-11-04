// Builds src/assets/world-regions.bin, the provinces and states of every country for the built-in visitor map, from
// Natural Earth's 1:10m admin-1 shapefile (public domain). Countries are not stored separately: the map merges each
// country's provinces, so country borders and province borders come from the same lines and always meet.
//
//   bun scripts/build-region-map.ts <ne_10m_admin_1_states_provinces.shp> <mapshaper>
//
// mapshaper (npm "mapshaper") simplifies the shapes and builds the topology; the result is written in the LSM1
// layout that src/lib/world-map-format.ts reads. See src/assets/world-regions-SOURCE.md.
import countriesMeta from "../src/lib/countries_meta.json"

const [shp, mapshaper] = process.argv.slice(2)
if (!shp || !mapshaper) throw new Error("usage: bun scripts/build-region-map.ts <admin-1 .shp> <mapshaper>")

// Natural Earth files these places under the country that governs them (or under none); GeoIP databases give them
// their own country codes. Keyed by ISO 3166-2 code where there is one, otherwise by name.
const OWN_COUNTRY: Record<string, string> = {
  "FR-GF": "GF", "FR-GP": "GP", "FR-MQ": "MQ", "FR-RE": "RE", "FR-YT": "YT", "NO-21": "SJ", "NO-X01~": "BV",
  "Christmas Island": "CX", "Cocos (Keeling) Islands": "CC",
}
// ISO 3166-2 renumbered Poland's and Czechia's regions; Natural Earth still has the old letters, GeoIP the new codes.
const RENAMED: Record<string, string> = {
  "PL-DS": "PL-02", "PL-KP": "PL-04", "PL-LU": "PL-06", "PL-LB": "PL-08", "PL-LD": "PL-10", "PL-MA": "PL-12", "PL-MZ": "PL-14", "PL-OP": "PL-16",
  "PL-PK": "PL-18", "PL-PD": "PL-20", "PL-PM": "PL-22", "PL-SL": "PL-24", "PL-SK": "PL-26", "PL-WN": "PL-28", "PL-WP": "PL-30", "PL-ZP": "PL-32",
  "CZ-PR": "CZ-10", "CZ-ST": "CZ-20", "CZ-JC": "CZ-31", "CZ-PL": "CZ-32", "CZ-KA": "CZ-41", "CZ-US": "CZ-42", "CZ-LI": "CZ-51", "CZ-KR": "CZ-52",
  "CZ-PA": "CZ-53", "CZ-VY": "CZ-63", "CZ-JM": "CZ-64", "CZ-OL": "CZ-71", "CZ-ZL": "CZ-72", "CZ-MO": "CZ-80",
}
const known = new Set(Object.keys(countriesMeta))

const tmp = `${process.env.TMPDIR || "/tmp"}/plost-regions-${process.pid}.json`
const run = Bun.spawnSync([mapshaper, shp, "-simplify", "15%", "weighted", "keep-shapes",
  "-filter-fields", "iso_a2,iso_3166_2,name,name_zh",
  "-o", tmp, "format=topojson", "quantization=100000"], { stdout: "inherit", stderr: "inherit" })
if (!run.success) throw new Error("mapshaper failed")
const source = Bun.file(tmp)

type Geometry = { type: string, arcs?: unknown, properties: Record<string, string> }
const topology = await source.json()
await source.delete()
const [layer] = Object.keys(topology.objects)
const collection = topology.objects[layer] as { type: string, geometries: Geometry[] }
const unplaced = new Set<string>()
collection.geometries = collection.geometries.filter(g => g.type !== null && g.arcs).map((g) => {
  const p = g.properties
  let country = OWN_COUNTRY[p.iso_3166_2] || OWN_COUNTRY[p.name] || p.iso_a2
  if (!known.has(country)) {
    unplaced.add(p.name)
    country = ""
  }
  const code = RENAMED[p.iso_3166_2] || (/^[A-Z]{2}-[A-Z0-9]{1,3}$/.test(p.iso_3166_2) ? p.iso_3166_2 : "")
  // c: country, i: ISO 3166-2 code ("" for areas without one), n/z: English and Chinese names for codes CLDR lacks.
  return { ...g, properties: { c: country, i: code, n: p.name || "", z: p.name_zh || "" } }
})
delete topology.objects[layer]
topology.objects.regions = collection
if (topology.bbox) delete topology.bbox

// LSM1: magic, header length, the topology without its arcs, then each arc's points as zigzag varints.
const arcs = topology.arcs as number[][][]
delete topology.arcs
const header = new TextEncoder().encode(JSON.stringify(topology))
const bytes: number[] = []
const uint = (value: number) => {
  while (value >= 128) {
    bytes.push((value & 127) | 128)
    value = Math.floor(value / 128)
  }
  bytes.push(value)
}
const zigzag = (value: number) => value < 0 ? -value * 2 - 1 : value * 2
uint(arcs.length)
for (const arc of arcs) {
  uint(arc.length)
  for (const [x, y] of arc) {
    uint(zigzag(x))
    uint(zigzag(y))
  }
}
const out = new Uint8Array(8 + header.length + bytes.length)
const view = new DataView(out.buffer)
view.setUint32(0, 0x314d534c, true)
view.setUint32(4, header.length, true)
out.set(header, 8)
out.set(bytes, 8 + header.length)
const target = new URL("../src/assets/world-regions.bin", import.meta.url)
await Bun.write(target, out)
console.log(`${collection.geometries.length} regions, ${arcs.length} arcs, ${out.length} bytes; without a country: ${[...unplaced].join(", ")}`)
