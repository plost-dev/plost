import {downloadDashboardZip} from "~/lib/export"
import {useState} from "react"
import {UIIcon} from "./UIIcon"
import {Surface} from "./Surface"
import {DotsLoader} from "./DotsLoader"
import {DashTopBar} from "./DashTopBar"
import {VisitorGraph} from "./VisitorGraph"
import {MapCard} from "./WorldMap"
import {TrafficHeatmap} from "./TrafficHeatmap"
import {RecentVisitors} from "./RecentVisitors"
import {List,ReportCard} from "./ui"
import {PageReport} from "./PageReport"
import {RowIcon} from "./icons"
import {useT} from "~/lib/i18n"
import {countryName} from "~/lib/countries"
import {languageName} from "~/lib/languages"
import {isDirectSource} from "~/lib/source-icons"
import {reportValueLabel} from "~/lib/report-values"
import {isoDate,parseDate,parseDashSearch,type DashSearch} from "~/lib/range"
import {readChartType,saveChartType,type ChartType} from "~/lib/chart-type-preference"
import {readHiddenSeries,saveHiddenSeries} from "~/lib/chart-series-preference"
import type {TrafficMetric} from "./TrendChart"
import type {dashboardFn} from "~/lib/actions"

type Data=Omit<Awaited<ReturnType<typeof dashboardFn>>,"access">

