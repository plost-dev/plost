import type { HttpCheck } from "./site-checks"

export function expiryState(expiresAt: string | null | undefined, now: string) {
  // A registry may publish a civil date without a timezone. Keep that precision:
  // compare calendar dates, and do not claim expiry at an invented midnight.
  if (expiresAt && /^\d{4}-\d{2}-\d{2}$/.test(expiresAt)) {
    const end = Date.parse(expiresAt), current = Date.parse(now)
    if (!Number.isFinite(end) || !Number.isFinite(current) || new Date(end).toISOString().slice(0, 10) !== expiresAt) return {days: null, tone: "unknown" as const}
    const days = Math.round((end - Date.parse(new Date(current).toISOString().slice(0, 10))) / 86400_000)
    return {days: Math.max(0, days), tone: days < 0 ? "bad" as const : days <= 30 ? "warning" as const : "good" as const}
  }
  const remaining = expiresAt ? Date.parse(expiresAt) - Date.parse(now) : NaN
  if (!Number.isFinite(remaining)) return {days: null, tone: "unknown" as const}
  return {days: Math.max(0, Math.ceil(remaining / 86400_000)), tone: remaining <= 0 ? "bad" as const : remaining <= 30 * 86400_000 ? "warning" as const : "good" as const}
}

/** Empty intervals remain unknown; the ratio describes observed checks, not unobserved time. */
export function checkSummary(checks: HttpCheck[], now: string) {
  const end = Date.parse(now)
  const start = end - 86400_000
  const observed = checks.filter(check => Date.parse(check.checkedAt) > start && Date.parse(check.checkedAt) <= end)
  const successes = observed.filter(check => check.ok)
  const times = successes.flatMap(check => check.durationMs == null ? [] : [check.durationMs]).sort((a, b) => a - b)
  const buckets = Array.from({length: 48}, (_, index) => {
    const from = start + index * 1800_000
    const rows = observed.filter(check => Date.parse(check.checkedAt) > from && Date.parse(check.checkedAt) <= from + 1800_000)
    return {from: new Date(from).toISOString(), total: rows.length, failed: rows.filter(check => !check.ok).length,
      tone: !rows.length ? "unknown" : rows.some(check => !check.ok) ? "bad" : "good"}
  })
  return {total: observed.length, rate: observed.length ? successes.length / observed.length * 100 : null,
    median: times.length ? (times[Math.floor((times.length - 1) / 2)] + times[Math.floor(times.length / 2)]) / 2 : null, buckets}
}

export function responseSegments(checks: HttpCheck[]) {
  const segments: HttpCheck[][] = []
  let segment: HttpCheck[] = []
  for (const check of checks) {
    if (segment.length && Date.parse(check.checkedAt) - Date.parse(segment.at(-1)!.checkedAt) > 10 * 60_000) {segments.push(segment); segment = []}
    if (!check.ok || check.durationMs == null) {if (segment.length) segments.push(segment); segment = []; continue}
    segment.push(check)
  }
  if (segment.length) segments.push(segment)
  return segments
}
