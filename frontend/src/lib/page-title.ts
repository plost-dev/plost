import type { Locale } from "./i18n"

const labels = {
  sites: { "zh-CN": "站点", en: "Sites" },
  settings: { "zh-CN": "设置", en: "Settings" },
  addSite: { "zh-CN": "添加站点", en: "Add site" },
  siteSettings: { "zh-CN": "站点设置", en: "Site settings" },
  activity: { "zh-CN": "活动日志", en: "Activity log" },
  health: { "zh-CN": "站点检查", en: "Site check" },
  login: { "zh-CN": "登录", en: "Log in" },
  setup: { "zh-CN": "初始化", en: "Set up" },
  share: { "zh-CN": "公开统计", en: "Shared statistics" },
  api: { "zh-CN": "API 文档", en: "API reference" },
  notFound: { "zh-CN": "页面不存在", en: "Page not found" },
} as const

type PageLabel = keyof typeof labels

function domainLabel(segment: string): string | undefined {
  try {
    const domain = decodeURIComponent(segment)
    // A title uses text only, but malformed paths must not become misleading domains.
    if (!domain || /[\s/\\?#\u0000-\u001f\u007f]/u.test(domain)) return undefined
    return domain
  } catch {
    return undefined
  }
}

export function pageTitle(pathname: string, locale: Locale): string {
  const path = pathname.replace(/\/+$/, "") || "/"
  if (path === "/") return "Plost"

  let label: PageLabel = "notFound"
  if (path === "/dashboard" || path === "/sites") label = "sites"
  else if (path === "/account" || path === "/backup") label = "settings"
  else if (path === "/sites/new") label = "addSite"
  else if (path === "/login") label = "login"
  else if (path === "/setup") label = "setup"
  else if (path === "/docs/api") label = "api"
  else if (/^\/share\/[^/]+$/.test(path)) label = "share"
  else {
    const site = /^\/sites\/([^/]+)(?:\/(settings|activity|health))?$/.exec(path)
    if (site && domainLabel(site[1])) {
      if (site[2] === "settings") label = "siteSettings"
      else if (site[2] === "activity") label = "activity"
      else if (site[2] === "health") label = "health"
      else return `${domainLabel(site[1])} | Plost`
    }
  }
  return `${labels[label][locale]} | Plost`
}
