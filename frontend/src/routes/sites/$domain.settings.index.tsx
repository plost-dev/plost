import { ActionButton, ActionUpload } from "~/components/ActionButton"
import { SiteFavicon } from "~/components/SiteFavicon"
import { SettingsForm, Field, FormActions } from "~/components/SettingsForm"
import { createFileRoute, getRouteApi, useRouter } from "@tanstack/react-router"
import { useMemo, useState } from "react"
import { Toggle } from "~/components/Toggle"
import { Tile } from "~/components/ui"
import { SiteInfoPanel } from "~/components/SiteHealthPanel"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { timezoneOptions } from "~/lib/timezones"
import { cleanDomainInput } from "~/lib/domain-input"
import { removeSiteIconFn, saveSettingsFn, saveSiteCheckFn, uploadSiteIconFn } from "~/lib/actions"

const settings = getRouteApi("/sites/$domain/settings")

export const Route = createFileRoute("/sites/$domain/settings/")({ component: GeneralPage })

function GeneralPage() {
  const { data } = settings.useLoaderData()
  const { domain } = settings.useParams()
  const router = useRouter()
  const { t } = useT()
  const [name, setName] = useState(data.site.name || "")
  const [timezone, setTimezone] = useState(data.site.timezone)
  const ZONES = useMemo(() => timezoneOptions(timezone), [timezone])
  const [newDomain, setNewDomain] = useState(domain)
  const [healthEnabled, setHealthEnabled] = useState(data.health?.enabled || false)
  const [healthPath, setHealthPath] = useState(data.health?.path || "/")

  return <>
    <Tile icon="globe" title={t("站点详情")}>
      <SettingsForm onSubmit={async event => {
        event.preventDefault()
        const saved = await saveSettingsFn({data:{domain,name,timezone,newDomain,public:data.site.public}})
        // The checks follow the site, so they are saved under its new address if it was renamed.
        if (data.health) await saveSiteCheckFn({data:{domain:saved.domain,enabled:healthEnabled,path:healthPath || "/"}})
        if (saved.domain !== domain) await router.navigate({to:"/sites/$domain/settings",params:{domain:saved.domain}})
        else await router.invalidate()
        showToast("设置已保存")
      }}>
        <Field label={t("站点名称")}><input className="squircle input" title={t("在站点列表中显示，留空则默认使用域名。")} placeholder={domain} value={name} onChange={event => setName(event.target.value)} /></Field>
        <Field label={t("站点域名")}><input className="squircle input" value={newDomain} onChange={event => setNewDomain(event.target.value)}
          onPaste={event => { event.preventDefault(); setNewDomain(cleanDomainInput(event.clipboardData.getData("text"))) }}
          onBlur={() => setNewDomain(cleanDomainInput(newDomain))} required /></Field>
        <Field label={t("报表时区")}><select className="squircle input" value={timezone} onChange={event => setTimezone(event.target.value)}>
          {ZONES.some(zone => zone.value === timezone) ? null : <option value={timezone}>{timezone}</option>}
          {ZONES.map(zone => <option key={zone.value} value={zone.value}>{t(zone.label)}</option>)}
        </select></Field>
        {data.health ? <div className="settings-field"><span className="settings-label">{t("站点检查")}</span><Toggle checked={healthEnabled} onChange={setHealthEnabled}>{t("开启检查")}</Toggle></div> : null}
        {data.health && healthEnabled ? <Field label={t("检查路径")}><input className="squircle input" required maxLength={2048} value={healthPath} onChange={event=>setHealthPath(event.target.value)} placeholder="/" autoCapitalize="none" autoCorrect="off" spellCheck={false}/></Field> : null}
        <FormActions><ActionButton className="btn btn-primary" type="submit">{t("保存设置")}</ActionButton></FormActions>
      </SettingsForm>
    </Tile>
    {data.health ? <Tile icon="shield" title={t("站点信息")} subtitle={t("域名与 SSL 证书的到期时间")}><SiteInfoPanel report={data.health}/></Tile> : null}
    <SiteIconTile domain={domain} initial={data.iconVersion || 0} />
  </>
}

/** The site's icon: the one favicon.la finds by default, or one uploaded here, which shows everywhere the site does. */
function SiteIconTile({ domain, initial }: { domain: string, initial: number }) {
  const router = useRouter()
  const { t } = useT()
  const [version, setVersion] = useState(initial)
  const [busy, setBusy] = useState(false)
  const done = async (next: number, message: string) => {
    setVersion(next)
    await router.invalidate()
    showToast(message)
  }
  return <Tile icon="globe" title={t("站点图标")} subtitle={t(version ? "正在使用上传的图标。" : "正在使用 favicon.la 自动获取的图标。")}>
    <div className="settings-avatar-row">
      <SiteFavicon domain={domain} version={version} className="squircle settings-site-icon-preview" />
      <div className="settings-avatar-controls flex min-w-0 flex-wrap gap-2">
        <ActionUpload className="btn-secondary" accept="image/png,image/jpeg,image/gif,image/webp,image/x-icon,.ico" disabled={busy} onChange={async event => {
          const file = event.target.files?.[0]
          event.target.value = ""
          if (!file) return
          setBusy(true)
          try { await done((await uploadSiteIconFn(domain, file)).iconVersion, "站点图标已更新") }
          catch (error) { showToast(error instanceof Error ? error.message : "上传失败", "error") }
          finally { setBusy(false) }
        }}>{t("上传图标")}</ActionUpload>
        {version ? <ActionButton type="button" className="btn btn-secondary" disabled={busy} onClick={async () => {
          setBusy(true)
          try { await removeSiteIconFn(domain); await done(0, "已恢复默认图标") }
          finally { setBusy(false) }
        }}>{t("恢复默认")}</ActionButton> : null}
      </div>
    </div>
  </Tile>
}
