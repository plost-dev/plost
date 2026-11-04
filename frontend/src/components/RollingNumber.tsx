import { useEffect, useRef, useState } from "react"
import { useRollCount } from "~/lib/roll-signal"

/**
 * A number whose digits roll like an odometer: forward to the new digit when the value changes, and once all the way
 * round when the page asks every number to roll (the refresh button). A number that just appeared, such as the site
 * cards after switching between cards and the compact list, shows its digits at once. Anything that is
 * not a digit (a decimal point, k, %) stays still. Screen readers get the plain text.
 */
export function RollingNumber({ text }: { text: string }) {
  const spin = useRollCount()
  const chars = [...text]
  return (
    <span className="rolling-number" aria-label={text} role="text">
      {chars.map((c, i) => {
        // Keyed from the right, so the ones digit stays the same element when the number grows a digit.
        const key = chars.length - i
        return /\d/.test(c)
          ? <RollDigit key={key} digit={Number(c)} spin={spin} delay={i * 60} />
          : <span key={`c${key}`} aria-hidden="true">{c}</span>
      })}
    </span>
  )
}

// Four rounds of 0–9: a roll moves at most one round plus nine, and the strip is put back in the first round after.
const STRIP = Array.from({ length: 40 }, (_, i) => i % 10)

function RollDigit({ digit, spin, delay }: { digit: number, spin: number, delay: number }) {
  const [pos, setPos] = useState(digit)
  const [still, setStill] = useState(true)
  const seen = useRef({ spin, digit })
  useEffect(() => {
    const prev = seen.current
    seen.current = { spin, digit }
    if (prev.spin === spin && prev.digit === digit) return
    const round = prev.spin !== spin ? 10 : 0
    setStill(false)
    setPos((p) => p + ((digit - (p % 10) + 10) % 10) + round)
  }, [digit, spin])
  return (
    <span className="rolling-digit" aria-hidden="true">
      <span className="rolling-strip" style={{ transform: `translateY(${-pos * 10 / 4}%)`, transition: still ? "none" : undefined, transitionDelay: still ? undefined : `${delay}ms` }}
        onTransitionEnd={() => { setStill(true); setPos((p) => p % 10) }}>
        {STRIP.map((n, i) => <span key={i}>{n}</span>)}
      </span>
      <span className="rolling-sizer">{digit}</span>
    </span>
  )
}
