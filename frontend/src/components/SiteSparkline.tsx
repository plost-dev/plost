import { createContext, useContext, useId, type ReactNode } from "react"
import { curveMonotoneX, line } from "d3"
import { DEFAULT_SPARKLINE, type SparklinePreference, type SparklineStyle } from "~/lib/sparkline-styles"

const SparklineContext = createContext<SparklinePreference>(DEFAULT_SPARKLINE)

export function SparklineProvider({ value, children }: { value: SparklinePreference; children: ReactNode }) {
  return <SparklineContext.Provider value={value}>{children}</SparklineContext.Provider>
}

export function useSparkline(): SparklinePreference {
  return useContext(SparklineContext)
}

const W = 240
const H = 48

/** The trend on a site card, drawn in currentColor in one of the account's chosen styles. */
export function SiteSparkline({ values, style, className = "h-12 w-full" }: { values: number[]; style?: SparklineStyle; className?: string }) {
  const preference = useSparkline()
  const fade = useId()
  const chosen = style ?? preference.style
  const pts = values.length ? values : [0, 0]
  const n = pts.length
  const max = Math.max(1, ...pts)
  const min = Math.min(...pts)
  const slot = W / n
  const cx = (i: number) => (i + 0.5) * slot
  const top = (v: number) => H - 2 - (v / max) * (H - 4)
  // Lines use the whole height, so a flat stretch still reads as a line rather than hugging the floor.
  const ly = (v: number) => max === min ? H / 2 : H - 3 - ((v - min) / (max - min)) * (H - 6)
  const lineX = (i: number) => n > 1 ? (i / (n - 1)) * W : W / 2
  const areaUnder = (d: string) => `${d} L${lineX(n - 1).toFixed(1)},${H} L${lineX(0).toFixed(1)},${H} Z`
  const stroke = { fill: "none", stroke: "currentColor", strokeWidth: 2, strokeLinejoin: "round" as const, strokeLinecap: "round" as const, vectorEffect: "non-scaling-stroke" as const }
  const bars = (opacity: (i: number) => number, width = 0.62) => pts.map((v, i) => {
    const w = Math.max(1, slot * width)
    return v > 0 ? <rect key={i} x={cx(i) - w / 2} y={top(v)} width={w} height={H - top(v)} rx={1.5} fill="currentColor" opacity={opacity(i)} /> : null
  })
  const window = n > 24 ? 7 : 4
  const average = pts.map((_, i) => { const s = pts.slice(Math.max(0, i - window + 1), i + 1); return s.reduce((a, b) => a + b, 0) / s.length })

  let body: ReactNode
  switch (chosen) {
    case "bars": body = bars(() => 0.85); break
    case "bars-average": body = <>{bars(() => 0.28)}<path d={average.map((v, i) => `${i ? "L" : "M"}${cx(i).toFixed(1)},${top(v).toFixed(1)}`).join("")} {...stroke} strokeWidth={1.8} /></>; break
    case "smooth": {
      const d = line<number>().x((_, i) => lineX(i)).y(ly).curve(curveMonotoneX)(pts) || ""
      body = <><defs><linearGradient id={fade} x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="currentColor" stopOpacity={0.32} /><stop offset="1" stopColor="currentColor" stopOpacity={0} /></linearGradient></defs>
        <path d={areaUnder(d)} fill={`url(#${CSS.escape(fade)})`} /><path d={d} {...stroke} strokeWidth={1.8} /></>
      break
    }
    // Bars grown both ways from the middle, like an audio waveform.
    case "mirror": body = pts.map((v, i) => {
      const w = Math.max(1, slot * 0.5), h = Math.max(1.5, (v / max) * (H - 4))
      return <rect key={i} x={cx(i) - w / 2} y={(H - h) / 2} width={w} height={h} rx={w / 2} fill="currentColor" opacity={0.85} />
    }); break
    case "lollipop": body = pts.map((v, i) => <g key={i}>
      <line x1={cx(i)} x2={cx(i)} y1={H} y2={top(v) + 3} stroke="currentColor" strokeWidth={1} opacity={0.35} vectorEffect="non-scaling-stroke" />
      <circle cx={cx(i)} cy={top(v) + 3} r={2.3} fill="currentColor" />
    </g>); break
    // Each bar as a stack of up to six cells, the top one full strength.
    case "blocks": body = pts.flatMap((v, i) => {
      const cells = v > 0 ? Math.max(1, Math.round((v / max) * 6)) : 0, ch = (H - 2) / 6, w = Math.max(1, slot - 2)
      return Array.from({ length: cells }, (_, k) => <rect key={`${i}-${k}`} x={i * slot + 1} y={H - (k + 1) * ch} width={w} height={ch - 1.5} rx={1} fill="currentColor" opacity={k === cells - 1 ? 1 : 0.4} />)
    }); break
    case "step": body = <path d={pts.map((v, i) => `${i ? `H${(i * slot).toFixed(1)}V` : "M0,"}${ly(v).toFixed(1)}`).join("") + `H${W}`} {...stroke} strokeWidth={1.6} strokeLinejoin="miter" />; break
    case "dots": body = pts.map((v, i) => <circle key={i} cx={cx(i)} cy={ly(v)} r={2.2} fill="currentColor" opacity={i === n - 1 ? 1 : 0.55} />); break
    case "heat": body = pts.map((v, i) => <rect key={i} x={i * slot + 1} y={14} width={Math.max(1, slot - 2)} height={20} rx={2.5} fill="currentColor" opacity={0.12 + 0.88 * (max === min ? 0 : (v - min) / (max - min))} />); break
    default: {
      // The original look: scaled from zero, like the bars.
      const d = pts.map((v, i) => `${i ? "L" : "M"}${lineX(i).toFixed(1)},${top(v).toFixed(1)}`).join("")
      body = <><path d={areaUnder(d)} fill="currentColor" opacity={0.12} /><path d={d} {...stroke} /></>
    }
  }
  return <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className={className} aria-hidden="true">{body}</svg>
}
