// Lists interface strings that a language catalog in src/locales/ does not translate yet, and placeholders
// ({0}, {n}, …) a translation dropped. Run: bun scripts/check-locales.ts (exits 1 when something is missing).
// Sources are the Simplified Chinese texts: the keyed ZH table in src/lib/i18n.tsx and the keys of ui-en.json.
import { readFileSync, readdirSync } from "fs"

const i18n = readFileSync("src/lib/i18n.tsx", "utf8")
const zhBlock = i18n.slice(i18n.indexOf("const ZH"), i18n.indexOf("const EN"))
const sources = new Set<string>([
  ...[...zhBlock.matchAll(/^\s*"[^"]+": "((?:[^"\\]|\\.)*)",$/gm)].map(match => JSON.parse(`"${match[1]}"`) as string),
  ...Object.keys(JSON.parse(readFileSync("src/lib/ui-en.json", "utf8"))),
])
const placeholders = (text: string) => [...text.matchAll(/\{\w+\}/g)].map(match => match[0]).sort().join(" ")

let problems = 0
for (const file of readdirSync("src/locales").filter(name => name.endsWith(".json")).sort()) {
  const catalog: Record<string, string> = JSON.parse(readFileSync(`src/locales/${file}`, "utf8"))
  const missing = [...sources].filter(source => typeof catalog[source] !== "string" || !catalog[source].trim())
  const broken = [...sources].filter(source => catalog[source] && new Set(placeholders(catalog[source]).split(" ")).size < new Set(placeholders(source).split(" ")).size)
  if (!missing.length && !broken.length) continue
  problems += missing.length + broken.length
  console.log(`${file}: ${missing.length} missing, ${broken.length} with dropped placeholders`)
  for (const source of [...missing, ...broken].slice(0, 20)) console.log(`  ${source}`)
}
console.log(problems ? `${problems} problems` : "All languages cover every string.")
process.exit(problems ? 1 : 0)
