import { Link, useRouter } from "@tanstack/react-router"
import { useEffect, useRef, useState } from "react"
import { UIIcon } from "~/components/UIIcon"
import { OverviewGlobe, type OverviewPlace } from "~/components/OverviewGlobe"
import { SiteSparkline } from "~/components/SiteSparkline"
import { overviewChartsFn, overviewLiveFn, type OverviewCharts, type sitesFn } from "~/lib/actions"
import { numberShort } from "~/lib/format"
import { axisScale, formatAxisTick } from "~/lib/axis"
import { useT } from "~/lib/i18n"
import type { SiteLayout } from "~/lib/site-layout-preference"
import { OVERVIEW_CHARTS, OVERVIEW_CHART_LABEL, readOverviewChart, saveOverviewChart, type OverviewChart } from "~/lib/overview-chart-preference"

type OverviewSite = Awaited<ReturnType<typeof sitesFn>>[number]
type Point = OverviewCharts["points"][number]

const change = (now: number, before: number) => before ? Math.round((now - before) / before * 100) : 0
const DAYS = ["一", "二", "三", "四", "五", "六", "日"]
// Up to three sites get a colour of their own; the rest share the last, grey one.
const SITE_COLORS = ["var(--accent-color)", "#60a5fa", "#f59e0b", "var(--color-gray-300)"]

export function OverviewHero({ sites, layout, onLayout }: { sites: OverviewSite[], layout: SiteLayout, onLayout: (value: SiteLayout) => void }) {
  const { t } = useT()
  const router = useRouter()
  const [places, setPlaces] = useState<OverviewPlace[]>([])
  const [chart, setChart] = useState<OverviewChart>(readOverviewChart)
  const [data, setData] = useState<{ view: OverviewChart, charts: OverviewCharts } | null>(null)
  const [updatedAt, setUpdatedAt] = useState(() => Date.now())
  const [now, setNow] = useState(() => Date.now())
  const [refreshing, setRefreshing] = useState(false)

  // Each loader refresh hands over a new list; the globe and the chart follow it.
  useEffect(() => {
    let alive = true
    setUpdatedAt(Date.now())
    void overviewLiveFn().then((rows) => { if (alive) setPlaces(rows) }).catch(() => {})
    return () => { alive = false }
  }, [sites])
  useEffect(() => {
    let alive = true
    void overviewChartsFn(chart).then((charts) => { if (alive) setData({ view: chart, charts }) }).catch(() => {})
    return () => { alive = false }
  }, [sites, chart])
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])

  const minutes = Math.floor(Math.max(0, now - updatedAt) / 60_000)
  async function refresh() {
    setRefreshing(true)
    try { await router.invalidate() } finally { setRefreshing(false); setNow(Date.now()) }
  }
  function chooseChart(value: OverviewChart) {
    setChart(value)
    saveOverviewChart(value)
  }

  const zone = sites[0]?.timezone || "UTC"
  const ready = data?.view === chart ? data.charts : null
  return (
    <section className="squircle overview-hero" aria-labelledby="overview-hero-title">
      <div className="overview-hero-body">
        <div className="overview-hero-head">
          <div>
            <h1 id="overview-hero-title" className="overview-hero-title">{t("nav.sites")}</h1>
            <p className="overview-hero-sub">
              {t("footer.sites").replace("{n}", numberShort(sites.length))} · <span role="status">{minutes < 1 ? t("刚刚更新") : t("{n} 分钟前更新").replace("{n}", String(minutes))}</span>
            </p>
          </div>
          <div className="overview-hero-tools">
            <ChartMenu chart={chart} onChart={chooseChart} />
            <div className="squircle overview-hero-seg" role="group" aria-label={t("站点显示方式")}>
              <button type="button" className="squircle" aria-pressed={layout === "grid"} onClick={() => onLayout("grid")} aria-label={t("卡片")} title={t("卡片")}><UIIcon name="grid" /></button>
              <button type="button" className="squircle" aria-pressed={layout === "compact"} onClick={() => onLayout("compact")} aria-label={t("紧凑")} title={t("紧凑")}><UIIcon name="list" /></button>
            </div>
            <button type="button" className="squircle overview-hero-icon" disabled={refreshing} onClick={refresh} aria-label={t("刷新")} title={t("刷新")}>
              <UIIcon name="refresh" className={refreshing ? "is-spinning" : undefined} />
            </button>
            <Link to="/sites/new" className="squircle overview-hero-icon is-primary" aria-label={t("添加网站")} title={t("添加网站")}><UIIcon name="plus" /></Link>
          </div>
        </div>
        {!ready ? <div className="overview-hero-loading" aria-hidden="true" />
          : chart === "hourly" ? <HourlyChart charts={ready} zone={zone} />
          : chart === "daily" ? <DailyChart charts={ready} zone={zone} />
          : chart === "heatmap" ? <HeatmapChart charts={ready} />
          : chart === "sites" ? <SitesChart charts={ready} sites={sites} />
          : <MetricsChart charts={ready} />}
      </div>
      <OverviewGlobe places={places} />
    </section>
  )
}

