import { SettingsTips } from "./SettingsTips"

/** The notes for both map services, at the bottom of the visitor map settings. */
export function MapServiceTips() {
  return <SettingsTips title="使用提示" tips={[
    "Token 和 Key 保存在服务的 env 文件里，不存数据库，保存后立即生效。请在 Mapbox 和 Google Cloud 里把它们限制为本站域名。",
    "两种地图同时只能开一种，都不开时显示内置地图。地图样式选择后自动保存，统计页重新打开后生效。",
  ]} />
}
