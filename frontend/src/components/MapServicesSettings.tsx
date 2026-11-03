import { useEffect, useState } from "react"
import { mapboxSettingsFn, saveMapboxSettingsFn, type MapboxSettings } from "~/lib/mapbox-actions"
import { googleMapsSettingsFn, saveGoogleMapsSettingsFn, type GoogleMapsSettings } from "~/lib/google-maps-actions"
import { showToast } from "~/lib/toast"
import { GoogleMapsSettingsPanel } from "./GoogleMapsSettingsPanel"
import { MapboxSettingsPanel } from "./MapboxSettingsPanel"
import { MapServiceTips } from "./MapServiceTips"

type MapServices = { mapbox: MapboxSettings; google: GoogleMapsSettings }
let loading: Promise<MapServices> | null = null

/**
 * Both services' settings, fetched once and shared, so the settings page can start this before the map page is
 * opened and switching to it does not wait for the server. A save clears it for the next visit.
 */
export function loadMapServices() {
  loading ??= Promise.all([mapboxSettingsFn(), googleMapsSettingsFn()]).then(([mapbox, google]) => ({ mapbox, google }))
  loading.catch(() => { loading = null })
  return loading
}

/** The map settings page: shows the services once their settings arrive. */
export function MapServicesPage({ onSaved }: { onSaved: () => void }) {
  const [data, setData] = useState<MapServices | null>(null)
  useEffect(() => {
    let live = true
    loadMapServices().then(next => { if (live) setData(next) }, error => showToast(error instanceof Error ? error.message : "读取失败", "error"))
    return () => { live = false }
  }, [])
  if (!data) return null
  return <MapServicesSettings mapbox={data.mapbox} google={data.google} onSaved={() => { loading = null; onSaved() }} />
}

/**
 * The visitor map services. Both may be off, and then the built-in map is used; at most one is on, and it replaces the
 * built-in map. Each switch saves at once, and turning one on turns the other off.
 */
function MapServicesSettings({ mapbox: initialMapbox, google: initialGoogle, onSaved }: { mapbox: MapboxSettings; google: GoogleMapsSettings; onSaved: () => void }) {
  const [mapbox, setMapbox] = useState(initialMapbox)
  const [google, setGoogle] = useState(initialGoogle)
  // A Mapbox switch saved on before any token was entered does nothing, so it shows as off.
  const [mapboxOn, setMapboxOn] = useState(initialMapbox.enabled && initialMapbox.configured)
  const [googleOn, setGoogleOn] = useState(initialGoogle.enabled && initialGoogle.configured)
  const fail = (error: unknown) => { showToast(error instanceof Error ? error.message : "保存失败", "error"); return false }

  async function toggleMapbox(next: boolean) {
    if (next && !mapbox.configured) return fail(new Error("请先填写并保存 Mapbox Token"))
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
    if (next && !google.configured) return fail(new Error("请先填写并保存 Google Maps API Key"))
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
    <MapboxSettingsPanel settings={mapbox} enabled={mapboxOn} onToggle={toggleMapbox} onKeySaved={next => { setMapbox(next); onSaved() }} />
    <GoogleMapsSettingsPanel settings={google} enabled={googleOn} onToggle={toggleGoogle} onKeySaved={next => { setGoogle(next); onSaved() }} />
    <MapServiceTips />
  </>
}
