import { useSyncExternalStore } from "react"

export type Toast = { id: number, text: string, tone: "success" | "error", duration: number }

// Short confirmations leave quickly; a sentence that adds a condition stays long enough to read.
const durationFor = (text: string) => text.length > 14 ? 3600 : 2400

let current: Toast | null = null
let nextId = 0
let timer: ReturnType<typeof setTimeout> | undefined
const listeners = new Set<() => void>()
const emit = () => listeners.forEach(listener => listener())
const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }

/**
 * Shows a short message just below the top bar; a newer one replaces the one on screen.
 * The text is a translation key, translated when it is drawn, so a language switched at the
 * same moment as the save is reflected in the message.
 */
export function showToast(text: string, tone: Toast["tone"] = "success") {
  const duration = durationFor(text)
  current = { id: ++nextId, text, tone, duration }
  clearTimeout(timer)
  timer = setTimeout(() => { current = null; emit() }, duration)
  emit()
}

export function useToast() {
  return useSyncExternalStore(subscribe, () => current, () => null)
}
