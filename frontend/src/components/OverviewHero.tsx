import { Link, useRouter } from "@tanstack/react-router"
import { useCallback, useEffect, useRef, useState } from "react"
import { UIIcon } from "~/components/UIIcon"
import type { OverviewPlace } from "~/components/OverviewGlobe"
import { OverviewMap } from "~/components/OverviewMap"
import { overviewMapFn, type OverviewMapConfig } from "~/lib/map-usage-actions"
import { RollingNumber } from "~/components/RollingNumber"
import { rollAllNumbers } from "~/lib/roll-signal"
import { useLiveChanges } from "~/lib/live-changes"
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
  const [mapConfig, setMapConfig] = useState<OverviewMapConfig | null>(null)
  const [updatedAt, setUpdatedAt] = useState(() => Date.now())
  const [now, setNow] = useState(() => Date.now())
  const [refreshing, setRefreshing] = useState(false)
  // Bumped by the refresh button: the live dots on the map appear again one by one.
  const [replay, setReplay] = useState(0)
  const [loading, setLoading] = useState(0)
  const loadingRef = useRef<Promise<void> | null>(null)

  // The figures and the globe, once at a time; the refresh icon turns while they load.
  const load = useCallback(() => loadingRef.current ??= (async () => {
    setLoading(n => n + 1)
    try {
      const [rows, figures] = await Promise.all([overviewLiveFn().catch(() => null), overviewChartsFn().catch(() => null)])
      if (rows) setPlaces(rows)
      if (figures) setCharts(figures)
      if (rows || figures) setUpdatedAt(Date.now())
      setNow(Date.now())
    } finally {
      setLoading(n => n - 1)
      loadingRef.current = null
    }
  })(), [])
  // Each loader refresh, by hand or by the minute timer, hands over a new list; the figures follow it.
  useEffect(() => { void load() }, [sites, load])
  // New visits are pushed by the server and only move the live dots, quietly: the figures wait for the minute.
  useLiveChanges(() => { void overviewLiveFn().then(setPlaces).catch(() => {}) })
  // Which map to draw, as the visitor map settings say; the built-in globe shows until it is known.
  useEffect(() => { void overviewMapFn().then(setMapConfig).catch(() => setMapConfig({ service: "" })) }, [])
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 5_000)
    return () => window.clearInterval(timer)
  }, [])

  const seconds = Math.floor(Math.max(0, now - updatedAt) / 1000)
  const updated = seconds < 10 ? t("刚刚更新") : seconds < 60 ? t("{n} 秒前更新").replace("{n}", String(Math.floor(seconds / 5) * 5)) : t("{n} 分钟前更新").replace("{n}", String(Math.floor(seconds / 60)))
  async function refresh() {
    setRefreshing(true)
    setReplay(n => n + 1)
    // Kept turning for a moment at least, so a fast reload still shows that it happened.
    try { await Promise.all([router.invalidate(), load(), new Promise(resolve => setTimeout(resolve, 600))]) } finally {
      setRefreshing(false)
      // Every figure on the page rolls over once the new ones are in.
      rollAllNumbers()
    }
  }
  const spinning = refreshing || loading > 0
  return (
    <section className="squircle overview-hero" aria-labelledby="overview-hero-title">
      <div className="overview-hero-head">
        <div className="overview-hero-heading">
          <span className="squircle overview-hero-badge" aria-hidden="true"><UIIcon name="globe" /></span>
          <h1 id="overview-hero-title" className="overview-hero-title">{t("nav.sites")}</h1>
          {/* Two quiet chips say what the figures cover; the green dot says the page updates by itself. */}
          <div className="overview-hero-meta">
            <span className="overview-hero-chip">{t("footer.sites").replace("{n}", numberShort(sites.length))}</span>
            <span className="overview-hero-chip"><UIIcon name="clock" />{t("近 24 小时")}</span>
            <span className="overview-hero-live">
              <i aria-hidden="true" />
              <span role="status">{updated}</span>
              <button type="button" className="overview-hero-refresh" disabled={refreshing} onClick={refresh} aria-label={t("刷新")} title={t("刷新")}>
                <UIIcon name="refresh" className={spinning ? "is-spinning" : undefined} />
              </button>
            </span>
          </div>
        </div>
        <div className="overview-hero-tools">
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
      <OverviewMap places={places} config={mapConfig} replay={replay} />
    </section>
  )
}

function Delta({ value }: { value: number }) {
  const { t } = useT()
  return (
    <span className={`overview-hero-delta${value > 0 ? " is-up" : value < 0 ? " is-down" : ""}`} title={t("与前一周期相比")}>
      {value > 0 ? <UIIcon name="arrow-up-right" /> : value < 0 ? <UIIcon name="arrow-down-right" /> : null}<RollingNumber text={`${Math.abs(value)}%`} />
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
      <div className="overview-hero-tiles">
        {tiles.map(tile => (
          <div key={tile.label} className={`overview-hero-tile${tile.delta === undefined ? " is-online" : ""}`}>
            <span className="label">{tile.delta === undefined ? <i /> : null}{tile.label}</span>
            <strong><RollingNumber text={numberShort(tile.value)} /></strong>
            {tile.delta === undefined ? <span className="overview-hero-note">{t("实时，5 分钟内")}</span> : <Delta value={tile.delta} />}
          </div>
        ))}
      </div>
    </>
  )
}
