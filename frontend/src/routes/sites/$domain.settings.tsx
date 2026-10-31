import { Surface } from "~/components/Surface"
import { TrackingDocs } from "~/components/TrackingDocs"
import { TrackingSnippet } from "~/components/TrackingSnippet"
import { ActionButton, ActionLink } from "~/components/ActionButton"
import { SettingsLayout, SettingsNav } from "~/components/SettingsLayout"
import { SettingsForm, Field, FormActions, ConfirmModal } from "~/components/SettingsForm"
import { Link, createFileRoute, redirect, useRouter } from "@tanstack/react-router"
import { useMemo, useEffect, useState } from "react"
import { SettingsHeader } from "~/components/SettingsChrome"
import { Shell } from "~/components/Shell"
import { Tile } from "~/components/ui"
import { useT } from "~/lib/i18n"
import { timezoneOptions } from "~/lib/timezones"
import {
  createShareFn,
  meFn,
  removeShareFn,
  removeSiteFn,
  saveSettingsFn,
  saveSiteCheckFn,
  siteSettingsFn,
} from "~/lib/actions"

const TABS = [
  { id: "general", label: "常规" },
  { id: "visibility", label: "可见性" },
  { id: "installation", label: "安装脚本" },
  { id: "danger-zone", label: "危险操作" },
] as const

export const Route = createFileRoute("/sites/$domain/settings")({
  validateSearch: (s: Record<string, unknown>) => ({tab: TABS.find(tab => tab.id === s.tab)?.id || "general"}),
  loader: async ({ params }) => {
    const me = await meFn()
    if (!me) throw redirect({ to: "/login" })
    const data = await siteSettingsFn({ data: { domain: params.domain } })
    return { me, data }
  },
  component: SettingsPage,
})

