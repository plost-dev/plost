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

  async function toggleMapbox(next: boolean, token: string) {
    if (next && !mapbox.configured && !token) return fail(new Error("请先填写 Mapbox Token"))
    try {
      await saveMapboxSettingsFn({ data: { enabled: next, ...(token ? { token } : {}) } })
      setMapboxOn(next)
      if (next) setGoogleOn(false)
      showToast(next ? "已改用 Mapbox，重新打开统计页后生效。" : googleOn ? "已关闭 Mapbox。" : "已关闭 Mapbox，统计页使用内置地图。")
      onSaved()
      return true
    } catch (error) { return fail(error) }
  }
  async function toggleGoogle(next: boolean, key: string) {
    if (next && !google.configured && !key) return fail(new Error("请先填写 Google 地图 API Key"))
    try {
      await saveGoogleMapsSettingsFn({ data: { enabled: next, ...(key ? { key } : {}) } })
      setGoogleOn(next)
      if (next) setMapboxOn(false)
      showToast(next ? "已改用 Google 地图，重新打开统计页后生效。" : mapboxOn ? "已关闭 Google 地图。" : "已关闭 Google 地图，统计页使用内置地图。")
      onSaved()
      return true
    } catch (error) { return fail(error) }
  }
  return <>
    <MapboxSettingsPanel initial={mapbox} enabled={mapboxOn} onToggle={toggleMapbox} onSaved={onSaved} />
    <GoogleMapsSettingsPanel initial={google} enabled={googleOn} previewKey={googleOn ? google.previewKey : undefined} onToggle={toggleGoogle} onSaved={onSaved} />
    <MapServiceTips />
  </>
}
