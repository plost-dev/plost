import { Surface } from "~/components/Surface"
import { SettingsTips } from "~/components/SettingsTips"
import { TrackingDocs } from "~/components/TrackingDocs"
import { TrackingSnippet } from "~/components/TrackingSnippet"
import { ActionButton, ActionLink } from "~/components/ActionButton"
import { SettingsLayout, SettingsNav } from "~/components/SettingsLayout"
import { SettingsForm, Field, FormActions, ConfirmModal } from "~/components/SettingsForm"
import { Link, createFileRoute, redirect, useRouter } from "@tanstack/react-router"
import { useMemo, useEffect, useState } from "react"
import { SettingsHeader } from "~/components/SettingsChrome"
import { Toggle } from "~/components/Toggle"
import { Shell } from "~/components/Shell"
import { Tile } from "~/components/ui"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
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

type Tab = typeof TABS[number]["id"]
// The general page lives at /settings itself, the others at /settings/<page>.
const tabParam = (tab: Tab) => tab === "general" ? undefined : tab

export const Route = createFileRoute("/sites/$domain/settings/{-$tab}")({
  // Older links named the page as ?tab=…; they, and unknown pages, are sent on to the page's own address.
  validateSearch: (s: Record<string, unknown>): { tab?: string } => typeof s.tab === "string" ? { tab: s.tab } : {},
  beforeLoad: ({ params, search }) => {
    const tab = TABS.find(item => item.id === (search.tab ?? params.tab))?.id ?? "general"
    if (search.tab !== undefined || params.tab !== tabParam(tab)) {
      throw redirect({ to: "/sites/$domain/settings/{-$tab}", params: { domain: params.domain, tab: tabParam(tab) }, search: {}, replace: true })
    }
  },
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
  const { domain, tab: page } = Route.useParams()
  const tab: Tab = TABS.find(item => item.id === page)?.id ?? "general"
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
          {TABS.map(item => <Link key={item.id} to="/sites/$domain/settings/{-$tab}" params={{domain, tab: tabParam(item.id)}} aria-current={tab === item.id ? "page" : undefined} className={`squircle settings-nav-link${tab === item.id ? " is-active" : ""}`}>{t(item.label)}</Link>)}
        </SettingsNav>}>
          {tab === "general" ? <Tile icon="globe" title={t("站点详情")}>
            <SettingsForm onSubmit={async event => {
              event.preventDefault()
              const saved = await saveSettingsFn({data:{domain,name,timezone,newDomain,public:data.site.public}})
              // The checks follow the site, so they are saved under its new address if it was renamed.
              if (data.health) await saveSiteCheckFn({data:{domain:saved.domain,enabled:healthEnabled,path:healthPath || "/"}})
              if (saved.domain !== domain) await router.navigate({to:"/sites/$domain/settings/{-$tab}",params:{domain:saved.domain}})
              else await router.invalidate()
              showToast("设置已保存")
            }}>
              <Field label={t("站点名称")}><input className="squircle input" title={t("在站点列表中显示，留空则默认使用域名。")} placeholder={domain} value={name} onChange={event => setName(event.target.value)} /></Field>
              <Field label={t("站点域名")}><input className="squircle input" value={newDomain} onChange={event => setNewDomain(event.target.value)} required /></Field>
              <Field label={t("报表时区")}><select className="squircle input" value={timezone} onChange={event => setTimezone(event.target.value)}>
                {ZONES.some(zone => zone.value === timezone) ? null : <option value={timezone}>{timezone}</option>}
                {ZONES.map(zone => <option key={zone.value} value={zone.value}>{t(zone.label)}</option>)}
              </select></Field>
              {data.health ? <div className="settings-field"><span className="settings-label">{t("站点检查")}</span><Toggle checked={healthEnabled} onChange={setHealthEnabled}>{t("开启检查")}</Toggle></div> : null}
              {data.health && healthEnabled ? <Field label={t("检查路径")}><input className="squircle input" required maxLength={2048} value={healthPath} onChange={event=>setHealthPath(event.target.value)} placeholder="/" autoCapitalize="none" autoCorrect="off" spellCheck={false}/></Field> : null}
              <FormActions><ActionButton className="btn btn-primary" type="submit">{t("保存设置")}</ActionButton></FormActions>
            </SettingsForm>
          </Tile> : null}
          {tab === "general" ? <SettingsTips tips={[
            "站点域名改动后，埋点脚本里的 data-domain 也要一起改成新域名，否则新的访问不会记到这个站点。",
            "时区决定「今天」「昨天」从几点开始算，也决定按天、按小时统计的分界。",
            "站点检查每 5 分钟访问一次检查路径，跟随跳转后返回 HTTP 200 视为正常，并检查证书和域名到期时间。无需安装探针或统计脚本。",
          ]} /> : null}

          {tab === "visibility" ? (
            <div>
              <Tile icon="eye" title={t("公开仪表盘")}>
                <SettingsForm onSubmit={async event => {
                  event.preventDefault()
                  await saveSettingsFn({data:{domain,public:isPublic}})
                  await router.invalidate()
                  showToast("设置已保存")
                }}>
                  <Toggle checked={isPublic} onChange={setIsPublic}>{t("公开这个站点的仪表盘")}</Toggle>
                  <FormActions><ActionButton className="btn btn-primary" type="submit">{t("保存设置")}</ActionButton></FormActions>
                </SettingsForm>
              </Tile>
              <Tile icon="plug" title={t("共享链接")}>
                <p className="mb-4 text-sm"><Link to="/account" search={{ tab: "api" }} className="text-indigo-600 hover:underline">{t("API")} · {t("在线人数")} / UV / PV</Link></p>
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
              <SettingsTips tips={[
                "公开仪表盘打开后，任何人都能用站点地址查看这个站点的统计，不需要登录。",
                "共享链接生成不需登录就能打开的只读报表，可以随时删除；删除后链接立即失效。",
                "两者都只开放报表，不包括活动日志、账号、系统设置和备份。",
              ]} />
            </div>
          ) : null}

          {tab === "installation" ? (
            <div>
              <Tile icon="code" title={t("埋点脚本")}>
                <TrackingSnippet code={snippet} />
                <TrackingDocs domain={domain} />
                <p className="mt-4 text-sm"><Link to="/account" search={{ tab: "api" }} className="text-indigo-600 hover:underline">{t("API")} · {t("在线人数")} / UV / PV</Link></p>
              </Tile>
              <SettingsTips tips={[
                "把这段代码放到网站的 <head> 里。默认会记录页面浏览、外链点击、文件下载，以及标题、语言和屏幕分辨率。",
                "页面完全加载完才计一次浏览，单页应用切换路由、后退前进也会计入；自动化浏览器和常见爬虫不计。",
                "不论直连，还是经过反向代理、Cloudflare 或其他 CDN，访客的 IP 和国家都会自动识别，不用额外设置。",
              ]} />
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
