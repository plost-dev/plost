import { translate, type Locale } from "./i18n"
const zh: Record<string, string> = {
  CN: "中国", US: "美国", JP: "日本", KR: "韩国", GB: "英国", DE: "德国", FR: "法国",
  RU: "俄罗斯", IN: "印度", SG: "新加坡", HK: "中国香港", TW: "中国台湾", AU: "澳大利亚",
  CA: "加拿大", BR: "巴西", NL: "荷兰", IT: "意大利", ES: "西班牙", VN: "越南", TH: "泰国",
  PL: "波兰", EE: "爱沙尼亚", LV: "拉脱维亚", LT: "立陶宛", FI: "芬兰", SE: "瑞典",
  NO: "挪威", DK: "丹麦", IE: "爱尔兰", PT: "葡萄牙", AT: "奥地利", CH: "瑞士",
  BE: "比利时", CZ: "捷克", HU: "匈牙利", RO: "罗马尼亚", BG: "保加利亚", GR: "希腊",
  TR: "土耳其", UA: "乌克兰", IL: "以色列", AE: "阿联酋", SA: "沙特阿拉伯",
  ID: "印度尼西亚", MY: "马来西亚", PH: "菲律宾", NZ: "新西兰", MX: "墨西哥",
  AR: "阿根廷", CL: "智利", CO: "哥伦比亚", ZA: "南非", EG: "埃及", NG: "尼日利亚",
  PK: "巴基斯坦", BD: "孟加拉", KH: "柬埔寨", LA: "老挝", MM: "缅甸", NP: "尼泊尔",
  KZ: "哈萨克斯坦", UZ: "乌兹别克斯坦", MO: "中国澳门",
}

export function countryName(code: string, locale = "zh-CN") {
  const key = (code || "").trim().toUpperCase()
  if (!key || key === "(NONE)") return translate(locale as Locale, "未知")
  if (locale !== "zh-CN" && /^[A-Z]{2}$/.test(key)) {
    try { return new Intl.DisplayNames([locale], { type: "region" }).of(key) || key } catch { return key }
  }
  return zh[key] || (/^[A-Z]{2}$/.test(key) ? new Intl.DisplayNames(["zh-CN"], { type: "region" }).of(key) : undefined) || key
}
