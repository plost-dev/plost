import { useEffect, useState } from "react"
import { mapboxSettingsFn, saveMapboxSettingsFn, type MapboxSettings } from "~/lib/mapbox-actions"
import { googleMapsSettingsFn, saveGoogleMapsSettingsFn, type GoogleMapsSettings } from "~/lib/google-maps-actions"
import { mapUsageFn, saveMapUsageFn, type MapUsage } from "~/lib/map-usage-actions"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { Toggle } from "./Toggle"
import { Tile } from "./ui"
import { GoogleMapsSettingsPanel } from "./GoogleMapsSettingsPanel"
import { MapboxSettingsPanel } from "./MapboxSettingsPanel"

type MapServices = { mapbox: MapboxSettings; google: GoogleMapsSettings; usage: MapUsage }
let loading: Promise<MapServices> | null = null

/**
 * Both services' settings, fetched once and shared, so the settings page can start this before the map page is
 * opened and switching to it does not wait for the server. A save clears it for the next visit.
 */
export function loadMapServices() {
  loading ??= Promise.all([mapboxSettingsFn(), googleMapsSettingsFn(), mapUsageFn()]).then(([mapbox, google, usage]) => ({ mapbox, google, usage }))
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
  return <MapServicesSettings mapbox={data.mapbox} google={data.google} usage={data.usage} onSaved={() => { loading = null; onSaved() }} />
}

/**
 * The visitor map services. Both may be off, and then the built-in map is used; at most one is on, and it replaces the
 * built-in map. Each switch saves at once, and turning one on turns the other off.
 */
function MapServicesSettings({ mapbox: initialMapbox, google: initialGoogle, usage, onSaved }: { mapbox: MapboxSettings; google: GoogleMapsSettings; usage: MapUsage; onSaved: () => void }) {
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
    <MapUsagePanel initial={usage} service={mapboxOn ? "Mapbox" : googleOn ? "Google 地图" : ""} onSaved={onSaved} />
    <MapOutlinePanel initial={usage.outline} onSaved={onSaved} />
  </>
}

const OUTLINES: Array<{ id: MapUsage["outline"]; label: string }> = [
  { id: "regions", label: "国家和省/州" },
  { id: "countries", label: "仅国家" },
]

/** How detailed the built-in map is: countries with their provinces, or country outlines only for speed. */
function MapOutlinePanel({ initial, onSaved }: { initial: MapUsage["outline"]; onSaved: () => void }) {
  const { t } = useT()
  const [outline, setOutline] = useState(initial)
  const [pending, setPending] = useState(false)
  async function change(next: MapUsage["outline"]) {
    if (next === outline) return
    setPending(true)
    try {
      setOutline((await saveMapUsageFn({ outline: next })).outline)
      showToast("已保存，重新打开统计页后生效。")
      onSaved()
    } catch (error) { showToast(error instanceof Error ? error.message : "保存失败", "error") }
    finally { setPending(false) }
  }
  return <Tile icon="globe" title={t("内置地图轮廓")} subtitle={t("没有使用 Mapbox 或 Google 地图时，统计页的访客分布地图画到多细。")}>
    <div className="squircle segmented-control inline-flex w-fit rounded-md bg-gray-100" role="radiogroup" aria-label={t("内置地图轮廓")}>
      {OUTLINES.map(item => <button key={item.id} type="button" role="radio" aria-checked={item.id === outline} aria-pressed={item.id === outline} disabled={pending}
        className="squircle rounded px-3 py-1 text-sm font-medium text-gray-600 hover:text-gray-900" onClick={() => void change(item.id)}>{t(item.label)}</button>)}
    </div>
  </Tile>
}

/** Where the map service that is on is used. Each switch saves at once; without a service on, both show the built-in map. */
function MapUsagePanel({ initial, service, onSaved }: { initial: MapUsage; service: string; onSaved: () => void }) {
  const { t } = useT()
  const [usage, setUsage] = useState(initial)
  async function change(next: Partial<MapUsage>) {
    try {
      setUsage(await saveMapUsageFn(next))
      showToast("已保存，重新打开页面后生效。")
      onSaved()
    } catch (error) { showToast(error instanceof Error ? error.message : "保存失败", "error") }
  }
  return <Tile icon="map" title={t("使用范围")} subtitle={service ? t("{s} 用在哪些页面；没勾选的地方使用内置地图。").replace("{s}", t(service)) : t("先打开上面的 Mapbox 或 Google 地图，再选择用在哪些页面。")}>
    <div className="flex flex-wrap items-center gap-x-8 gap-y-3">
      <Toggle checked={usage.detail} disabled={!service} onChange={detail => void change({ detail })}>{t("站点详情页的访客地图")}</Toggle>
      <Toggle checked={usage.overview} disabled={!service} onChange={overview => void change({ overview })}>{t("站点首页的地球")}</Toggle>
    </div>
  </Tile>
}
