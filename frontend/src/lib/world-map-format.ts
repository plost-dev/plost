// LSM1: 4-byte magic, uint32 LE JSON header length, UTF-8 TopoJSON header,
// then unsigned varints: arc count, each arc's point count and zigzag x/y deltas.
// Coordinates and the original quantization transform are preserved exactly.
export type WorldTopology = {
  type: "Topology"
  transform: { scale: [number, number], translate: [number, number] }
  objects: {
    countries: {
      type: "GeometryCollection"
      geometries: Array<({ type: "Polygon", arcs: number[][] } | { type: "MultiPolygon", arcs: number[][][] }) & {
        id?: string
        properties: { a3: string }
      }>
    }
  }
  arcs: number[][][]
}

export function decodeWorldMap(buffer: ArrayBuffer): WorldTopology {
  const topology = decodeTopology(buffer) as WorldTopology
  if (topology.objects?.countries?.type !== "GeometryCollection") throw new Error("Invalid world map asset")
  return topology
}

/** Decodes any topology in the LSM1 layout; world-regions.bin uses it for provinces (scripts/build-region-map.ts). */
export function decodeTopology(buffer: ArrayBuffer): { type: "Topology", transform: WorldTopology["transform"], objects: Record<string, unknown>, arcs: number[][][] } {
  const bytes = new Uint8Array(buffer)
  const view = new DataView(buffer)
  const invalid = () => new Error("Invalid world map asset")
  if (bytes.length < 8 || view.getUint32(0, true) !== 0x314d534c) throw invalid()
  const headerEnd = 8 + view.getUint32(4, true)
  if (headerEnd >= bytes.length) throw invalid()
  const header = JSON.parse(new TextDecoder().decode(bytes.subarray(8, headerEnd)))
  if (header.type !== "Topology" || !header.objects || typeof header.objects !== "object") throw invalid()
  let offset = headerEnd
  function uint() {
    let value = 0
    for (let shift = 0; shift <= 28; shift += 7) {
      if (offset >= bytes.length) throw invalid()
      const byte = bytes[offset++]
      value += (byte & 127) * 2 ** shift
      if (!(byte & 128)) return value
    }
    throw invalid()
  }
  const arcCount = uint()
  if (arcCount > bytes.length - offset) throw invalid()
  const arcs: number[][][] = []
  for (let i = 0; i < arcCount; i++) {
    const pointCount = uint()
    if (pointCount > (bytes.length - offset) / 2) throw invalid()
    const points: number[][] = []
    for (let j = 0; j < pointCount; j++) {
      const x = uint(), y = uint()
      points.push([x % 2 ? -(x + 1) / 2 : x / 2, y % 2 ? -(y + 1) / 2 : y / 2])
    }
    arcs.push(points)
  }
  if (offset !== bytes.length) throw invalid()
  return { ...header, arcs }
}
