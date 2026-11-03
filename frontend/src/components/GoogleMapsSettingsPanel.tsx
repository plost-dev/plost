import { useT } from "~/lib/i18n"
import { saveGoogleMapsSettingsFn, type GoogleMapsSettings } from "~/lib/google-maps-actions"
import { Tile } from "./ui"
import { Toggle } from "./Toggle"
import { GoogleMapStylePicker } from "./MapStylePicker"
import { MapKeyForm } from "./MapKeyForm"

/** The Google Maps card, with the account's Google map style below the key. Its switch belongs to MapServicesSettings, which keeps it and Mapbox from both being on. */
export function GoogleMapsSettingsPanel({settings, enabled, onToggle, onKeySaved}: {
  settings: GoogleMapsSettings; enabled: boolean; onToggle: (next: boolean) => Promise<boolean>; onKeySaved: (next: GoogleMapsSettings) => void
}) {
  const {t} = useT()
  return <Tile brand="google-maps" title="Google 地图"
    titleAddon={<a className="settings-heading-link" href="https://console.cloud.google.com/google/maps-apis/credentials" target="_blank" rel="noreferrer">{t("打开 Google Cloud 获取 API Key")}</a>}
    actions={<Toggle checked={enabled} onChange={next => void onToggle(next)} hideLabel title={t("使用 Google 地图作为访客地图")}>{t("使用 Google 地图作为访客地图")}</Toggle>}>
    <MapKeyForm label="Google Maps API Key" name="PLOST_GOOGLE_MAPS_KEY" example="AIza…" masked={settings.masked} editable={settings.editable}
      save={async key => onKeySaved(await saveGoogleMapsSettingsFn({data: {key}}))}/>
    <GoogleMapStylePicker />
  </Tile>
}