function ChartMenu({ chart, onChart }: { chart: OverviewChart, onChart: (value: OverviewChart) => void }) {
  const { t } = useT()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const closeOutside = (event: Event) => { if (event.target instanceof Node && !ref.current?.contains(event.target)) setOpen(false) }
    const onEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return
      ref.current?.querySelector<HTMLButtonElement>("button")?.focus()
      setOpen(false)
    }
    document.addEventListener("pointerdown", closeOutside, true)
    document.addEventListener("focusin", closeOutside)
    document.addEventListener("keydown", onEscape)
    return () => {
      document.removeEventListener("pointerdown", closeOutside, true)
      document.removeEventListener("focusin", closeOutside)
      document.removeEventListener("keydown", onEscape)
    }
  }, [open])
  return (
    <div ref={ref} className="relative">
      <button type="button" className="squircle overview-hero-icon" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)} aria-label={t("图表样式")} title={t("图表样式")}>
        <UIIcon name="bar-chart" />
      </button>
      {open ? (
        <div role="menu" aria-label={t("图表样式")} className="squircle dashboard-popover absolute right-0 z-20 mt-2 w-48 rounded-md bg-white p-1 font-medium text-gray-800 shadow-lg ring-1 ring-black/5">
          {OVERVIEW_CHARTS.map((id) => (
            <button key={id} type="button" role="menuitemradio" aria-checked={chart === id}
              className="squircle control-menu-item flex w-full items-center justify-between gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-gray-100"
              onClick={() => { onChart(id); setOpen(false) }}>
              {t(OVERVIEW_CHART_LABEL[id])}
              {chart === id ? <UIIcon name="check" className="size-4 text-gray-900" /> : null}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function Delta({ value }: { value: number }) {
  const { t } = useT()
  return (
    <span className={`overview-hero-delta${value > 0 ? " is-up" : value < 0 ? " is-down" : ""}`} title={t("与前一周期相比")}>
      {value > 0 ? <UIIcon name="arrow-up-right" /> : value < 0 ? <UIIcon name="arrow-down-right" /> : null}{Math.abs(value)}%
    </span>
  )
}

const clock = (at: number, zone: string) => new Date(at * 1000).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: zone })
const monthDay = (at: number, zone: string) => new Date(at * 1000).toLocaleDateString("zh-CN", { month: "numeric", day: "numeric", timeZone: zone })

/** Visitors in front of pageviews, one column per bucket; the last, unfinished bucket is hatched. */
function Bars({ points, label, tick }: { points: Point[], label: (p: Point) => string, tick: (p: Point, i: number) => string | null }) {
  const { t, locale } = useT()
  const [hover, setHover] = useState<number | null>(null)
  const scale = axisScale(Math.max(1, ...points.map(p => p.pageviews)))
  const at = (v: number) => `calc(18px + (100% - 18px) * ${v / scale.max})`
  const shown = hover === null ? null : points[hover]
  return (
    <>
      <div className="overview-hero-chart">
        <div className="overview-hero-axis">{scale.ticks.map(v => <span key={v} style={{ bottom: at(v) }}>{formatAxisTick(v)}</span>)}</div>
        <div className="overview-hero-plot" onPointerLeave={() => setHover(null)}>
          {scale.ticks.slice(1).map(v => <i key={v} className="overview-hero-grid" style={{ bottom: at(v) }} />)}
          <div className="overview-hero-bars">
            {points.map((p, i) => (
              <div key={p.at} className={`overview-hero-bar${hover === i ? " is-hover" : ""}${i === points.length - 1 ? " is-now" : ""}`} onPointerEnter={() => setHover(i)}>
                <span className="pv" style={{ height: `${p.pageviews / scale.max * 100}%` }} />
                <span className="uv" style={{ height: `${p.visitors / scale.max * 100}%` }} />
                {tick(p, i) ? <em>{tick(p, i)}</em> : null}
              </div>
            ))}
          </div>
          {shown && hover !== null ? (
            <div className="squircle overview-hero-tip" style={{ left: `${(hover + .5) / points.length * 100}%` }}>
              <strong>{label(shown)}</strong>
              <span><i className="uv" />{t("访客")} {shown.visitors.toLocaleString(locale)}</span>
              <span><i className="pv" />{t("浏览量")} {shown.pageviews.toLocaleString(locale)}</span>
            </div>
          ) : null}
        </div>
      </div>
      <div className="overview-hero-legend"><span><i className="uv" />{t("访客")}</span><span><i className="pv" />{t("浏览量")}</span></div>
    </>
  )
}

function HourlyChart({ charts, zone }: { charts: OverviewCharts, zone: string }) {
  const { t, locale } = useT()
  const points = charts.points
  const peak = points.reduce<Point | null>((best, p) => !best || p.visitors > best.visitors ? p : best, null)
  return (
    <>
      <div className="overview-hero-kpis">
        <div className="overview-hero-big"><strong>{numberShort(charts.totals.visitors)}</strong><Delta value={change(charts.totals.visitors, charts.previous.visitors)} /></div>
        <p className="overview-hero-note">
          {t("访客（近 24 小时，全部站点）")}
          {peak && peak.visitors > 0 ? ` · ${t("高峰 {0}，{1} 人", { 0: clock(peak.at, zone), 1: peak.visitors.toLocaleString(locale) })}` : ""}
        </p>
      </div>
      <Bars points={points} label={p => `${clock(p.at, zone)} – ${clock(p.at + 3600, zone)}`}
        tick={(p, i) => i === points.length - 1 ? t("现在") : i % 6 === 0 && i < points.length - 3 ? clock(p.at, zone) : null} />
    </>
  )
}

function DailyChart({ charts, zone }: { charts: OverviewCharts, zone: string }) {
  const { t } = useT()
  const points = charts.points
  const total = points.reduce((sum, p) => sum + p.visitors, 0)
  return (
    <>
      <div className="overview-hero-kpis is-row">
        <Figure value={total} label={t("访客（近 30 天）")} />
        <Figure value={points.length ? Math.round(total / points.length) : 0} label={t("日均")} />
        <Figure value={points[points.length - 1]?.visitors || 0} label={t("今天（截至现在）")} />
      </div>
      <Bars points={points} label={p => monthDay(p.at, zone)}
        tick={(p, i) => i === points.length - 1 ? t("今天") : i % 7 === 0 && i < points.length - 4 ? monthDay(p.at, zone) : null} />
    </>
  )
}

function Figure({ value, label, text }: { value?: number, label: string, text?: string }) {
  return <div><strong className="overview-hero-mid">{text ?? numberShort(value || 0)}</strong><p className="overview-hero-note">{label}</p></div>
}

function HeatmapChart({ charts }: { charts: OverviewCharts }) {
  const { t, locale } = useT()
  const [hover, setHover] = useState<{ d: number, h: number } | null>(null)
  const cells = new Map(charts.heatmap.map(c => [`${c.dow}-${c.hour}`, c.value]))
  const max = Math.max(1, ...cells.values())
  let best = { d: 0, h: 0, v: 0 }
  for (const c of charts.heatmap) if (c.value > best.v) best = { d: c.dow, h: c.hour, v: c.value }
  const total = charts.heatmap.reduce((sum, c) => sum + c.value, 0)
  const slot = (d: number, h: number) => `${t("周" + DAYS[d - 1])} ${String(h).padStart(2, "0")}:00`
  return (
    <>
      <div className="overview-hero-kpis is-row">
        <Figure value={total} label={t("访客（近 30 天）")} />
        <Figure text={best.v ? slot(best.d, best.h) : "—"} label={t("最忙时段")} />
      </div>
      <div className="overview-hero-heat" onPointerLeave={() => setHover(null)}>
        {DAYS.map((name, index) => (
          <div key={name} className="overview-hero-heat-row">
            <span>{t(name)}</span>
            {Array.from({ length: 24 }, (_, h) => {
              const v = cells.get(`${index + 1}-${h}`) || 0
              return <i key={h} className={hover?.d === index + 1 && hover.h === h ? "is-hover" : undefined} onPointerEnter={() => setHover({ d: index + 1, h })}
                style={{ background: v ? `color-mix(in srgb, var(--accent-color) ${Math.round(12 + v / max * 88)}%, var(--color-gray-100))` : undefined }} />
            })}
          </div>
        ))}
        <div className="overview-hero-heat-row is-axis"><span />{Array.from({ length: 24 }, (_, h) => <em key={h}>{h % 6 === 0 ? String(h).padStart(2, "0") : ""}</em>)}</div>
      </div>
      <p className="overview-hero-note overview-hero-heat-foot">
        {hover ? t("{0} · {1} 访客", { 0: slot(hover.d, hover.h), 1: (cells.get(`${hover.d}-${hover.h}`) || 0).toLocaleString(locale) }) : t("近 30 天按星期和小时汇总的访客，颜色越深越忙")}
      </p>
    </>
  )
}

function SitesChart({ charts, sites }: { charts: OverviewCharts, sites: OverviewSite[] }) {
  const { t, locale } = useT()
  const ranked = [...sites].sort((a, b) => b.visitors - a.visitors)
  const rest = ranked.slice(3)
  const parts = ranked.slice(0, 3).map(s => ({ key: s.domain, name: s.name || s.domain, visitors: s.visitors, change: s.change as number | null }))
  if (rest.length) parts.push({ key: "", name: t("其他 {0} 个站点", { 0: rest.length }), visitors: rest.reduce((sum, s) => sum + s.visitors, 0), change: null })
  const sum = Math.max(1, parts.reduce((total, p) => total + p.visitors, 0))
  const share = (v: number) => `${Math.round(v / sum * 100)}%`
  return (
    <>
      <div className="overview-hero-kpis">
        <div className="overview-hero-big"><strong>{numberShort(charts.totals.visitors)}</strong><Delta value={change(charts.totals.visitors, charts.previous.visitors)} /></div>
        <p className="overview-hero-note">{t("访客（近 24 小时，全部站点）")}</p>
      </div>
      <div className="squircle overview-hero-share">
        {parts.map((p, i) => p.visitors ? <span key={p.key} style={{ flexGrow: p.visitors, background: SITE_COLORS[p.key ? i : 3] }} title={`${p.name} ${share(p.visitors)}`} /> : null)}
      </div>
      <ul className="overview-hero-share-list">
        {parts.map((p, i) => (
          <li key={p.key}>
            <i style={{ background: SITE_COLORS[p.key ? i : 3] }} />
            <span className="name" title={p.name}>{p.name}</span>
            <span className="num">{p.visitors.toLocaleString(locale)}</span>
            <span className="pct">{share(p.visitors)}</span>
            {p.change === null ? <span /> : <Delta value={p.change} />}
          </li>
        ))}
      </ul>
    </>
  )
}

function MetricsChart({ charts }: { charts: OverviewCharts }) {
  const { t } = useT()
  const { totals, previous, points } = charts
  const tiles: Array<{ label: string, value: number, line?: number[], delta?: number }> = [
    { label: t("访客"), value: totals.visitors, line: points.map(p => p.visitors), delta: change(totals.visitors, previous.visitors) },
    { label: t("访问次数"), value: totals.visits, line: points.map(p => p.visits), delta: change(totals.visits, previous.visits) },
    { label: t("浏览量"), value: totals.pageviews, line: points.map(p => p.pageviews), delta: change(totals.pageviews, previous.pageviews) },
    { label: t("当前在线"), value: totals.online },
  ]
  return (
    <>
      <p className="overview-hero-note">{t("近 24 小时 · 全部站点")}</p>
      <div className="overview-hero-tiles">
        {tiles.map(tile => (
          <div key={tile.label} className={`squircle overview-hero-tile${tile.line ? "" : " is-online"}`}>
            <span className="label">{tile.line ? null : <i />}{tile.label}</span>
            <strong>{numberShort(tile.value)}</strong>
            {tile.delta === undefined ? <span className="overview-hero-note">{t("实时，5 分钟内")}</span> : <Delta value={tile.delta} />}
            {tile.line ? <span className="spark"><SiteSparkline values={tile.line} /></span> : null}
          </div>
        ))}
      </div>
    </>
  )
}
