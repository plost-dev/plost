import { useT } from "~/lib/i18n"
import { Tile } from "./ui"

const TIPS = [
  "Mapbox 填以 pk. 开头的公开 Token，限制为本站域名，并开启 styles:read、fonts:read；不要填写 sk. 私密密钥。",
  "Google 地图填以 AIza 开头的浏览器 Key。它会在浏览器里使用，请在 Google Cloud 里限制为本站域名（HTTP 引荐来源），API 只开启 Maps JavaScript API。",
  "保存后重新打开统计页生效。两种都没开启，或者 Token、Key 加载失败时，显示内置地图。",
]

/** The notes for both map services, at the bottom of the visitor map settings. */
export function MapServiceTips() {
  const { t } = useT()
  return <Tile icon="key" title="使用提示">
    <ol className="map-service-tips">{TIPS.map((tip, i) => <li key={i}><span className="squircle map-service-tip-number">{i + 1}</span><span>{t(tip)}</span></li>)}</ol>
  </Tile>
}
