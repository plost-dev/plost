import { useT } from "~/lib/i18n"
import { type GoogleMapsSettings } from "~/lib/google-maps-actions"
import { Tile } from "./ui"
import { Toggle } from "./Toggle"
import { GoogleMapStylePicker } from "./MapStylePicker"
import { EnvKeyStatus } from "./EnvKeyStatus"

/** The Google Maps card, with the account's Google map style below the key. Its switch belongs to MapServicesSettings, which keeps it and Mapbox from both being on. */
export function GoogleMapsSettingsPanel({initial, enabled, onToggle}: {
  initial: GoogleMapsSettings; enabled: boolean; onToggle: (next: boolean) => Promise<boolean>
}) {
  const {t} = useT()
  return <Tile brand="google-maps" title="Google 地图"
    titleAddon={<a className="settings-heading-link" href="https://console.cloud.google.com/google/maps-apis/credentials" target="_blank" rel="noreferrer">{t("打开 Google Cloud 获取 API Key")}</a>}
    actions={<Toggle checked={enabled} onChange={next => void onToggle(next)} hideLabel title={t("使用 Google 地图作为访客地图")}>{t("使用 Google 地图作为访客地图")}</Toggle>}>
    <EnvKeyStatus label="Google Maps API Key" name="PLOST_GOOGLE_MAPS_KEY" configured={initial.configured}/>
    <GoogleMapStylePicker />
  </Tile>
}
