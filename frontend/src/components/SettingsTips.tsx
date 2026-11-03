import { useT } from "~/lib/i18n"
import { Tile } from "./ui"

/** A settings page's notes, gathered in one numbered card at the bottom instead of spread over its forms. */
export function SettingsTips({ tips, title = "使用说明" }: { tips: string[]; title?: string }) {
  const { t } = useT()
  return <Tile icon="key" title={title}>
    <ol className="settings-tips">{tips.map((tip, i) => <li key={i}><span className="squircle settings-tip-number">{i + 1}</span><span>{t(tip)}</span></li>)}</ol>
  </Tile>
}
