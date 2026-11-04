import subdivisions from "../assets/subdivisions.json"
import { countryName } from "./countries"
import { isChinese, toTraditional, translate, type Locale } from "./i18n"
import { Converter } from "opencc-js/t2cn"

const names: Record<string, string[]> = subdivisions
const aliases = new Map<string, string>()
const simplifiedChinese = Converter({from: "t", to: "cn"})
/** Chinese place names in the script of the interface: simplified, or traditional for zh-TW. */
const chineseScript = (locale: string, value: string) => locale === "zh-TW" ? toTraditional(simplifiedChinese(value)) : simplifiedChinese(value)
const clean = (value: string) => simplifiedChinese(value).normalize("NFKC").trim().toLowerCase()

function alias(country: string, value: string) {
  return `${country}:${clean(value)}`
}

for (const [code, translations] of Object.entries(names)) {
  for (const text of translations) {
    if (!text) continue
    const country = code.slice(0, 2).toUpperCase()
    aliases.set(alias(country, text), code)
    if (country === "CN") aliases.set(alias(country, text.replace(/(?:壮族自治区|回族自治区|维吾尔自治区|自治区|特别行政区|省|市)$/, "")), code)
  }
}

/** Resolve imported ISO codes and provider names without rewriting stored groups. */
export function subdivisionCode(country: string, region: string): string {
  country = country.trim().toUpperCase()
  const key = clean(region).replace(/[-_]/g, "")
  const code = (key.startsWith(country.toLowerCase()) && names[key] ? key : names[country.toLowerCase() + key] ? country.toLowerCase() + key : undefined)
    || aliases.get(alias(country, region))
  return code ? `${code.slice(0, 2).toUpperCase()}-${code.slice(2).toUpperCase()}` : ""
}

export function geographyLabel(kind: "region" | "city", name: string, country: string, locale: string): string {
  const explicitCountry = name.match(/^([a-z]{2})[-_]/i)?.[1]
  const nation = (country || explicitCountry || "").trim().toUpperCase()
  const code = kind === "region" ? subdivisionCode(nation, name) : ""
  const translations = code ? names[code.replace("-", "").toLowerCase()] : undefined
  const unknown = translate(locale as Locale, kind === "city" ? "未知城市" : "未知地区")
  const raw = name.trim()
  // A city without a Chinese name keeps its own (English) name rather than a placeholder that hides which city it is.
  if (kind === "city" && isChinese(locale) && /\p{L}/u.test(raw.replace(/\p{Script=Han}/gu,""))) {
    return nation ? `${countryName(nation, locale)} · ${raw}` : raw
  }
  const text = translations?.[isChinese(locale) ? 1 : 0] || translations?.[0]
    || (!raw || /^\d+$/.test(raw) || /^\(?none\)?$/i.test(raw) || /^[A-Z]{2}[-_][A-Z0-9]{1,4}$/.test(raw) ? unknown : raw)
  const fullName = nation ? `${countryName(nation, locale)} · ${text}` : text
  return isChinese(locale) ? chineseScript(locale, fullName) : fullName
}

/** A province's own name without its country, for the map: CLDR's translation, else the names the map carries. */
export function regionName(code: string, english: string, chinese: string, locale: string): string {
  const translations = code ? names[code.replace("-", "").toLowerCase()] : undefined
  if (!isChinese(locale)) return translations?.[0] || english || code
  return chineseScript(locale, translations?.[1] || chinese || translations?.[0] || english || code)
}
