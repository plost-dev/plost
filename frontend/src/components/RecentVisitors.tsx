import { useEffect, useRef, useState } from "react"
import { reportValueLabel } from "~/lib/report-values"
import { createPortal } from "react-dom"
import { recentVisitorsFn } from "~/lib/actions"
import { countryName } from "~/lib/countries"
import { isDirectSource } from "~/lib/source-icons"
import { useT, intlLocale } from "~/lib/i18n"
import { ActionButton } from "./ActionButton"
import { CountryFlag } from "./icons"
import { UIIcon } from "./UIIcon"
import { ReportValue } from "./ReportValue"
import { HelpHint } from "./HelpHint"

type Result = Awaited<ReturnType<typeof recentVisitorsFn>>
export function RecentVisitors({domain,onClose}:{domain:string;onClose:()=>void}) {
  const {t,locale}=useT()
  const dialog=useRef<HTMLDialogElement>(null)
  const [data,setData]=useState<Result|null>(null)
  const [error,setError]=useState(false)
  const [busy,setBusy]=useState(true)
  const [refresh,setRefresh]=useState(0)
  const [onlyActive,setOnlyActive]=useState(false)
  useEffect(()=>{dialog.current?.showModal(); const previous=document.body.style.overflow; document.body.style.overflow="hidden"; return()=>{document.body.style.overflow=previous}},[])
  useEffect(()=>{
    let disposed=false, pending=false
    async function load(){
      if(pending)return
      pending=true;setBusy(true)
      try {const result=await recentVisitorsFn({data:{domain}}); if(!disposed){setData(result);setError(false)}}
      catch {if(!disposed)setError(true)}
      finally {pending=false;if(!disposed)setBusy(false)}
    }
    void load()
    const timer=setInterval(()=>{if(document.visibilityState==="visible")void load()},30000)
    return()=>{disposed=true;clearInterval(timer)}
  },[domain,refresh])
  const format=(value:string)=>new Intl.DateTimeFormat(intlLocale(locale),{timeZone:data?.timezone || "UTC",hour:"2-digit",minute:"2-digit",second:"2-digit",hourCycle:"h23"}).format(new Date(value))
  const rows=data?.rows.filter(row=>!onlyActive || row.active) || []
  return createPortal(<dialog ref={dialog} className="squircle recent-visitors-dialog" aria-labelledby="recent-visitors-title" onCancel={onClose} onClick={event=>{if(event.target===event.currentTarget)onClose()}}>
    <header><div className="flex min-w-0 items-center gap-2"><h2 id="recent-visitors-title">{t("最近访客")}</h2><HelpHint><p>{t("近 5 分钟有事件上报视为活跃，不代表持续连接在线。")}</p><p>{t("全站最近访客，不受统计页日期和筛选影响；每 30 秒刷新。")}</p><p>{t("匿名标识用于区分访客，不包含姓名或原始 IP。")}</p></HelpHint></div><ActionButton className="ui-icon-button" aria-label={t("关闭")} onClick={onClose}><UIIcon name="close"/></ActionButton></header>
    <div className="recent-visitors-controls"><div className="squircle segmented-control" role="group" aria-label={t("访客范围")}><button className="squircle" aria-pressed={!onlyActive} onClick={()=>setOnlyActive(false)}>{t("近 30 分钟")} <strong>{data?.total ?? "—"}</strong></button><button className="squircle" aria-pressed={onlyActive} onClick={()=>setOnlyActive(true)}>{t("当前活跃")} <strong>{data?.online ?? "—"}</strong></button></div><ActionButton  disabled={busy} onClick={()=>setRefresh(n=>n+1)}>{t(busy ? "刷新中…" : "刷新")}</ActionButton></div>
    {error ? <p role="alert" className="squircle settings-feedback is-error">{t("刷新失败，请重试")}</p> : null}
    <div className="recent-visitors-list">
      {!rows.length ? <div className="activity-empty"><UIIcon name="user"/><strong>{t(busy && !data ? "加载中…" : error && !data ? "暂时无法读取访客" : "这个时间范围内没有访客")}</strong></div> : rows.map(row=><details key={row.id} className="recent-visitor"><summary><span className={`recent-visitor-dot ${row.active ? "is-active" : ""}`}/><div><strong>{t("匿名访客")} · {row.id.slice(-8)}</strong><span>{row.pathname || "/"}</span></div><div className="recent-visitor-device"><strong>{(row.device ? reportValueLabel("device", row.device, locale) : t("未知设备"))}</strong><span>{row.browser} · {row.os}</span></div><time dateTime={row.lastSeen}>{format(row.lastSeen)}</time></summary><dl className="activity-record-details">
      {[[t("状态"),t(row.active ? "近 5 分钟活跃" : "近 30 分钟访问过")],[t("浏览器"),row.browser],[t("操作系统"),row.os],[t("设备"),row.device ? reportValueLabel("device", row.device, locale) : ""],[t("屏幕分辨率"),row.screen],[t("语言"),row.language],[t("来源"),isDirectSource(row.source) ? t("直接访问") : row.source],[t("首次出现（本时间段）"),format(row.firstSeen)],[t("最后活动"),format(row.lastSeen)],[t("页面浏览"),String(row.pageviews)],[t("事件数"),String(row.events)]].map(([label,value])=>{const kind=label===t("语言") ? "language" : label===t("浏览器") ? "browser" : label===t("操作系统") ? "os" : label===t("来源") ? "source" : undefined; return <div key={label}><dt>{label}</dt><dd>{kind ? <ReportValue kind={kind} name={kind==="source" ? row.source : value} wrap/> : value || "—"}</dd></div>})}
      <div><dt>{t("国家/地区")}</dt><dd>{row.country ? <CountryFlag code={row.country}/> : null} {countryName(row.country,locale)}</dd></div>
      <div className="recent-visitor-pages"><dt>{t("访问页面（最多 20 个，不按访问顺序）")}</dt><dd>{row.pages.map(page=><span key={page}>{page || "/"}</span>)}</dd></div>
    </dl></details>)}
    </div>
    <footer><span>{data && data.total>200 ? t("仅显示最近 200 位访客。") : ""}</span><span>{data ? `${t("更新于")} ${format(data.fetchedAt)} · ${data.timezone}` : ""}</span></footer>
  </dialog>,document.body)
}
