import { translate, type Locale } from "./i18n"

const labels = {
  sites: "nav.sites",
  settings: "设置",
  addSite: "添加站点",
  siteSettings: "站点设置",
  activity: "活动日志",
  login: "登录",
  setup: "初始化",
  share: "公开统计",
  api: "API 文档",
  notFound: "页面不存在",
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
  else if (path === "/settings" || path.startsWith("/settings/")) label = "settings"
  else if (path === "/sites/new") label = "addSite"
  else if (path === "/login") label = "login"
  else if (path === "/setup") label = "setup"
  else if (path === "/docs/api") label = "api"
  else if (/^\/share\/[^/]+$/.test(path)) label = "share"
  else {
    const site = /^\/sites\/([^/]+)(?:\/(settings|activity))?(?:(?<=settings)\/[a-z-]+)?$/.exec(path)
    if (site && domainLabel(site[1])) {
      if (site[2] === "settings") label = "siteSettings"
      else if (site[2] === "activity") label = "activity"
      else return `${domainLabel(site[1])} | Plost`
    }
  }
  return `${translate(locale, labels[label])} | Plost`
}
