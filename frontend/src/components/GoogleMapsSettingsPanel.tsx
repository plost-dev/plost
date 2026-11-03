import { useState } from "react"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { saveGoogleMapsSettingsFn, type GoogleMapsSettings } from "~/lib/google-maps-actions"
import { ActionButton } from "./ActionButton"
import { Field, FormActions, SettingsForm } from "./SettingsForm"
import { Tile } from "./ui"
import { Toggle } from "./Toggle"
import { PasswordInput } from "~/components/PasswordInput"

/** Google Maps as the visitor map instead of Mapbox; only one of the two can be on. */
export function GoogleMapsSettingsPanel({initial, onSaved}: {initial: GoogleMapsSettings; onSaved: () => void}) {
  const {t} = useT()
  const [settings, setSettings] = useState(initial)
  const [enabled, setEnabled] = useState(initial.enabled)
  const [key, setKey] = useState("")
  return <Tile brand="google-maps" title="Google 地图"
    titleAddon={<a className="settings-heading-link" href="https://console.cloud.google.com/google/maps-apis/credentials" target="_blank" rel="noreferrer">{t("打开 Google Cloud 获取 API Key")}</a>}
    actions={<Toggle checked={enabled} onChange={setEnabled} hideLabel title={t("使用 Google 地图作为访客地图")}>{t("使用 Google 地图作为访客地图")}</Toggle>}>
    <SettingsForm onSubmit={async () => {
      const next = await saveGoogleMapsSettingsFn({data: {enabled, ...(key.trim() ? {key: key.trim()} : {})}})
      setSettings(next); setKey(""); showToast(next.enabled ? "已改用 Google 地图，重新打开统计页后生效。" : "地图设置已保存，重新打开统计页后生效。")
      onSaved()
    }}>
      <Field label="Google Maps API Key" hint={settings.configured ? t("已保存 Key，留空保留；填写新值可替换。") : t("填写以 AIza 开头的浏览器 Key。")}>
        <PasswordInput className="squircle input" aria-label="Google Maps API Key" autoComplete="off" spellCheck={false} required={enabled && !settings.configured} value={key} placeholder={settings.configured ? "••••••••" : "AIza…"} onChange={event => setKey(event.target.value)}/>
      </Field>
      <p className="mt-3 text-xs leading-6 text-gray-500">{t("Key 会在浏览器里使用。请在 Google Cloud 里把它限制为本站域名（HTTP 引荐来源），API 只开启 Maps JavaScript API。开启 Google 地图会关闭 Mapbox，反之亦然。")}</p>
      <FormActions><ActionButton type="submit" className="btn btn-primary">{t("保存设置")}</ActionButton></FormActions>
    </SettingsForm>
  </Tile>
}
