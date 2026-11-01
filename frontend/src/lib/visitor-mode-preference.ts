export type VisitorMode = "persistent" | "sessions"

const STORAGE_KEY = "litestats:visitor-mode"

export function readVisitorMode(storage?: Storage): VisitorMode {
  try { return storage?.getItem(STORAGE_KEY) === "sessions" ? "sessions" : "persistent" }
  catch { return "persistent" }
}

export function saveVisitorMode(mode: VisitorMode, storage?: Storage) {
  try { storage?.setItem(STORAGE_KEY, mode) } catch { /* Local switching still works without storage. */ }
}
