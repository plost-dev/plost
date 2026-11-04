// Regenerates src/locales/zh-TW.json from the Simplified Chinese source strings with OpenCC
// (mainland → Taiwan phrasing, e.g. 设置 → 設定, 账号 → 帳號). Run: bun scripts/build-zh-tw.ts
import { readFileSync, writeFileSync } from "fs"
import { Converter } from "opencc-js/cn2t"

const toTaiwan = Converter({ from: "cn", to: "twp" })
// Where OpenCC's character-by-character result is not what Taiwanese software says.
const OVERRIDES: Record<string, string> = {
  "退出登录": "登出",
  "近 90 天在线": "近 90 天上線率",
}
const i18n = readFileSync("src/lib/i18n.tsx", "utf8")
const zhBlock = i18n.slice(i18n.indexOf("const ZH"), i18n.indexOf("const EN"))
const sources = new Set<string>([
  ...[...zhBlock.matchAll(/^\s*"[^"]+": "((?:[^"\\]|\\.)*)",$/gm)].map(match => JSON.parse(`"${match[1]}"`) as string),
  ...Object.keys(JSON.parse(readFileSync("src/lib/ui-en.json", "utf8"))),
])
// Taiwanese software terms OpenCC leaves in their mainland form, applied after conversion.
const PHRASES: [string, string][] = [["自定義", "自訂"], ["會話", "工作階段"]]
const taiwan = (text: string) => PHRASES.reduce((value, [from, to]) => value.replaceAll(from, to), toTaiwan(text))
const catalog: Record<string, string> = {}
for (const source of [...sources].sort()) {
  // Placeholders, URLs and code stay byte-for-byte; only the prose around them is converted.
  catalog[source] = OVERRIDES[source] ?? source.split(/(\{\w+\}|https?:\/\/\S+|`[^`]*`)/).map((part, index) => index % 2 ? part : taiwan(part)).join("")
}
writeFileSync("src/locales/zh-TW.json", JSON.stringify(catalog, null, 1) + "\n")
console.log(`zh-TW: ${Object.keys(catalog).length} strings`)
