import { useState } from "react"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { saveGoogleMapsSettingsFn, type GoogleMapsSettings } from "~/lib/google-maps-actions"
import { ActionButton } from "./ActionButton"
import { Field, FormActions, SettingsForm } from "./SettingsForm"
import { Tile } from "./ui"
import { Toggle } from "./Toggle"
import { GoogleMapStylePicker } from "./MapStylePicker"
import { PasswordInput } from "~/components/PasswordInput"

/** The Google Maps card, with the account's Google map style below the key. Its switch belongs to MapServicesSettings, which keeps it and Mapbox from both being on. */
export function GoogleMapsSettingsPanel({initial, enabled, onToggle, onSaved}: {
  initial: GoogleMapsSettings; enabled: boolean
  onToggle: (next: boolean, key: string) => Promise<boolean>; onSaved: () => void
}) {
  const {t} = useT()
  const [settings, setSettings] = useState(initial)
  const [key, setKey] = useState("")
  return <Tile brand="google-maps" title="Google 地图"
    titleAddon={<a className="settings-heading-link" href="https://console.cloud.google.com/google/maps-apis/credentials" target="_blank" rel="noreferrer">{t("打开 Google Cloud 获取 API Key")}</a>}
    actions={<Toggle checked={enabled} onChange={next => void onToggle(next, key.trim()).then(ok => { if (ok && key.trim()) { setSettings({...settings, configured: true}); setKey("") } })} hideLabel title={t("使用 Google 地图作为访客地图")}>{t("使用 Google 地图作为访客地图")}</Toggle>}>
    <SettingsForm onSubmit={async () => {
      const next = await saveGoogleMapsSettingsFn({data: {enabled, ...(key.trim() ? {key: key.trim()} : {})}})
      setSettings(next); setKey(""); showToast(next.enabled ? "已改用 Google 地图，重新打开统计页后生效。" : "地图设置已保存，重新打开统计页后生效。")
      onSaved()
    }}>
      <Field label="Google Maps API Key" hint={settings.configured ? t("已保存 Key，留空保留；填写新值可替换。") : undefined}>
        <PasswordInput className="squircle input" aria-label="Google Maps API Key" autoComplete="off" spellCheck={false} required={enabled && !settings.configured} value={key} placeholder={settings.configured ? "••••••••" : "AIza…"} onChange={event => setKey(event.target.value)}/>
      </Field>
      <FormActions><ActionButton type="submit" className="btn btn-primary">{t("保存设置")}</ActionButton></FormActions>
    </SettingsForm>
    <GoogleMapStylePicker />
  </Tile>
}
