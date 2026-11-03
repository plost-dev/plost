import { useState } from "react"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { saveMapboxSettingsFn } from "~/lib/mapbox-actions"
import { ActionButton } from "./ActionButton"
import { Field, FormActions, SettingsForm } from "./SettingsForm"
import { Tile } from "./ui"
import { Toggle } from "./Toggle"
import { PasswordInput } from "~/components/PasswordInput"

export function MapboxSettingsPanel({initial, onSaved}: {initial: {enabled: boolean; configured: boolean}; onSaved?: () => void}) {
  const {t} = useT()
  const [settings, setSettings] = useState(initial)
  const [enabled, setEnabled] = useState(initial.enabled)
  const [token, setToken] = useState("")
  return <Tile icon="globe" title="Mapbox" logo={{src: "/brands/mapbox.svg", width: 104}}
    titleAddon={<a className="settings-heading-link" href="https://console.mapbox.com/account/access-tokens/" target="_blank" rel="noreferrer">{t("打开 Mapbox 获取 Token")}</a>}
    actions={<Toggle checked={enabled} onChange={setEnabled} hideLabel title={t("使用 Mapbox 访客地图")}>{t("使用 Mapbox 访客地图")}</Toggle>}>
    <SettingsForm onSubmit={async () => {
      const next = await saveMapboxSettingsFn({data: {enabled, ...(token.trim() ? {token: token.trim()} : {})}})
      setSettings(next); setToken(""); showToast("地图设置已保存，重新打开统计页后生效。")
      onSaved?.()
    }}>
      <Field label="Mapbox Access Token" hint={settings.configured ? t("已保存密钥，留空保留；填写新值可替换。") : t("填写以 pk. 开头的公开 Token。") }>
        <PasswordInput className="squircle input" aria-label="Mapbox Access Token" autoComplete="off" spellCheck={false} required={enabled && !settings.configured} value={token} placeholder={settings.configured ? "••••••••" : "pk.…"} onChange={event => setToken(event.target.value)}/>
      </Field>
      <p className="mt-3 text-xs leading-6 text-gray-500">{t("地图会在浏览器使用此公开 Token。请限制为本站域名，并开启 styles:read、fonts:read；不要填写 sk. 私密密钥。")}</p>
      <FormActions><ActionButton type="submit" className="btn btn-primary" >{t("保存设置")}</ActionButton></FormActions>
    </SettingsForm>
  </Tile>
}
