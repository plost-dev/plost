import { countryName } from "./countries"
import { translate, type Locale } from "./i18n"
import { languageCode, languageName } from "./languages"
import { isDirectSource } from "./source-icons"
import { geographyLabel } from "./geography"

type ReportValue = { name: string; label?: string; code?: string }

const BROWSER_NAMES: Record<string, string> = {
  "edge-chromium": "Edge",
  "chromium-webview": "Chromium WebView",
  "edge-ios": "Edge iOS",
  yandexbrowser: "Yandex Browser",
  fxios: "Firefox iOS",
  android: "Android Browser",
  ie: "Internet Explorer",
  silk: "Amazon Silk",
  instagram: "Instagram",
  facebook: "Facebook",
  pie: "Android Browser",
}

const CHANNEL_NAMES: Record<string, string> = {
  direct: "直接访问",
  organic_search: "自然搜索",
  organic_social: "社交媒体",
  paid_search: "付费搜索",
  paid_social: "付费社交",
  email: "电子邮件",
  display: "展示广告",
  affiliate: "联盟推广",
  ai: "AI 来源",
  referral: "外部引荐",
  other: "其他渠道",
}

const EVENT_NAMES: Record<string, string> = {
  pageview: "页面浏览",
  session: "新会话",
  "404": "404 页面",
  "File Download": "文件下载",
  "Outbound Link: Click": "外链点击",
}
/** The events the tracker sends by itself; any other name is a site's own custom event. */
export const isBuiltInEvent = (name: string) => Object.hasOwn(EVENT_NAMES, name)

const DEVICE_NAMES: Record<string, string> = {
  Desktop: "桌面电脑",
  Laptop: "笔记本电脑",
  Mobile: "手机",
  Tablet: "平板",
  "Ultra-wide": "超宽屏",
}

export function browserDisplayName(name: string): string {
  const value = name.trim()
  return BROWSER_NAMES[value.toLowerCase()] || value
}

/** Resolve display text at render time so changing locale cannot leave stale labels. */
export function reportValueLabel(kind: string | undefined, name: string, locale: Locale, label?: string, country?: string): string {
  if (kind === "language") return languageName(name, locale)
  if (kind === "country") return countryName(name, locale)
  if (kind === "region" || kind === "city") return geographyLabel(kind, kind === "city" && locale === "en" ? name : label || name, country || "", locale)
  if (kind === "source" && isDirectSource(name)) return translate(locale, "直接访问")
  if (kind === "channel" && Object.hasOwn(CHANNEL_NAMES, name)) return translate(locale, CHANNEL_NAMES[name])
  if (kind === "event" && Object.hasOwn(EVENT_NAMES, name)) return translate(locale, EVENT_NAMES[name])
  if (kind === "device" && Object.hasOwn(DEVICE_NAMES, name)) return translate(locale, DEVICE_NAMES[name])
  if ((kind === "browser" || kind === "os") && (!name.trim() || name.trim().toLowerCase() === "(none)")) {
    return translate(locale, "未知")
  }
  return kind === "browser" ? browserDisplayName(label || name) : label || name
}

/** Retain the stored key for search while exposing the current localized display value. */
export function reportSearchText(kind: string | undefined, row: ReportValue, locale: Locale): string {
  return [row.name, reportValueLabel(kind, row.name, locale, row.label, row.code), kind === "language" ? languageCode(row.name) : ""]
    .filter(Boolean).join(" ")
}

/** Return an original filter key, never a translated label or a merged historical group. */
export function resolveReportFilter(kind: string | undefined, entered: string, options: ReportValue[], locale: Locale): string {
  const value = entered.trim()
  // Exact stored keys must win even when another option uses that text as its label.
  const exact = options.find(option => option.name === entered) || options.find(option => option.name === value)
  if (exact) return exact.name
  const lower = value.toLowerCase()
  const insensitive = options.find(option => option.name.toLowerCase() === lower)
  if (insensitive) return insensitive.name
  const displayed = options.find(option => reportValueLabel(kind, option.name, locale, option.label, option.code).toLowerCase() === lower)
  if (displayed) return displayed.name
  // These are input affordances only. Other legacy direct aliases remain exact keys.
  if (kind === "source" && ["direct", "direct traffic", "直接访问"].includes(lower)) return "Direct"
  return kind === "country" ? value.toUpperCase() : value
}
