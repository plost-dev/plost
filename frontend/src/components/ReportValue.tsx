import { CountryFlag, RowIcon, type RowIconKind } from "./icons"
import { useT } from "~/lib/i18n"
import { languageCode, languageFlag } from "~/lib/languages"
import { reportValueLabel } from "~/lib/report-values"

/** Presentation only: the original name remains the query and row identity. */
export function ReportValue({kind, name, label, code, wrap = false, interactive = true}: {
  kind?: string
  name: string
  label?: string
  code?: string
  wrap?: boolean
  interactive?: boolean
}) {
  const {locale} = useT()
  const text = reportValueLabel(kind, name, locale, label, code)
  const iconKind = ["source", "ai", "channel", "country", "browser", "os", "device"].includes(kind || "") ? kind as RowIconKind : undefined
  return <span className="report-value">
    {kind === "language" ? <span className="report-value-icon" title={locale === "en" ? "Locale symbol, not visitor location" : "语言地区标识，不代表访客位置"}><CountryFlag code={languageFlag(name)}/></span> : iconKind ? <span className="report-value-icon"><RowIcon kind={iconKind} name={name}/></span> : null}
    {kind === "language" ? <span className="language-row-copy"><span>{text}</span>{languageCode(name) ? <span className="language-row-code" lang="en">{languageCode(name)}</span> : null}</span> : <span title={text} className={`report-value-label${interactive ? " group-hover/row:underline" : ""}${wrap ? "" : " truncate"}`}>{text}</span>}
  </span>
}
