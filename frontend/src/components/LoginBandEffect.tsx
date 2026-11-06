import type { CSSProperties } from "react"
import type { LoginEffect } from "~/lib/login-effect"

// Fixed positions and timings, so every render and every preview looks the same.
const DOTS = Array.from({ length: 20 }, (_, i) => ({
  left: (i * 37 + 11) % 100,
  size: 2 + (i * 7) % 3,
  duration: 5 + (i * 13) % 6,
  delay: -((i * 29) % 100) / 10,
}))
const BARS = Array.from({ length: 18 }, (_, i) => ({ delay: -i * 0.31, duration: 2.4 + (i % 5) * 0.45 }))
const WAVE = "M0 60 C150 20 300 100 450 60 S750 20 900 60 S1050 100 1200 60 S1500 20 1650 60 S1950 100 2100 60 S2250 20 2400 60 V120 H0Z"
const LINE = "M0 200 L40 185 L80 192 L120 160 L160 172 L200 130 L240 145 L280 105 L320 118 L360 80 L400 70"

/** The animated layer behind the sign-in band's content. "shift" animates the band's own background instead. */
export function LoginBandEffect({ effect }: { effect: LoginEffect }) {
  if (effect === "aurora") return <div className="login-fx login-fx-aurora" aria-hidden="true"><i /><i /><i /></div>
  if (effect === "wave") return <div className="login-fx login-fx-wave" aria-hidden="true">
    <svg className="login-fx-wave-front" viewBox="0 0 2400 120" preserveAspectRatio="none"><path d={WAVE} /></svg>
    <svg className="login-fx-wave-back" viewBox="0 0 2400 120" preserveAspectRatio="none"><path d={WAVE} /></svg>
  </div>
  if (effect === "dots") return <div className="login-fx login-fx-dots" aria-hidden="true">
    {DOTS.map((dot, i) => <i key={i} style={{ left: `${dot.left}%`, width: dot.size, height: dot.size, animationDuration: `${dot.duration}s`, animationDelay: `${dot.delay}s` } as CSSProperties} />)}
  </div>
  if (effect === "bars") return <div className="login-fx login-fx-bars" aria-hidden="true">
    {BARS.map((bar, i) => <i key={i} style={{ animationDelay: `${bar.delay}s`, animationDuration: `${bar.duration}s` }} />)}
  </div>
  if (effect === "line") return <div className="login-fx login-fx-line" aria-hidden="true">
    <svg viewBox="0 0 400 240" preserveAspectRatio="none">
      <path className="login-fx-line-area" d={`${LINE} V240 H0Z`} />
      <path className="login-fx-line-stroke" d={LINE} pathLength={1} />
    </svg>
  </div>
  return null
}
