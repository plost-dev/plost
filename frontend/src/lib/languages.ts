import { translate, type Locale } from "./i18n"
const NAMES: Record<string, string> = {
  zh: "中文", "zh-CN": "中文", "zh-Hans": "中文", "zh-TW": "中文（台湾）", "zh-Hant": "中文（繁体）", "zh-HK": "中文（香港）",
  en: "英语", "en-US": "英语", "en-GB": "英语（英国）", "en-AU": "英语（澳大利亚）",
  ja: "日语", "ja-JP": "日语",
  ko: "韩语", "ko-KR": "韩语",
  de: "德语", "de-DE": "德语",
  fr: "法语", "fr-FR": "法语",
  es: "西班牙语", "es-ES": "西班牙语", "es-MX": "西班牙语（墨西哥）",
  pt: "葡萄牙语", "pt-BR": "葡萄牙语（巴西）", "pt-PT": "葡萄牙语",
  ru: "俄语", "ru-RU": "俄语",
  it: "意大利语", "it-IT": "意大利语",
  nl: "荷兰语", "nl-NL": "荷兰语",
  pl: "波兰语", "pl-PL": "波兰语",
  tr: "土耳其语", "tr-TR": "土耳其语",
  ar: "阿拉伯语", "ar-SA": "阿拉伯语",
  hi: "印地语", "hi-IN": "印地语",
  th: "泰语", "th-TH": "泰语",
  vi: "越南语", "vi-VN": "越南语",
  id: "印尼语", "id-ID": "印尼语",
  sv: "瑞典语", "da": "丹麦语", "fi": "芬兰语", "no": "挪威语", "nb": "挪威语",
  et: "爱沙尼亚语", "et-EE": "爱沙尼亚语",
  cs: "捷克语", "hu": "匈牙利语", "ro": "罗马尼亚语", "el": "希腊语",
  uk: "乌克兰语", "he": "希伯来语", "fa": "波斯语",
}

export function languageCode(code: string) {
  // Historical values are display-only here. Strip invisible controls so they
  // cannot reorder the visible label/code or impersonate a different locale.
  const raw = (code || "").replace(/[\u0000-\u001f\u007f-\u009f\u061c\u200e\u200f\u202a-\u202e\u2066-\u2069]/g, "").trim()
  if (!raw || raw.toLowerCase() === "(none)") return ""
  const key = raw.replace(/_/g, "-")
  try { return Intl.getCanonicalLocales(key)[0] || key } catch { return key }
}

/** Locale decoration only; this must never be used as a visitor country. */
export function languageFlag(code: string) {
  const key = languageCode(code)
  if (!key) return "xx"
  try {
    const locale = new Intl.Locale(key)
    if (["und", "mul", "zxx"].includes(locale.language) && !locale.region) return "xx"
    const region = locale.region || locale.maximize().region
    return region && /^[A-Z]{2}$/.test(region) ? region.toLowerCase() : "xx"
  } catch { return "xx" }
}

export function languageName(code: string, locale = "zh-CN") {
  const key = languageCode(code)
  if (!key) return translate(locale as Locale, "未知")
  // DisplayNames does not accept Unicode/private-use extensions. Keep those in
  // the adjacent code, but use the base locale to describe the language.
  let base = key
  try { base = new Intl.Locale(key).baseName } catch { /* Preserve unknown historical text. */ }
  const short = base.split("-")[0]
  const exactName = Object.hasOwn(NAMES, base) ? NAMES[base] : undefined
  const shortName = Object.hasOwn(NAMES, short) ? NAMES[short] : undefined
  if (locale !== "zh-CN") {
    try { return new Intl.DisplayNames([locale], { type: "language" }).of(base) || base } catch { return base }
  }
  if (exactName) return exactName
  // A short-name fallback would hide valid region/script qualifiers such as
  // zh-Hant-TW, en-CA or fr-CA in Chinese while English retained them.
  try { return new Intl.DisplayNames(["zh-CN"], { type: "language" }).of(base) || shortName || base } catch { return shortName || base }
}

export function parseAcceptLanguage(header: string) {
  const first = (header || "").split(",")[0] || ""
  return first.split(";")[0].trim().replace("_", "-").slice(0, 16)
}