// Original card, chart, map, toolbar and report primitives are reused verbatim.
// Only the removed business templates and search integrations are absent.
export function AnalyticsDashboard({data,domain,search,onSearch,readonly=true,canManage=false,loading=false}:{data:Data;domain:string;search:DashSearch;onSearch:(s:DashSearch)=>void;readonly?:boolean;canManage?:boolean;loading?:boolean}){
  const {t,locale}=useT()
  const [sourceTab,setSourceTab]=useState("source")
  const [pageTab,setPageTab]=useState("pages")
  const [pageMode,setPageMode]=useState("path")
  const [eventMode,setEventMode]=useState("count")
  const [locationTab,setLocationTab]=useState("countries")
  const [deviceTab,setDeviceTab]=useState("browser")
  const [visitorsOpen,setVisitorsOpen]=useState(false)
  const [chartType,setChartType]=useState<ChartType>(readChartType)
  const changeChartType=(next:ChartType)=>{setChartType(next);saveChartType(next)}
  const [hiddenSeries,setHiddenSeries]=useState<ReadonlyArray<TrafficMetric>>(readHiddenSeries)
  const toggleSeries=(key:TrafficMetric)=>{
    const next=hiddenSeries.includes(key)?hiddenSeries.filter(k=>k!==key):[...hiddenSeries,key]
    if(next.length>=3)return
    setHiddenSeries(next);saveHiddenSeries(next)
  }
  const go=(next:Partial<DashSearch>)=>onSearch(parseDashSearch({...search,...next}))
  const countries=data.countries.map(r=>({...r,label:countryName(r.name,locale)}))
  const sources=sourceTab==="channel"?data.channels:sourceTab==="ai"?data.aiSources:sourceTab==="utm_source"?data.utm:sourceTab==="utm_medium"?data.utmMediums:sourceTab==="utm_campaign"?data.campaigns:data.sources
  const sourceKind=sourceTab==="source"?"source":sourceTab==="channel"?"channel":sourceTab==="ai"?"ai":"campaign"
  const deviceRows=deviceTab==="os"?data.os:deviceTab==="device"?data.devices:deviceTab==="language"?data.languages.map(r=>({...r,label:languageName(r.name,locale)})):deviceTab==="screen"?data.screens:data.browsers
  const deviceKind=deviceTab==="os"?"os":deviceTab==="device"?"device":deviceTab==="language"?"language":deviceTab==="screen"?"screen":"browser"
  const deviceTitle=t(deviceTab==="os"?"操作系统":deviceTab==="device"?"设备":deviceTab==="language"?"语言":deviceTab==="screen"?"屏幕":"浏览器")
  const locationRows=locationTab==="regions"?data.regions:locationTab==="cities"?data.cities:countries
  const chips=([
    search.source&&{key:"source",label:t("来源 {0}",{0:isDirectSource(search.source)?t("直接访问"):search.source})},
    search.page&&{key:"page",label:t("页面 {0}",{0:search.page})},
    search.country&&{key:"country",label:t("地区 {0}",{0:countryName(search.country,locale)})},
    search.browser&&{key:"browser",label:t("浏览器 {0}",{0:reportValueLabel("browser",search.browser,locale)})},
    search.os&&{key:"os",label:t("系统 {0}",{0:reportValueLabel("os",search.os,locale)})},
    search.device&&{key:"device",label:t("设备 {0}",{0:search.device})},
  ].filter(Boolean))as Array<{key:"source"|"page"|"country"|"browser"|"os"|"device";label:string}>
  return <div className="analytics-dashboard grid min-w-0 grid-cols-1 md:grid-cols-2" aria-busy={loading||undefined} data-loading={loading||undefined}>
    {loading?<DotsLoader className="dots-loader-floating" label={t("加载中…")}/>:null}
    <DashTopBar domain={domain} search={search} readonly={readonly} canManage={canManage} health={data.health} onPeriod={go} chartType={chartType} onChartType={changeChartType} hiddenSeries={hiddenSeries} onToggleSeries={toggleSeries}
      filterOptions={{page:data.pages,source:data.sources.map(r=>({...r,label:isDirectSource(r.name)?t("直接访问"):r.name})),country:countries,device:data.devices,browser:data.browsers,os:data.os}}
      filters={chips.length?<div className="dash-active-filters" role="group" aria-label={t("筛选")}>{chips.map(c=><button key={c.key} type="button" title={c.label} className="squircle active-filter-chip flex min-h-8 items-center gap-1.5 rounded-md px-2.5 py-1 text-sm" onClick={()=>go({[c.key]:""})}>{c.key!=="page"?<span className="report-value-icon"><RowIcon kind={c.key} name={String(search[c.key]||"")}/></span>:null}<span className="filter-chip-label">{c.label}</span><UIIcon name="close" className="ml-1.5 size-3"/></button>)}<button type="button" className="squircle control-compact text-sm text-gray-500 hover:text-gray-900" onClick={()=>go({source:"",page:"",country:"",browser:"",os:"",device:""})}>{t("清除筛选")}</button></div>:null}
      onExport={()=>downloadDashboardZip({...data,domain,from:search.from,to:search.to})}/>
    <VisitorGraph overview={data.overview} compare={data.compare} seriesBy={data.seriesBy} chartType={chartType} hiddenSeries={hiddenSeries} onToggleSeries={toggleSeries} interval={search.interval} realtime={search.period==="realtime"} onZoom={date=>{const day=date.slice(0,10);if(search.interval==="month"){const start=parseDate(day);go({period:"custom",from:isoDate(new Date(start.getFullYear(),start.getMonth(),1)),to:isoDate(new Date(start.getFullYear(),start.getMonth()+1,0))})}else go({period:"custom",from:day,to:day})}}/>
    <MapCard mapboxToken={data.mapboxToken} googleMapsKey={data.googleMapsKey} rows={countries} live={data.liveGeo} liveCount={data.overview.live} onViewVisitors={readonly?undefined:()=>setVisitorsOpen(true)} onCountryClick={code=>{setLocationTab("regions");go({country:code})}}/>
    {visitorsOpen&&!readonly?<RecentVisitors domain={domain} onClose={()=>setVisitorsOpen(false)}/>:null}
    <ReportCard className="location-report col-span-full" tabs={[{id:"countries",label:"国家/地区"},{id:"regions",label:"地区"},{id:"cities",label:"城市"}]} active={locationTab} onChange={setLocationTab} details={{title:t(locationTab==="regions"?"地区":locationTab==="cities"?"城市":"国家/地区"),kind:locationTab==="regions"?"region":locationTab==="cities"?"city":"country",rows:locationRows,onPick:locationTab==="countries"?name=>go({country:name}):undefined}}><List compact plain kind={locationTab==="regions"?"region":locationTab==="cities"?"city":"country"} rows={locationRows} onPick={locationTab==="countries"?name=>go({country:name}):undefined}/></ReportCard>
    <ReportCard className="source-report" tabs={[{id:"channel",label:"渠道"},{id:"source",label:"来源"},{id:"ai",label:"AI"},{id:"campaigns",label:"活动",dropdown:[{id:"utm_medium",label:"UTM 媒介"},{id:"utm_source",label:"UTM 来源"},{id:"utm_campaign",label:"UTM 活动"}]}]} active={sourceTab} onChange={setSourceTab} details={{title:t(sourceTab==="channel"?"渠道":sourceTab==="ai"?"AI 来源":sourceTab.startsWith("utm_")?"活动":"来源"),kind:sourceKind,rows:sources,onPick:sourceTab==="source"?name=>go({source:name}):undefined}}><List plain kind={sourceKind} rows={sources} onPick={sourceTab==="source"?name=>go({source:name}):undefined}/></ReportCard>
    <PageReport data={data} domain={domain} active={pageTab} onChange={setPageTab} mode={pageMode} onMode={setPageMode} eventMode={eventMode} onEventMode={setEventMode} onPick={name=>go({page:name})}/>
    <Surface className="analytics-panel traffic-report traffic-paired-report"><TrafficHeatmap cells={data.heatmap}/></Surface>
    <ReportCard className="device-report traffic-paired-report" tabs={[{id:"browser",label:"浏览器"},{id:"os",label:"操作系统"},{id:"device",label:"设备"},{id:"language",label:"语言"},{id:"screen",label:"屏幕"}]} active={deviceTab} onChange={setDeviceTab} details={{title:deviceTitle,kind:deviceKind,rows:deviceRows,onPick:deviceTab==="language"||deviceTab==="screen"?undefined:name=>go({[deviceTab==="os"?"os":deviceTab==="device"?"device":"browser"]:name})}}><div key={deviceTab} className="device-report-scroll" role="region" aria-label={deviceTitle} tabIndex={0}><List plain maxItems={0} kind={deviceKind} rows={deviceRows} onPick={deviceTab==="language"||deviceTab==="screen"?undefined:name=>go({[deviceTab==="os"?"os":deviceTab==="device"?"device":"browser"]:name})}/></div></ReportCard>
  </div>
}
