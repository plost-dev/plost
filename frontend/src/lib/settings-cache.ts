import { useCallback, useState } from "react"
import { siteDomainsFn } from "~/lib/actions"
import { apiKeysFn } from "~/lib/api-key-actions"
import { passkeyListFn } from "~/lib/auth-actions"
import { geoipSettingsFn } from "~/lib/geoip-actions"
import { rpc } from "~/lib/rpc"

// The settings pages' last answers, kept for the session. Switching back to a page shows them at once while a fresh
// read runs, instead of an empty card until the server answers; the layout reads them all ahead (prefetchSettings).
const values = new Map<string, unknown>()
const reading = new Map<string, Promise<unknown>>()

const READS = {
  geoip: geoipSettingsFn,
  apiKeys: apiKeysFn,
  siteDomains: siteDomainsFn,
  passkeys: passkeyListFn,
  backupSettings: () => rpc<unknown>("/api/backup/settings"),
  backupLocal: () => rpc<unknown>("/api/backup/local"),
} satisfies Record<string, () => Promise<unknown>>
export type SettingsRead = keyof typeof READS

/** Reads afresh (one request at a time per key) and remembers the answer. */
export function readSettings<T>(key: SettingsRead): Promise<T> {
  let next = reading.get(key) as Promise<T> | undefined
  if (!next) {
    next = (READS[key]() as Promise<T>).then(value => { values.set(key, value); return value }).finally(() => reading.delete(key))
    reading.set(key, next)
  }
  return next
}

export function cachedSettings<T>(key: SettingsRead): T | undefined {
  return values.get(key) as T | undefined
}

/** Starts every settings read in the background, so the pages already have their data when opened. */
export function prefetchSettings(): void {
  for (const key of Object.keys(READS) as SettingsRead[]) if (!values.has(key)) void readSettings(key).catch(() => {})
}

/** Forgets everything, such as after signing out. */
export function clearSettingsCache(): void {
  values.clear()
}

/** useState that starts from the remembered answer and remembers every change. */
export function useCachedState<T>(key: SettingsRead, initial: T): [T, (next: T | ((current: T) => T)) => void] {
  const [value, setValue] = useState<T>(() => values.has(key) ? values.get(key) as T : initial)
  const set = useCallback((next: T | ((current: T) => T)) => setValue(current => {
    const value = typeof next === "function" ? (next as (current: T) => T)(current) : next
    values.set(key, value)
    return value
  }), [key])
  return [value, set]
}
