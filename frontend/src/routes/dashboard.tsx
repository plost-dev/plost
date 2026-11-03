import { UIIcon } from "~/components/UIIcon"
import { SiteSparkline } from "~/components/SiteSparkline"
import { Surface } from "~/components/Surface"
import { ActionLink } from "~/components/ActionButton"
import { SiteFavicon } from "~/components/SiteFavicon"
import { redirect, Link, createFileRoute, useRouter } from "@tanstack/react-router"
import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react"
import { Shell } from "~/components/Shell"
import { meFn, overviewLiveFn, sitesFn } from "~/lib/actions"
import { OverviewGlobe, type OverviewPlace } from "~/components/OverviewGlobe"
import { numberShort } from "~/lib/format"
import { useT } from "~/lib/i18n"
import { readSiteLayout, setSiteLayout, subscribeSiteLayout, type SiteLayout } from "~/lib/site-layout-preference"
import { useRouterAutoRefresh } from "~/lib/router-refresh"
import { expiryState } from "~/lib/site-check-view"

export const Route = createFileRoute("/dashboard")({
  loader: async () => {
    const me = await meFn()
    if (!me) throw redirect({ to: "/login" })
    return { me, sites: await sitesFn() }
  },
  component: DashboardPage,
})

function DashboardPage() {
  const { me, sites } = Route.useLoaderData()
  const { t } = useT()
  const [q, setQ] = useState("")
  const layout = useSyncExternalStore(subscribeSiteLayout, readSiteLayout, (): SiteLayout => "compact")
  const [layoutSaveFailed, setLayoutSaveFailed] = useState(false)
  useRouterAutoRefresh()
  const now = new Date().toISOString()
  function changeLayout(value: SiteLayout) {
    setLayoutSaveFailed(!setSiteLayout(value))
  }
  const [searching, setSearching] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key !== "/") return
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return
      e.preventDefault()
      searchRef.current?.focus()
      searchRef.current?.select()
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [])

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    if (!s) return sites
    return sites.filter((site) => site.domain.toLowerCase().includes(s) || (site.name && site.name.toLowerCase().includes(s)))
  }, [q, sites])

  return (
    <Shell user={me} headerSearch={
      <div className="management-search relative">
        <div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 text-gray-800">
          <UIIcon name="search" className="size-4" />
        </div>
        <input
          ref={searchRef}
          className="squircle input pl-9"
          aria-label={t("搜索站点")}
          placeholder={searching ? t("搜索站点") : t("按 / 搜索")}
          value={q}
          onFocus={() => setSearching(true)}
          onBlur={() => setSearching(false)}
          onChange={(e) => setQ(e.target.value)}
        />
      </div>
    }>
      <div>
        <OverviewHero sites={sites} layout={layout} onLayout={changeLayout} />
        {layoutSaveFailed ? <p className="text-xs text-gray-500" role="status">{t("已切换，当前浏览器无法保存显示方式。")}</p> : null}

        {q && filtered.length === 0 ? (
          <p className="mt-4 text-center text-gray-900">{t("没有找到站点，换个关键词试试。")}</p>
        ) : null}

        {sites.length === 0 ? (
          <div className="mx-auto flex max-w-md flex-col items-center justify-center py-12">
            <h3 className="text-center text-base font-medium text-gray-900">{t("添加第一个个人站点")}</h3>
            <p className="mt-1 text-center text-sm text-pretty text-gray-500">{t("用简洁、保护隐私的统计，更好地了解你的受众。")}</p>
            <Link to="/sites/new" className="squircle btn btn-primary mt-6">
              {t("添加网站")}</Link>
          </div>
        ) : (
          <ul className="squircle management-site-list">
            {filtered.map((site) => (
              <li key={site.id} className="squircle management-site-item group relative">
                <Link to="/sites/$domain" params={{ domain: site.domain }} className="block">
                  <Surface className="management-site-row">
                    <div className="management-site-identity flex w-full items-center gap-x-2.5">
                      <SiteFavicon domain={site.domain} className="size-[18px]" />
                      <div className="management-site-name min-w-0 flex-1">
                        <div className="management-site-heading">
                          <h3 title={site.name || site.domain} className="management-site-title text-base leading-6 font-semibold text-gray-900">{site.name || site.domain}</h3>
                          <p className={`management-site-ssl${{warning: " is-expiring", bad: " is-expired", unknown: " is-unknown"}[expiryState(site.certificateExpiresAt, now).tone as string] ?? ""}`}><UIIcon name="lock"/>SSL {expiryState(site.certificateExpiresAt, now).days ?? "—"}{expiryState(site.certificateExpiresAt, now).days === null ? "" : ` ${t("天")}`}</p>
                        </div>
                      </div>
                    </div>
                    <div className="management-site-data">
                      {layout === "compact" ? <CompactMetrics site={site}/> : <>
                        <span className="h-12 max-w-sm text-indigo-500 sm:max-w-none">
                          <SiteSparkline values={site.sparkline || []} />
                        </span>
                        <div className="flex items-center justify-between max-lg:col-span-full">
                          <div className="flex flex-col">
                            <p className="text-lg font-bold text-gray-900 sm:text-xl">{numberShort(site.visitors || 0)}</p>
                            <p className="text-gray-600">{site.visitors === 1 ? t("位访客（近 24 小时）") : t("位访客（近 24 小时）")}</p>
                          </div>
                          <div className="management-site-trend">
                            <p className="management-site-online"><i/>{t("当前在线")} {site.live}</p>
                            <p className="text-sm text-gray-900">
                              {site.change > 0 ? (
                                <UIIcon name="arrow-up-right" className="mr-0.5 inline-block h-3 w-3 text-green-500" />
                              ) : site.change < 0 ? (
                                <UIIcon name="arrow-down-right" className="mr-0.5 inline-block h-3 w-3 text-red-500" />
                              ) : null}
                              {Math.abs(site.change || 0)}%
                            </p>
                          </div>
                        </div>
                      </>}
                    </div>
                  </Surface>
                </Link>
                <div className="absolute top-2 right-2">
                  <ActionLink to="/sites/$domain/settings" params={{ domain: site.domain }} search={{ tab: "general" }}
                    className="btn-ghost site-card-menu-trigger" aria-label={`${t("设置")} ${site.domain}`} title={t("设置")}>
                    <UIIcon name="settings" className="size-[18px]" />
                  </ActionLink>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Shell>
  )
}

type OverviewSite = Awaited<ReturnType<typeof sitesFn>>[number]

function OverviewHero({ sites, layout, onLayout }: { sites: OverviewSite[], layout: SiteLayout, onLayout: (value: SiteLayout) => void }) {
  const { t } = useT()
  const router = useRouter()
  const [places, setPlaces] = useState<OverviewPlace[]>([])
  const [updatedAt, setUpdatedAt] = useState(() => Date.now())
  const [now, setNow] = useState(() => Date.now())
  const [refreshing, setRefreshing] = useState(false)
  const visitors = sites.reduce((sum, site) => sum + (site.visitors || 0), 0)

  // Each loader refresh hands over a new list; the globe follows it.
  useEffect(() => {
    let alive = true
    setUpdatedAt(Date.now())
    void overviewLiveFn().then((rows) => { if (alive) setPlaces(rows) }).catch(() => {})
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
      <div className="overview-hero-body">
        <div>
          <h1 id="overview-hero-title" className="overview-hero-title">{t("nav.sites")}</h1>
          <p className="overview-hero-sub">{t("footer.sites").replace("{n}", numberShort(sites.length))}</p>
        </div>
        <div className="overview-hero-metric">
          <strong>{numberShort(visitors)}</strong>
          <span>{t("位访客（近 24 小时，全部站点）")}</span>
        </div>
        <div className="overview-hero-actions management-site-tools">
          <Link to="/sites/new" className="squircle btn btn-primary" aria-label={t("添加网站")} title={t("添加网站")}>
            <UIIcon name="plus" className="size-4" />
            <span className="management-tool-label">{t("添加网站")}</span>
          </Link>
          <div className="squircle site-layout-switch" role="group" aria-label={t("站点显示方式")}>
            <span className="squircle site-layout-thumb" aria-hidden="true"/>
            <button className="squircle btn" data-layout="grid" type="button" aria-label={t("卡片")} title={t("卡片")} aria-pressed={layout === "grid"} onClick={() => onLayout("grid")}>
              <UIIcon name="grid" className="size-4" />
              <span className="management-tool-label">{t("卡片")}</span>
            </button>
            <button className="squircle btn" data-layout="compact" type="button" aria-label={t("紧凑")} title={t("紧凑")} aria-pressed={layout === "compact"} onClick={() => onLayout("compact")}>
              <UIIcon name="list" className="size-4" />
              <span className="management-tool-label">{t("紧凑")}</span>
            </button>
          </div>
          <button type="button" className="overview-hero-refresh" disabled={refreshing} onClick={refresh} aria-label={t("刷新")} title={t("刷新")}>
            <UIIcon name="refresh" className={refreshing ? "is-spinning" : undefined} />
            <span role="status">{minutes < 1 ? t("刚刚更新") : t("{n} 分钟前更新").replace("{n}", String(minutes))}</span>
          </button>
        </div>
      </div>
      <OverviewGlobe places={places} />
    </section>
  )
}

export function CompactMetrics({site}: {site: OverviewSite}) {
  const {t} = useT()
  const metrics: Array<[string, number]> = [
    [t("访客"), site.visitors],
    [t("访问次数"), site.visits],
    [t("浏览量"), site.pageviews],
    [t("当前在线"), site.live],
  ]
  return <>
    <span className="management-compact-spark"><SiteSparkline values={site.sparkline}/></span>
    <div className="management-compact-metrics">{metrics.map(([label,value]) => <div key={label} className={label === t("当前在线") ? "is-online" : ""}><strong>{numberShort(value)}</strong><span>{label}</span></div>)}</div>
  </>
}