function SettingsPage() {
  const { me, data } = Route.useLoaderData()
  const { domain } = Route.useParams()
  const { tab } = Route.useSearch()
  const router = useRouter()
  const { t } = useT()
  const [name, setName] = useState(data.site.name || "")
  const [timezone, setTimezone] = useState(data.site.timezone)
  const ZONES = useMemo(() => timezoneOptions(timezone), [timezone])
  const [newDomain, setNewDomain] = useState(domain)
  const [isPublic, setIsPublic] = useState(!!data.site.public)
  const [healthEnabled, setHealthEnabled] = useState(data.health?.enabled || false)
  const [healthPath, setHealthPath] = useState(data.health?.path || "/")
  const [shareName, setShareName] = useState(t("公开报表"))
  const [ok, setOk] = useState("")
  useEffect(() => { setOk("") }, [tab])

  const [origin, setOrigin] = useState("")
  useEffect(() => { setOrigin(window.location.origin) }, [])
  const [remove, setRemove] = useState(false)
  const [confirmation, setConfirmation] = useState("")
  const snippet = `<script defer data-domain="${domain}" src="${origin}/script.js"></script>`

  return (
    <Shell user={me}>
      <Surface radius="large" className="site-settings-workspace">
        <SettingsHeader
          title={t("settings.site_for")}
          domain={domain}
          name={name || data.site.name || ""}
          back={(
            <ActionLink to="/sites/$domain" params={{ domain }} className="btn-secondary settings-back">
              {t("settings.back_stats")}
            </ActionLink>
          )}
        />
        <SettingsLayout navigationTitle={t(TABS.find(item => item.id === tab)?.label || "设置")} sidebar={<SettingsNav label={t("站点设置")}>
          {TABS.map(item => <Link key={item.id} to="/sites/$domain/settings" params={{domain}} search={{tab:item.id}} aria-current={tab === item.id ? "page" : undefined} className={`squircle settings-nav-link${tab === item.id ? " is-active" : ""}`}>{t(item.label)}</Link>)}
        </SettingsNav>}>
          {ok ? <p role="status" className="text-sm text-indigo-600">{t(ok || "")}</p> : null}

          {tab === "general" ? <Tile icon="globe" title={t("站点详情")}>
            <SettingsForm onSubmit={async event => {
              event.preventDefault()
              const saved = await saveSettingsFn({data:{domain,name,timezone,newDomain,public:data.site.public}})
              // The checks follow the site, so they are saved under its new address if it was renamed.
              if (data.health) await saveSiteCheckFn({data:{domain:saved.domain,enabled:healthEnabled,path:healthPath || "/"}})
              if (saved.domain !== domain) await router.navigate({to:"/sites/$domain/settings",params:{domain:saved.domain},search:{tab:"general"}})
              else await router.invalidate()
              setOk(t("设置已保存"))
            }}>
              <Field label={t("站点名称")}><input className="squircle input" title={t("在站点列表中显示，留空则默认使用域名。")} placeholder={domain} value={name} onChange={event => setName(event.target.value)} /></Field>
              <Field label={t("站点域名")}><input className="squircle input" value={newDomain} onChange={event => setNewDomain(event.target.value)} required /></Field>
              <Field label={t("报表时区")}><select className="squircle input" value={timezone} onChange={event => setTimezone(event.target.value)}>
                {ZONES.some(zone => zone.value === timezone) ? null : <option value={timezone}>{timezone}</option>}
                {ZONES.map(zone => <option key={zone.value} value={zone.value}>{t(zone.label)}</option>)}
              </select></Field>
              {data.health ? <Field label={t("站点检查")}><span className="flex items-center gap-3 text-sm text-gray-800"><input type="checkbox" checked={healthEnabled} onChange={event=>setHealthEnabled(event.target.checked)} />{t("开启检查")}</span></Field> : null}
              {data.health && healthEnabled ? <Field label={t("检查路径")} hint={t("每 5 分钟访问一次，跟随跳转后返回 HTTP 200 视为正常。无需安装探针或统计脚本。")}><input className="squircle input" required maxLength={2048} value={healthPath} onChange={event=>setHealthPath(event.target.value)} placeholder="/" autoCapitalize="none" autoCorrect="off" spellCheck={false}/></Field> : null}
              <FormActions><ActionButton className="btn btn-primary" type="submit">{t("保存设置")}</ActionButton></FormActions>
            </SettingsForm>
          </Tile> : null}

          {tab === "visibility" ? (
            <div>
              <Tile icon="eye" title={t("公开仪表盘")}>
                <SettingsForm onSubmit={async event => {
                  event.preventDefault()
                  await saveSettingsFn({data:{domain,public:isPublic}})
                  await router.invalidate()
                  setOk(t("设置已保存"))
                }}>
                  <label className="flex items-center gap-3 text-sm text-gray-800"><input type="checkbox" checked={isPublic} onChange={event => setIsPublic(event.target.checked)} />{t("公开这个站点的仪表盘")}</label>
                  <FormActions><ActionButton className="btn btn-primary" type="submit">{t("保存设置")}</ActionButton></FormActions>
                </SettingsForm>
              </Tile>
              <Tile icon="plug" title={t("共享链接")} subtitle={t("生成不需登录就能打开的报表链接。")}>
                <p className="mb-4 text-sm"><Link to="/docs/api" className="text-indigo-600 hover:underline">{t("API")} · {t("在线人数")} / UV / PV</Link></p>
                <SettingsForm
                  className="mb-4"
                  onSubmit={async event => {
                    event.preventDefault()
                    await createShareFn({ data: { domain, name: shareName } })
                    setShareName(t("公开报表"))
                    await router.invalidate()
                  }}
                >
                  <Field label={t("共享链接名称")}><input className="squircle input" value={shareName} onChange={event => setShareName(event.target.value)} /></Field>
                  <FormActions><ActionButton className="btn btn-primary" type="submit">{t("创建")}</ActionButton></FormActions>
                </SettingsForm>
                <ul className="settings-share-list space-y-2 text-sm">
                  {data.shares.map(share => (
                    <li key={share.id} className="settings-list-row flex min-w-0 flex-wrap justify-between gap-3">
                      {share.slug
                        ? <a className="min-w-0 flex-1 text-indigo-600 hover:text-indigo-500" href={`/share/${share.slug}`}><span className="min-w-0 [overflow-wrap:anywhere]">{share.name} · /share/{share.slug}</span></a>
                        : <span className="min-w-0 flex-1">{share.name}</span>}
                      <ActionButton className="shrink-0 text-red-600" type="button" onClick={async () => { await removeShareFn({ data: { domain, id: share.id } }); await router.invalidate() }}>{t("删除")}</ActionButton>
                    </li>
                  ))}
                </ul>
              </Tile>
            </div>
          ) : null}

          {tab === "installation" ? (
            <div>
              <Tile icon="code" title={t("埋点脚本")} subtitle={t("把这段代码放到网站的 <head> 里。默认会记录页面浏览、外链点击、文件下载，以及标题、语言和屏幕分辨率。")}>
                <TrackingSnippet code={snippet} />
                <TrackingDocs domain={domain} />
                <p className="mt-4 text-sm"><Link to="/docs/api" className="text-indigo-600 hover:underline">{t("API")} · {t("在线人数")} / UV / PV</Link></p>
              </Tile>
            </div>
          ) : null}

          {tab === "danger-zone" ? (
            <div>
              <div className="squircle settings-danger-notice">
                <h3 className="text-sm font-medium text-gray-900">{t("危险操作")}</h3>
                <p className="mt-1 text-sm text-gray-600">{t("下面的操作会造成无法恢复的数据丢失，请谨慎。")}</p>
              </div>
              <Tile icon="warn" title={t("删除站点")} subtitle={t("删除后会移除此站点及访问历史，无法撤销。")}>
                <ActionButton
                  className="btn btn-danger"
                  type="button"
                  onClick={() => { setConfirmation(""); setRemove(true) }}
                >{t("删除站点")}</ActionButton>
              </Tile>
            </div>
          ) : null}
        </SettingsLayout>
        <ConfirmModal
          open={remove}
          title={t("删除站点")}
          confirmLabel={t("确认删除")}
          danger
          onClose={() => setRemove(false)}
          onSubmit={async () => {
            if (confirmation !== domain) throw Error(t("请输入完整域名以确认删除"))
            await removeSiteFn({ data: { domain } })
            await router.navigate({ to: "/dashboard" })
          }}
        >
          <Field label={t("请输入完整域名以确认删除")}><input className="squircle input" value={confirmation} onChange={event => setConfirmation(event.target.value)} required /></Field>
        </ConfirmModal>
      </Surface>
    </Shell>
  )
}