// Explicit historical country domains from https://www.google.com/supported_domains.
// A wildcard such as google.<anything> also accepts unrelated registered domains.
const GOOGLE_SEARCH_DOMAINS = new Set(`
  com ad ae com.af com.ag al am co.ao com.ar as at com.au az ba com.bd be bf bg com.bh bi bj com.bn
  com.bo com.br bs bt co.bw by com.bz ca cd cf cg ch ci co.ck cl cm cn com.co co.cr com.cu cv com.cy
  cz de dj dk dm com.do dz com.ec ee com.eg es com.et fi com.fj fm fr ga ge gg com.gh com.gi gl gm gr
  com.gt gy com.hk hn hr ht hu co.id ie co.il im co.in iq is it je com.jm jo co.jp co.ke com.kh ki kg
  co.kr com.kw kz la com.lb li lk co.ls lt lu lv com.ly co.ma md me mg mk ml com.mm mn com.mt mu mv mw
  com.mx com.my co.mz com.na com.ng com.ni ne nl no com.np nr nu co.nz com.om com.pa com.pe com.pg
  com.ph com.pk pl pn com.pr ps pt com.py com.qa ro ru rw com.sa com.sb sc se com.sg sh si sk com.sl
  sn so sm sr st com.sv td tg co.th com.tj tl tm tn to com.tr tt com.tw co.tz com.ua co.ug co.uk
  com.uy co.uz com.vc co.ve co.vi com.vn vu ws rs co.za co.zm co.zw cat
`.trim().split(/\s+/).map((suffix) => `google.${suffix}`))

const YANDEX_SEARCH_DOMAINS = new Set(["yandex.ru", "yandex.com", "yandex.by", "yandex.kz", "yandex.com.tr", "yandex.co.il", "yandex.uz", "ya.ru"])

function searchQueryParameters(url: URL): string[] {
  const host = url.hostname.toLowerCase().replace(/\.$/, "").replace(/^www\./, "")
  const path = url.pathname.replace(/\/$/, "")
  const rootOr = (...paths: string[]) => !path || paths.includes(path)
  if (GOOGLE_SEARCH_DOMAINS.has(host.replace(/^(encrypted|images)\./, "")) && rootOr("/search", "/webhp", "/imgres")) return ["q"]
  if (["bing.com", "cn.bing.com", "global.bing.com"].includes(host) && rootOr("/search", "/images/search", "/videos/search", "/news/search")) return ["q"]
  if ((host === "search.yahoo.com" || host.endsWith(".search.yahoo.com") || host === "search.yahoo.co.jp") && rootOr("/search", "/yhs/search")) return ["p"]
  if (["duckduckgo.com", "html.duckduckgo.com", "lite.duckduckgo.com"].includes(host) && rootOr("/html", "/lite")) return ["q"]
  if (["baidu.com", "m.baidu.com", "wap.baidu.com"].includes(host) && rootOr("/s")) return ["wd", "word"]
  if (YANDEX_SEARCH_DOMAINS.has(host) && rootOr("/search")) return ["text"]
  // Search forms verified at https://www.sogou.com/ and https://www.so.com/.
  if (["sogou.com", "m.sogou.com"].includes(host) && rootOr("/web", "/sogou")) return ["query"]
  if (["so.com", "m.so.com"].includes(host) && rootOr("/s")) return ["q"]
  // https://support.brave.com/hc/en-us/articles/4403568397837-Brave-Search-FAQ
  if (host === "search.brave.com" && rootOr("/search")) return ["q"]
  return []
}

export function extractSearchQuery(referrer: string) {
  try {
    const url = new URL(referrer)
    if (!["http:", "https:"].includes(url.protocol) || url.username || url.password) return ""
    for (const parameter of searchQueryParameters(url)) {
      const value = (url.searchParams.get(parameter) || "")
        .replace(/[\u0000-\u001f\u007f-\u009f]/g, " ")
        .replace(/[\u202a-\u202e\u2066-\u2069]/g, "")
        .replace(/\s+/g, " ").trim()
      // Match the collector's 300-code-unit limit without splitting a surrogate pair.
      if (value) return value.slice(0, 300).replace(/[\uD800-\uDBFF]$/, "")
    }
    return ""
  } catch {
    return ""
  }
}
