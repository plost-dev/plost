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
