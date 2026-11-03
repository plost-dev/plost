import { useT } from "~/lib/i18n"
import { Tile } from "./ui"
import { Toggle } from "./Toggle"
import { MapStylePicker } from "./MapStylePicker"
import { MapKeyForm } from "./MapKeyForm"
import { saveMapboxSettingsFn, type MapboxSettings } from "~/lib/mapbox-actions"

/** The Mapbox card, with the account's Mapbox map style below the token. Its switch belongs to MapServicesSettings, which keeps it and Google Maps from both being on. */
export function MapboxSettingsPanel({settings, enabled, onToggle, onKeySaved}: {
  settings: MapboxSettings; enabled: boolean; onToggle: (next: boolean) => Promise<boolean>; onKeySaved: (next: MapboxSettings) => void
}) {
  const {t} = useT()
  return <Tile icon="globe" title="Mapbox" logo={{src: "/brands/mapbox.svg", width: 104}}
    titleAddon={<a className="settings-heading-link" href="https://console.mapbox.com/account/access-tokens/" target="_blank" rel="noreferrer">{t("打开 Mapbox 获取 Token")}</a>}
    actions={<Toggle checked={enabled} onChange={next => void onToggle(next)} hideLabel title={t("使用 Mapbox 访客地图")}>{t("使用 Mapbox 访客地图")}</Toggle>}>
    <MapKeyForm label="Mapbox Access Token" name="PLOST_MAPBOX_TOKEN" example="pk.…" masked={settings.masked} editable={settings.editable}
      save={async token => onKeySaved(await saveMapboxSettingsFn({data: {token}}))}/>
    <MapStylePicker />
  </Tile>
}
