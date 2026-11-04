import { ManagementHeader } from "~/components/SettingsChrome"
import { createFileRoute, redirect, useRouter } from "@tanstack/react-router"
import { useState } from "react"
import { ActivityLog } from "~/components/ActivityLog"
import { ActionButton, ActionLink } from "~/components/ActionButton"
import { PageState } from "~/components/PageState"
import { Shell } from "~/components/Shell"
import { SiteSwitcher } from "~/components/SiteSwitcher"
import { activityFn, meFn } from "~/lib/actions"
import { useT, intlLocale } from "~/lib/i18n"
import { useRouterAutoRefresh } from "~/lib/router-refresh"

type Window = "30m" | "24h" | "7d"
export const Route = createFileRoute("/sites/$domain/activity")({
  validateSearch: (search: Record<string,unknown>): {window:Window} => ({window: ["30m","24h","7d"].includes(String(search.window)) ? search.window as Window : "24h"}),
  loaderDeps: ({search})=>({window:search.window}),
  loader: async ({params,deps})=>{
    const me=await meFn()
    if(!me) throw redirect({to:"/login"})
    return {me, data:await activityFn({data:{domain:params.domain,window:deps.window}})}
  },
  component: ActivityPage,
  errorComponent:({error})=><PageState title="页面出错" description={error instanceof Error ? error.message : undefined}/>,
})
function ActivityPage() {
  const {t,locale}=useT()
  const {me,data}=Route.useLoaderData()
  const {domain}=Route.useParams()
  const {window:range}=Route.useSearch()
  const router=useRouter()
  const [auto,setAuto]=useState(true)
  const [busy,setBusy]=useState(false)
  const [error,setError]=useState("")
  useRouterAutoRefresh({enabled: auto, onError: () => setError(t("刷新失败，请重试"))})
  return <Shell user={me}>
    <ManagementHeader title={t("活动日志")} icon="activity"><ActionLink to="/sites/$domain" params={{domain}}>{t("返回统计")}</ActionLink></ManagementHeader>
    <div className="squircle activity-page-toolbar"><SiteSwitcher domain={domain}/><div>
      <label className="activity-window">{t("时间范围")}<select className="squircle input" value={range} onChange={event=>void router.navigate({to:"/sites/$domain/activity",params:{domain},search:{window:event.target.value as Window}})}><option value="30m">{t("近 30 分钟")}</option><option value="24h">{t("近 24 小时")}</option><option value="7d">{t("近 7 天")}</option></select></label>
      <label className="activity-auto"><input type="checkbox" checked={auto} onChange={event=>setAuto(event.target.checked)}/>{t("每 30 秒刷新")}</label>
      <ActionButton  disabled={busy} onClick={async()=>{setBusy(true);setError("");try{await router.invalidate()}catch{setError(t("刷新失败，请重试"))}finally{setBusy(false)}}}>{t(busy ? "刷新中…" : "刷新")}</ActionButton>
    </div></div>
    <div className="activity-update"><span className={`activity-live-dot ${auto ? "is-live" : ""}`}/>{t(auto ? "自动刷新已开启" : "自动刷新已暂停")}<span>{t("更新于")} {new Intl.DateTimeFormat(intlLocale(locale),{timeZone:data.timezone,hour:"2-digit",minute:"2-digit",second:"2-digit",hourCycle:"h23"}).format(new Date(data.fetchedAt))} · {data.timezone}</span></div>
    {error ? <p role="alert" className="squircle settings-feedback is-error">{error}</p> : null}
    <ActivityLog key={`${domain}-${range}`} rows={data.rows} timezone={data.timezone} hasMore={data.hasMore}/>
  </Shell>
}
