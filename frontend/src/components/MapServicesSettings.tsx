import { useState } from "react"
import { saveMapboxSettingsFn, type MapboxSettings } from "~/lib/mapbox-actions"
import { saveGoogleMapsSettingsFn, type GoogleMapsSettings } from "~/lib/google-maps-actions"
import { showToast } from "~/lib/toast"
import { GoogleMapsSettingsPanel } from "./GoogleMapsSettingsPanel"
import { MapboxSettingsPanel } from "./MapboxSettingsPanel"
import { MapServiceTips } from "./MapServiceTips"

/**
 * The visitor map services. Both may be off, and then the built-in map is used; at most one is on, and it replaces the
 * built-in map. Each switch saves at once, and turning one on turns the other off.
 */
export function MapServicesSettings({ mapbox, google, onSaved }: { mapbox: MapboxSettings; google: GoogleMapsSettings; onSaved: () => void }) {
  // A Mapbox switch saved on before any token was entered does nothing, so it shows as off.
  const [mapboxOn, setMapboxOn] = useState(mapbox.enabled && mapbox.configured)
  const [googleOn, setGoogleOn] = useState(google.enabled && google.configured)
  const fail = (error: unknown) => { showToast(error instanceof Error ? error.message : "保存失败", "error"); return false }

  async function toggleMapbox(next: boolean) {
    if (next && !mapbox.configured) return fail(new Error("请先在 env 文件里设置 PLOST_MAPBOX_TOKEN 并重启服务"))
    try {
      await saveMapboxSettingsFn({ data: { enabled: next } })
      setMapboxOn(next)
      if (next) setGoogleOn(false)
      showToast(next ? "已改用 Mapbox，重新打开统计页后生效。" : googleOn ? "已关闭 Mapbox。" : "已关闭 Mapbox，统计页使用内置地图。")
      onSaved()
      return true
    } catch (error) { return fail(error) }
  }
  async function toggleGoogle(next: boolean) {
    if (next && !google.configured) return fail(new Error("请先在 env 文件里设置 PLOST_GOOGLE_MAPS_KEY 并重启服务"))
    try {
      await saveGoogleMapsSettingsFn({ data: { enabled: next } })
      setGoogleOn(next)
      if (next) setMapboxOn(false)
      showToast(next ? "已改用 Google 地图，重新打开统计页后生效。" : mapboxOn ? "已关闭 Google 地图。" : "已关闭 Google 地图，统计页使用内置地图。")
      onSaved()
      return true
    } catch (error) { return fail(error) }
  }
  return <>
    <MapboxSettingsPanel initial={mapbox} enabled={mapboxOn} onToggle={toggleMapbox} />
    <GoogleMapsSettingsPanel initial={google} enabled={googleOn} onToggle={toggleGoogle} />
    <MapServiceTips />
  </>
}
