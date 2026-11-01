/**
 * Value axes on the dashboard all read the same way: whole steps of 1, 2 or 5 times a power of ten, and from a
 * thousand up whole thousands (from a million up whole millions), labelled 0, 1k, 2k. The exact value is shown when
 * hovering, never on the axis.
 */
// Values sit on 1, 2 or 5 times a power of ten with at most four steps above zero. From a thousand up the step is a
// whole number of thousands (and from a million up, of millions), so the axis reads 0, 1k, 2k, 3k instead of 500 and 1500.
export function axisScale(maxValue: number) {
  const raw = Math.max(maxValue, 1)
  const steps: number[] = []
  for (let exp = 0; exp <= 12; exp++) for (const m of [1, 2, 5]) steps.push(m * 10 ** exp)
  let step = steps.find(candidate => Math.ceil(raw / candidate) <= 4) ?? raw
  for (const unit of [1000, 1_000_000]) if (Math.ceil(raw / step) * step >= unit && step < unit) step = unit
  const max = Math.ceil(raw / step) * step
  const ticks: number[] = []
  for (let v = 0; v <= max + 1e-6; v += step) ticks.push(Math.round(v))
  return { max, ticks }
}

export function formatAxisTick(value: number) {
  const n = Math.round(value)
  if (n >= 1_000_000 && n % 1_000_000 === 0) return `${n / 1_000_000}M`
  if (n >= 1000 && n % 1000 === 0) return `${n / 1000}k`
  return String(n)
}
