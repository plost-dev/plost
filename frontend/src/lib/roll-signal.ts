import { useSyncExternalStore } from "react"

// A page-wide signal that every rolling number turns over once: the site overview's refresh button sends it, so the
// figures in the overview and on every site card roll together.
let count = 0
const listeners = new Set<() => void>()

export function rollAllNumbers() {
  count++
  listeners.forEach((listener) => listener())
}

export function useRollCount() {
  return useSyncExternalStore((listener) => { listeners.add(listener); return () => { listeners.delete(listener) } }, () => count, () => count)
}
