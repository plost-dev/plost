/** The whole instance counts either browsers ("persistent") or Umami-style IP + agent sessions ("sessions"). */
export type VisitorMode = "persistent" | "sessions"

export const visitorLabel = (mode?: VisitorMode) => mode === "sessions" ? "会话访客" : "独立访客"
