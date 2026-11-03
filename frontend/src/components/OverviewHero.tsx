import { Link, useRouter } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { UIIcon } from "~/components/UIIcon"
import { OverviewGlobe, type OverviewMapMode, type OverviewPlace } from "~/components/OverviewGlobe"
import { overviewChartsFn, overviewLiveFn, type OverviewCharts, type sitesFn } from "~/lib/actions"
import { numberShort } from "~/lib/format"
import { useT } from "~/lib/i18n"
import type { SiteLayout } from "~/lib/site-layout-preference"

type OverviewSite = Awaited<ReturnType<typeof sitesFn>>[number]

const change = (now: number, before: number) => before ? Math.round((now - before) / before * 100) : 0

export function OverviewHero({ sites, layout, onLayout }: { sites: OverviewSite[], layout: SiteLayout, onLayout: (value: SiteLayout) => void }) {
  const { t } = useT()
  const router = useRouter()
  const [places, setPlaces] = useState<OverviewPlace[]>([])
  const [charts, setCharts] = useState<OverviewCharts | null>(null)
  const [mode, setMode] = useState<OverviewMapMode>("globe")
  const [updatedAt, setUpdatedAt] = useState(() => Date.now())
  const [now, setNow] = useState(() => Date.now())
  const [refreshing, setRefreshing] = useState(false)

  // Each loader refresh hands over a new list; the globe and the figures follow it.
  useEffect(() => {
    let alive = true
    setUpdatedAt(Date.now())
    void overviewLiveFn().then((rows) => { if (alive) setPlaces(rows) }).catch(() => {})
    return () => { alive = false }
  }, [sites])
  useEffect(() => {
    let alive = true
    void overviewChartsFn().then((value) => { if (alive) setCharts(value) }).catch(() => {})
    return () => { alive = false }
  }, [sites])
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])

  const minutes = Math.floor(Math.max(0, now - updatedAt) / 60_000)
  async function refresh() {
    setRefreshing(true)
    try { await router.invalidate() } finally { setRefreshing(false); setNow(Date.now()) }
  }
  return (
    <section className="squircle overview-hero" aria-labelledby="overview-hero-title">
      <div className="overview-hero-head">
        <div className="overview-hero-heading">
          <span className="squircle overview-hero-badge" aria-hidden="true"><UIIcon name="globe" /></span>
          <h1 id="overview-hero-title" className="overview-hero-title">{t("nav.sites")}</h1>
          <p className="overview-hero-sub">
            {t("footer.sites").replace("{n}", numberShort(sites.length))} · <span role="status">{minutes < 1 ? t("刚刚更新") : t("{n} 分钟前更新").replace("{n}", String(minutes))}</span>
            <button type="button" className="overview-hero-refresh" disabled={refreshing} onClick={refresh} aria-label={t("刷新")} title={t("刷新")}>
              <UIIcon name="refresh" className={refreshing ? "is-spinning" : undefined} />
            </button>
          </p>
        </div>
        <div className="overview-hero-tools">
          <div className="squircle overview-hero-seg is-text" role="group" aria-label={t("地图样式")}>
            <button type="button" className="squircle" aria-pressed={mode === "globe"} onClick={() => setMode("globe")}>{t("地球")}</button>
            <button type="button" className="squircle" aria-pressed={mode === "map"} onClick={() => setMode("map")}>{t("平面")}</button>
          </div>
          <div className="squircle overview-hero-seg" role="group" aria-label={t("站点显示方式")}>
            <button type="button" className="squircle" aria-pressed={layout === "grid"} onClick={() => onLayout("grid")} aria-label={t("卡片")} title={t("卡片")}><UIIcon name="grid" /></button>
            <button type="button" className="squircle" aria-pressed={layout === "compact"} onClick={() => onLayout("compact")} aria-label={t("紧凑")} title={t("紧凑")}><UIIcon name="list" /></button>
          </div>
          <Link to="/sites/new" className="squircle overview-hero-icon is-primary" aria-label={t("添加网站")} title={t("添加网站")}><UIIcon name="plus" /></Link>
        </div>
      </div>
      <div className="overview-hero-body">
        {charts ? <MetricsChart charts={charts} /> : <div className="overview-hero-loading" aria-hidden="true" />}
      </div>
      <OverviewGlobe places={places} mode={mode} />
    </section>
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

function MetricsChart({ charts }: { charts: OverviewCharts }) {
  const { t } = useT()
  const { totals, previous } = charts
  const tiles: Array<{ label: string, value: number, delta?: number }> = [
    { label: t("访客"), value: totals.visitors, delta: change(totals.visitors, previous.visitors) },
    { label: t("访问次数"), value: totals.visits, delta: change(totals.visits, previous.visits) },
    { label: t("浏览量"), value: totals.pageviews, delta: change(totals.pageviews, previous.pageviews) },
    { label: t("当前在线"), value: totals.online },
  ]
  return (
    <>
      <p className="overview-hero-note">{t("近 24 小时 · 全部站点")}</p>
      <div className="overview-hero-tiles">
        {tiles.map(tile => (
          <div key={tile.label} className={`squircle overview-hero-tile${tile.delta === undefined ? " is-online" : ""}`}>
            <span className="label">{tile.delta === undefined ? <i /> : null}{tile.label}</span>
            <strong>{numberShort(tile.value)}</strong>
            {tile.delta === undefined ? <span className="overview-hero-note">{t("实时，5 分钟内")}</span> : <Delta value={tile.delta} />}
          </div>
        ))}
      </div>
    </>
  )
}
