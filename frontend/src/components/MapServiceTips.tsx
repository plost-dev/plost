import { SettingsTips } from "./SettingsTips"

/** The notes for both map services, at the bottom of the visitor map settings. */
export function MapServiceTips() {
  return <SettingsTips title="使用提示" tips={[
    "Mapbox 填以 pk. 开头的公开 Token，限制为本站域名，并开启 styles:read、fonts:read；不要填写 sk. 私密密钥。",
    "Google 地图填以 AIza 开头的浏览器 Key。它会在浏览器里使用，请在 Google Cloud 里限制为本站域名（HTTP 引荐来源），API 只开启 Maps JavaScript API。",
    "Mapbox 和 Google 地图各有三种地图样式，选择后自动保存到当前账号。「标准」适配界面深浅色，「街道」和「深色」保持自身配色。Google 地图开启后，样式预览用你的 Key 实时显示与 Mapbox 预览同一位置的地图；未开启时只按配色示意。",
    "开关点一下就保存，重新打开统计页后生效。两种都没开启，或者 Token、Key 加载失败时，显示内置地图。",
  ]} />
}
