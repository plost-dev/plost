/**
 * How the whole instance counts visitors, visits and bounces: this program's own way, Umami's way (IP + user agent
 * sessions) or an approximation of Google Analytics 4.
 */
export type VisitorMode = "plost" | "umami" | "google"

export const visitorLabel = (mode?: VisitorMode) => mode === "umami" ? "会话访客" : mode === "google" ? "用户" : "独立访客"
export const visitLabel = (mode?: VisitorMode) => mode === "google" ? "会话" : "访问次数"
