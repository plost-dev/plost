import type { OverviewPlace } from "~/components/OverviewGlobe"
import {rpc} from "./rpc"
import type {LivePoint} from "./live-arrivals"
import type {Overview,SeriesBy,Row,RecentVisitor,RecentEvent} from "./ch"
import type {DashSearch} from "./range"
import type {SiteCheckReport} from "./site-checks"

export type User={id:number;email:string;name:string;theme:"system"|"dark"|"light";locale:"zh-CN"|"en";avatar:string;totp:boolean;totp_enabled:boolean;isAdmin:boolean;gravatarUrl?:string}
export type Site={id:number;domain:string;name:string;timezone:string;public:boolean;createdAt:number}
type Input<T>={data:T}
type GoOverview={visitors:number;visits:number;pageviews:number;online:number;bounceRate:number;duration:number}
type GoPoint={at:number;visitors:number;pageviews:number;visits:number;bounceRate:number;duration:number}
export type GoReport={site:Site;range:{from:string;to:string;period:string};overview:GoOverview;previous:GoOverview;series:GoPoint[];sources:Row[];pages:Row[];countries:Row[];regions?:Array<{country:string;name:string;value:number}>;cities?:Array<{country:string;name:string;label?:string;value:number}>;browsers:Row[];os:Row[];devices:Row[];live?:Row[];liveRegions?:Array<{country:string;name:string;value:number}>;livePoints?:LivePoint[];languages:Row[];screens:Row[];events:Row[];readonly:boolean;details?:Record<string,any>}
const sitePath=(domain:string)=>"/api/sites/"+encodeURIComponent(domain)
const asUser=(u:User|null)=>u?{...u,isAdmin:true,totp_enabled:u.totp}:null
// The root and the page loaders both ask who is signed in as a page opens; while one answer is on its way, they share it.
let meAsking:Promise<User|null>|null=null
export function meFn(){
  meAsking??=rpc<{user:User|null}>("/api/auth").then(result=>asUser(result.user)).finally(()=>{meAsking=null})
  return meAsking
}
export async function instanceSetupFn(){const result=await rpc<{needsSetup:boolean;passkey?:boolean}>("/api/auth");return {needsSetup:result.needsSetup,passkey:!!result.passkey}}
export async function loginFn({data}:Input<{email:string;password:string}>){const r=await rpc<{requiresTOTP:boolean}>("/api/login","POST",data);return r.requiresTOTP?{needs2fa:true,email:data.email}:{needs2fa:false}}
export const setupAdminFn=({data}:Input<{email:string;password:string;name:string}>)=>rpc("/api/setup","POST",data)
export const logoutFn=()=>rpc("/api/logout","POST")
export async function updateProfileFn({data}:Input<Omit<Partial<User>,"avatar"|"theme">&{avatar?:string|null;theme?:string;currentPassword?:string;oldPassword?:string;password?:string}>){
  const u=await meFn();if(!u)throw Error("请先登录")
  const result=await rpc("/api/account","PUT",{name:data.name??u.name,email:data.email??u.email,locale:data.locale??u.locale,theme:data.theme??u.theme,...("avatar"in data?{avatar:data.avatar}:{}),currentPassword:data.currentPassword||data.oldPassword||"",password:data.password||""})
  try{if(data.theme)localStorage.setItem("plost:theme",data.theme);if(data.locale)localStorage.setItem("plost:locale",data.locale)}catch{}
  return result
}
export async function accountSettingsFn(){const user=await meFn();if(!user)throw Error("请先登录");return {user,passkeys:[]}}
export async function siteDomainsFn(){return (await rpc<Array<{site:Site;iconVersion?:number}>>("/api/sites")).map(r=>({...r.site,iconVersion:r.iconVersion||0}))}
export async function sitesFn(){
  const items=await rpc<Array<{site:Site;overview:GoOverview;series:GoPoint[];previous:GoOverview;iconVersion?:number}>>("/api/sites?period=24h&ui=1")
  return items.map(({site:s,overview:o,series,previous,iconVersion})=>({...s,iconVersion:iconVersion||0,visitors:o.visitors,visits:o.visits,pageviews:o.pageviews,live:o.online,change:previous.visitors?Math.round((o.visitors-previous.visitors)/previous.visitors*100):0,sparkline:series.map(p=>p.visitors)}))
}
export async function trafficPulseFn(){const items=await rpc<Array<{site:Site;overview:GoOverview}>>("/api/sites");return {sites:{count:items.length,visitors:items.reduce((n,s)=>n+s.overview.visitors,0),visits:items.reduce((n,s)=>n+s.overview.visits,0),pageviews:items.reduce((n,s)=>n+s.overview.pageviews,0)}}}
export const overviewLiveFn=()=>rpc<OverviewPlace[]>("/api/overview/live")
export type OverviewCharts={totals:GoOverview;previous:GoOverview}
export const overviewChartsFn=()=>rpc<OverviewCharts>("/api/overview/charts")
export async function createSiteFn({data}:Input<{domain:string;timezone:string;name?:string|null}>){return rpc<{domain:string}>("/api/sites","POST",data)}
export async function siteReadyFn({data}:Input<{domain:string}>){const r=await rpc<GoReport>(sitePath(data.domain)+"/report?period=all");return {ready:r.overview.pageviews>0}}
export type Share={id:number;name:string;slug:string;createdAt:number}
const siteSettingsOnly=({data}:Input<{domain:string}>)=>rpc<{site:Site;shares:Share[];iconVersion?:number}>(sitePath(data.domain)+"/settings")
export const siteCheckFn=({data}:Input<{domain:string}>)=>rpc<{report:SiteCheckReport;canManage:boolean}>(sitePath(data.domain)+"/checks")
export const runSiteCheckFn=({data}:Input<{domain:string}>)=>rpc<{ok:boolean}>(sitePath(data.domain)+"/checks/run","POST",{})
export const saveSiteCheckFn=({data}:Input<{domain:string;enabled:boolean;path:string}>)=>rpc<{ok:boolean}>(sitePath(data.domain)+"/checks","PUT",{enabled:data.enabled,path:data.path})
export async function siteSettingsFn({data}:Input<{domain:string}>){
  const [settings,health]=await Promise.all([siteSettingsOnly({data}),siteCheckFn({data}).then(r=>r.report,()=>null)])
  return {...settings,health}
}
export async function saveSettingsFn({data}:Input<{domain:string;name?:string;timezone?:string;public?:boolean;newDomain?:string}>){const {site}=await siteSettingsOnly({data});return rpc<{domain:string}>(sitePath(data.domain)+"/settings","PUT",{name:data.name??site.name,domain:data.newDomain??site.domain,timezone:data.timezone??site.timezone,public:data.public??site.public})}
export const removeSiteFn=({data}:Input<{domain:string}>)=>rpc(sitePath(data.domain),"DELETE",{confirmation:data.domain})
export const createShareFn=({data}:Input<{domain:string;name:string}>)=>rpc<{token:string}>(sitePath(data.domain)+"/shares","POST",{name:data.name})
export const removeShareFn=({data}:Input<{domain:string;id:number}>)=>rpc(sitePath(data.domain)+"/shares/"+data.id,"DELETE")

