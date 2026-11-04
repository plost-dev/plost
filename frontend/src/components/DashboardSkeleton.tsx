import type { CSSProperties } from "react"
import { DashTopBar } from "./DashTopBar"
import { Surface } from "./Surface"
import { useT } from "~/lib/i18n"
import "~/styles/traffic-overview.css"
import { parseDashSearch, type DashSearch } from "~/lib/range"

const ROWS = 8

function SkeletonRows({ count = ROWS, more = false }: { count?: number; more?: boolean }) {
  return <div className="skeleton-rows">
    {Array.from({ length: count }, (_, index) => <div key={index} className="skeleton-row" style={{ "--skeleton-delay": `${index * 70}ms` } as CSSProperties}>
      <span className="skeleton skeleton-line skeleton-label" />
      <span className="skeleton skeleton-line skeleton-value" />
    </div>)}
    {more ? <span className="skeleton squircle skeleton-more" /> : null}
  </div>
}

function SkeletonReport({ className = "", tabs = 3, rows, more }: { className?: string; tabs?: number; rows?: number; more?: boolean }) {
  return <Surface as="section" aria-hidden="true" className={`report-card relative flex min-w-0 w-full flex-col bg-white ${className}`}>
    <div className="report-heading flex w-full justify-between border-b border-gray-200">
      <div className="skeleton-tabs">{Array.from({ length: tabs }, (_, index) => <span key={index} className="skeleton skeleton-line skeleton-tab" />)}</div>
    </div>
    <SkeletonRows count={rows} more={more} />
  </Surface>
}

/**
 * Stands in for AnalyticsDashboard while its data loads. The toolbar is real, so the period and
 * filters work immediately; every card keeps the size of its loaded form to avoid layout jumps.
 */
export function AnalyticsDashboardSkeleton({ domain, search, onSearch, readonly = true, canManage = false }: {
  /** Empty when the site is only known once the data arrives (shared reports). */
  domain: string
  search: DashSearch
  onSearch: (search: DashSearch) => void
  readonly?: boolean
  canManage?: boolean
}) {
  const { t } = useT()
  return (
    <div className="analytics-dashboard dashboard-skeleton traffic-overview-skeleton grid min-w-0 grid-cols-1 md:grid-cols-2" aria-busy="true">
      {domain ? <DashTopBar
        domain={domain}
        search={search}
        onPeriod={next => onSearch(parseDashSearch({ ...search, ...next }))}
        readonly={readonly}
        canManage={canManage}
      /> : <div className="dash-toolbar col-span-full flex items-center py-1" aria-hidden="true"><span className="skeleton squircle skeleton-identity" /></div>}
      <p className="sr-only" role="status">{t("加载中…")}</p>

      <Surface className="visitor-graph analytics-panel col-span-full" aria-hidden="true">
        <div className="metric-grid">
          {Array.from({ length: 6 }, (_, index) => <span key={index} className="skeleton squircle metric-skeleton" style={{ "--skeleton-delay": `${index * 90}ms` } as CSSProperties} />)}
        </div>
        <div className="trend-section">
          <div className="analytics-heading"><span className="skeleton skeleton-line skeleton-heading" /></div>
          <span className="skeleton squircle trend-skeleton" />
        </div>
      </Surface>

      <section className="geography-section col-span-full" aria-hidden="true">
        <div className="geography-grid geography-with-ranking">
          <Surface className="geography-map analytics-panel">
            <div className="analytics-heading geography-map-heading">
              <span className="skeleton skeleton-line skeleton-heading" />
              <span className="skeleton squircle skeleton-count" />
              <span className="skeleton squircle skeleton-toggle" />
            </div>
            <span className="skeleton squircle map-skeleton" />
            <span className="skeleton skeleton-line map-legend-skeleton" />
          </Surface>
        </div>
      </section>

      <SkeletonReport className="source-report" tabs={4} more />
      <SkeletonReport tabs={5} more />
      <SkeletonReport className="location-report" tabs={3} more />
      <SkeletonReport className="device-report traffic-paired-report" tabs={6} />
    </div>
  )
}
