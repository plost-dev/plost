import { intlLocale } from "./i18n"
const THOUSAND = 1000
const HUNDRED_THOUSAND = 100000
const MILLION = 1000000

export function numberShort(num: number): string {
  if (num >= THOUSAND && num < MILLION) {
    const thousands = num / THOUSAND
    if (thousands === Math.floor(thousands) || num >= HUNDRED_THOUSAND) return `${Math.floor(thousands)}k`
    return `${Math.floor(thousands * 10) / 10}k`
  }
  if (num >= MILLION) {
    const millions = num / MILLION
    if (millions === Math.floor(millions) || num >= 100000000) return `${Math.floor(millions)}M`
    return `${Math.floor(millions * 10) / 10}M`
  }
  return String(num)
}

export function percentShort(n: number) {
  if (Math.abs(n) > 0 && Math.abs(n) < 0.1) return `${n.toFixed(2)}%`
  return `${n.toFixed(1).replace(/\.0$/, "")}%`
}

const WEEKDAYS = ["星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"]
const MONTHS_EN = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]

export function parseStamp(date: string) {
  if (date.includes(" ")) {
    const [d, t] = date.split(" ")
    const [y, m, day] = d.split("-").map(Number)
    const [hh, mm] = (t || "00:00").split(":").map(Number)
    return new Date(y, (m || 1) - 1, day || 1, hh || 0, mm || 0)
  }
  const [y, m, day] = date.split("-").map(Number)
  return new Date(y, (m || 1) - 1, day || 1)
}

const isChineseLocale = (locale: string) => locale === "zh-CN" || locale === "zh-TW"
const intlDate = (locale: string, d: Date, options: Intl.DateTimeFormatOptions) => new Intl.DateTimeFormat(intlLocale(locale), options).format(d)

export function formatDayShort(date: string, withYear = false, locale = "en") {
  const d = parseStamp(date)
  if (locale !== "en" && !isChineseLocale(locale)) return intlDate(locale, d, { day: "numeric", month: "short", ...(withYear ? { year: "numeric" } : {}) })
  const label = locale === "en" ? `${d.getDate()} ${MONTHS_EN[d.getMonth()]}` : `${d.getMonth() + 1}月${d.getDate()}日`
  return withYear ? (locale === "en" ? `${label} ${d.getFullYear()}` : `${d.getFullYear()}年${label}`) : label
}

export function formatDayLong(date: string, withYear = false, locale = "zh-CN") {
  const d = parseStamp(date)
  if (locale !== "en" && !isChineseLocale(locale)) return intlDate(locale, d, { weekday: "long", day: "numeric", month: "short", ...(withYear ? { year: "numeric" } : {}) })
  const weekday = locale === "en" ? new Intl.DateTimeFormat("en", { weekday: "long" }).format(d) : WEEKDAYS[d.getDay()]
  const day = locale === "en" ? `${weekday}, ${d.getDate()} ${MONTHS_EN[d.getMonth()]}` : `${d.getMonth() + 1}月${d.getDate()}日 ${weekday}`
  return withYear ? `${day} ${d.getFullYear()}` : day
}

export function formatMonthYYYY(date: string, locale = "en") {
  const d = parseStamp(date)
  if (locale !== "en" && !isChineseLocale(locale)) return intlDate(locale, d, { month: "long", year: "numeric" })
  return locale === "en" ? `${MONTHS_EN[d.getMonth()]} ${d.getFullYear()}` : `${d.getFullYear()}年${d.getMonth() + 1}月`
}

export function formatClock(date: string, withMinutes = false) {
  const d = parseStamp(date)
  const h = d.getHours()
  const m = String(d.getMinutes()).padStart(2, "0")
  return withMinutes ? `${h}:${m}` : `${h}:00`
}

export function formatDuration(sec: number) {
  const s = Math.max(0, Math.round(sec))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const r = s % 60
  if (h) return `${h}h ${String(m).padStart(2, "0")}m`
  if (m) return `${m}m ${String(r).padStart(2, "0")}s`
  return `${s}s`
}

/** Explicit locale and timezone keep server and browser rendering identical. */
export function formatTimestamp(value: string | number, locale = "zh-CN") {
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return "—"
  return new Intl.DateTimeFormat(locale === "zh-CN" ? locale : intlLocale(locale), { timeZone: "UTC", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23" }).format(date) + " UTC"
}
