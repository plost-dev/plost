import { useEffect, useRef, type CSSProperties } from "react"
import geometry from "~/assets/brand.json"

function GlyphPath({ letter }: { letter: typeof geometry.letters[number] }) {
  return <path d={letter.d} fillRule={letter.rule as "evenodd" | "nonzero"}
    fill={letter.stroke ? "none" : "currentColor"}
    stroke={letter.stroke ? "currentColor" : undefined}
    strokeWidth={letter.stroke ? geometry.stroke_width : undefined}
    strokeLinecap="butt" strokeLinejoin="round" />
}

// The same O stays visible as it moves into the wordmark. Rendering another O
// in the wordmark would produce a double stroke halfway through the transition.
export function LogoMark({ className = "logo-mark" }: { className?: string }) {
  return (
    <svg className={className} viewBox={geometry.mark_bb.join(" ")} aria-hidden="true">
      <g transform={`translate(${geometry.mark_dx} 0)`}>
        <path d={geometry.mark_d} fillRule="evenodd" fill="none" stroke="currentColor"
          strokeWidth={geometry.stroke_width} strokeLinecap="butt" strokeLinejoin="round" />
      </g>
    </svg>
  )
}

export function BrandWordmark() {
  return (
    <svg className="brand-wordmark" viewBox={geometry.vb.join(" ")} role="img" aria-label="Plost">
      {geometry.letters.map(letter => (
        <g key={letter.key} transform={`translate(${letter.dx} 0)`}>
          <GlyphPath letter={letter} />
        </g>
      ))}
    </svg>
  )
}

// Alternates between the centered O and the full wordmark while it is on screen.
// Hover and keyboard focus still expand it; reduced-motion users keep the O.
function useWordmarkLoop(enabled: boolean) {
  const ref = useRef<HTMLSpanElement>(null)
  useEffect(() => {
    const el = ref.current
    if (!enabled || !el || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return
    let expanded = false
    const tick = () => {
      if (el.matches(":hover")) return
      expanded = !expanded
      el.classList.toggle("is-expanded", expanded)
    }
    const start = window.setTimeout(tick, 900)
    const loop = window.setInterval(tick, 2600)
    return () => { window.clearTimeout(start); window.clearInterval(loop); el.classList.remove("is-expanded") }
  }, [enabled])
  return ref
}

export function BrandMark({ className = "", animated = false }: { className?: string, animated?: boolean }) {
  const ref = useWordmarkLoop(animated)
  const style = {
    "--plost-mark-width": `${geometry.css.mk_w * 100}%`,
    "--plost-wordmark-width": `${geometry.css.wm_w * 100}%`,
    "--plost-wordmark-height": `${geometry.css.wm_h * 100}%`,
    "--plost-morph-scale": geometry.css.k,
    "--plost-morph-x": geometry.css.tx,
    "--plost-morph-y": geometry.css.ty,
  } as CSSProperties
  return (
    <span ref={ref} className={`logo plost-logo${animated ? " plost-logo--animated" : ""} ${className}`.trim()} role="img" aria-label="Plost" style={style}>
      <span className="plost-logo__badge">
        <span className="plost-logo__pill" aria-hidden="true" />
        <span className="plost-logo__tile">
          <svg className="plost-logo__shape" viewBox="0 0 100 100" aria-hidden="true">
            <path className="plost-logo__shape-bg" d={geometry.shape} />
            <path className="plost-logo__shape-hl" d={geometry.inner} />
          </svg>
        </span>
        <svg className="plost-logo__wordmark" viewBox={geometry.vb.join(" ")} aria-hidden="true">
          {geometry.letters.filter(letter => letter.key !== geometry.mark_key).map(letter => (
            <g key={letter.key} transform={`translate(${letter.dx} 0)`}>
              <GlyphPath letter={letter} />
            </g>
          ))}
        </svg>
        <LogoMark className="plost-logo__mark" />
      </span>
    </span>
  )
}