export function overview(o:GoOverview):Overview{return {visitors:o.visitors,session_visitors:o.visitors,visits:o.visits,pageviews:o.pageviews,views_per_visit:o.visits?o.pageviews/o.visits:0,bounce_rate:o.bounceRate,visit_duration:o.duration,live:o.online}}
export function adaptReport(r:GoReport){
  const detail=r.details||{}
  const time=(at:number)=>new Date(at*1000).toLocaleString("sv-SE",{timeZone:r.site.timezone})
  const point=(key:keyof GoPoint)=>r.series.map(p=>({date:time(p.at),value:Number(p[key])}))
  const seriesBy:SeriesBy={visitors:point("visitors"),session_visitors:point("visitors"),pageviews:point("pageviews"),visits:point("visits"),views_per_visit:r.series.map(p=>({date:time(p.at),value:p.visits?p.pageviews/p.visits:0})),bounce_rate:point("bounceRate"),visit_duration:point("duration")}
  const total=(rows:Row[])=>rows.reduce((sum,row)=>sum+row.value,0)
  return {site:r.site,overview:overview(r.overview),compare:r.range.period==="all"?{}:overview(r.previous),series:seriesBy.visitors,seriesBy,sources:r.sources,pages:(detail.pageVisitors||r.pages)as Row[],countries:r.countries.filter(r=>r.name),regions:(r.regions||[]).map(x=>({name:x.name,value:x.value,code:x.country})) as Row[],cities:(r.cities||[]).map(x=>({name:x.name,label:x.label||undefined,value:x.value,code:x.country})) as Row[],browsers:r.browsers,os:r.os,devices:r.devices,languages:r.languages,screens:r.screens,channels:(detail.channels||[]) as Row[],aiSources:(detail.aiSources||[]) as Row[],entryPages:(detail.entryPages||[]) as Row[],exitPages:(detail.exitPages||[]) as Row[],hostnames:(detail.hostnames||[]) as Row[],titles:(detail.titles||[]) as Row[],notFoundPages:(detail.notFoundPages||[]) as Row[],utm:(detail.utm||[]) as Row[],utmMediums:(detail.utmMediums||[]) as Row[],campaigns:(detail.campaigns||[]) as Row[],heatmap:(detail.heatmap||[]) as Array<{dow:number;hour:number;value:number}>,mapboxToken:detail.mapboxToken||"",googleMapsKey:String(detail.googleMapsKey||""),mapOutline:(detail.mapOutline==="countries"?"countries":"regions") as "regions"|"countries",livePoints:r.livePoints||[],liveRegions:r.liveRegions||[],liveGeo:(r.live||[]).flatMap(row=>Array.from({length:row.value},()=>({country:row.name}))) as Array<{country:string}>,health:null as SiteCheckReport|null,eventSummary:{events:(detail.eventCounts||[]) as Row[],total:total(detail.eventCounts||[]),downloads:(detail.downloads||[]) as Row[],downloadsTotal:total(detail.downloads||[]),outbound:(detail.outbound||[]) as Row[],outboundTotal:total(detail.outbound||[]),properties:(detail.properties||[]) as Row[]},access:{member:!r.readonly,canManage:!r.readonly}}
}
function query(data:Partial<DashSearch>){const q=new URLSearchParams({period:data.period||"24h",ui:"1"});for(const key of ["from","to","interval","source","page","country","browser","os","device","hostname"]as const){if(data[key])q.set(key,String(data[key]))}return q.toString()}
export async function dashboardFn({data}:Input<Partial<DashSearch>&{domain:string}>){
  // What the server learns about the site is only for the signed-in manager; a visitor of a public site gets 401 here and sees nothing.
  const [report,health]=await Promise.all([rpc<GoReport>(sitePath(data.domain)+"/report?"+query(data)),siteCheckFn({data}).then(r=>r.report,()=>null)])
  return {...adaptReport(report),health:report.readonly?null:health}
}
export async function shareDashboardFn({data}:Input<Partial<DashSearch>&{slug:string}>){return adaptReport(await rpc<GoReport>("/api/share/"+encodeURIComponent(data.slug)+"?"+query(data)))}
export const recentVisitorsFn=({data}:Input<{domain:string}>)=>rpc<{rows:RecentVisitor[];total:number;online:number;timezone:string;fetchedAt:string}>(sitePath(data.domain)+"/visitors")
export async function activityFn({data}:Input<{domain:string;window?:string}>){const rows=await rpc<Array<{at:number;name:string;page:string;source:string;country:string;browser:string;os:string;device:string}>>(sitePath(data.domain)+"/activity?period="+(data.window==="30m"?"realtime":data.window==="7d"?"7d":"24h"));const {site}=await siteSettingsFn({data});return {rows:rows.slice(0,200).map(r=>({name:r.name,pathname:r.page,source:r.source,country:r.country,browser:r.browser,os:r.os,device:r.device,time:new Date(r.at*1000).toISOString()})) as RecentEvent[],timezone:site.timezone,hasMore:rows.length>200,fetchedAt:new Date().toISOString()}}
/** Uploads a site's own icon (PNG, JPG, GIF, WebP or ICO, up to 256 KB) and returns its new version. */
export async function uploadSiteIconFn(domain:string,file:File){
  const response=await fetch(sitePath(domain)+"/icon",{method:"PUT",credentials:"same-origin",headers:{"X-Plost-Request":"1","Content-Type":file.type||"application/octet-stream"},body:file})
  const result=await response.json().catch(()=>({}))
  if(!response.ok)throw new Error(typeof result.error==="string"?result.error:"上传失败")
  return result as {iconVersion:number}
}
export const removeSiteIconFn=(domain:string)=>rpc<{iconVersion:number}>(sitePath(domain)+"/icon","DELETE")
