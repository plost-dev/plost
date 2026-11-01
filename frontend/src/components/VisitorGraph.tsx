import { Surface } from "~/components/Surface"
import { UIIcon } from "~/components/UIIcon"
import { useT } from "~/lib/i18n"
import { TRAFFIC_METRICS, TrendChart } from "~/components/TrendChart"
import { formatDuration, numberShort } from "~/lib/format"
import { delta, type Interval } from "~/lib/range"
import type { MetricKey, Overview, SeriesBy } from "~/lib/ch"
import "~/styles/traffic-overview.css"

const SUMMARY_METRICS: ReadonlyArray<{
  key: MetricKey
  label: string
  format?: (value: number) => string
  invertChange?: boolean
}> = [
  ...TRAFFIC_METRICS,
  {key: "views_per_visit", label: "每次访问浏览量", format: value => value.toFixed(2)},
  {key: "bounce_rate", label: "跳出率", format: value => `${Math.round(value * 10) / 10}%`, invertChange: true},
  {key: "visit_duration", label: "访问时长", format: formatDuration},
]

export function VisitorGraph({
  overview,
  compare,
  seriesBy,
  interval = "day",
  realtime = false,
  onZoom,
}: {
  overview: Overview
  compare: Partial<Overview>
  seriesBy: SeriesBy
  interval?: Interval
  realtime?: boolean
  onZoom?: (date: string) => void
}) {
  const { t, locale } = useT()
  return (
    <Surface className="visitor-graph traffic-overview analytics-panel col-span-full">
      <dl className="metric-grid traffic-summary">
        {SUMMARY_METRICS.map(({ key, label, format, invertChange }) => {
          const value = overview[key]
          const change = delta(value, compare[key])
          const improved = change && (invertChange ? change.diff < 0 : change.diff > 0)
          const text = format ? format(value) : numberShort(value)
          const plotted = TRAFFIC_METRICS.some(metric => metric.key === key)
          return <div key={key} className="metric-item traffic-summary-item" data-metric={key}>
            <dt className="traffic-summary-label">{plotted ? <span className="traffic-series-dot" aria-hidden="true" /> : null}{t(label)}</dt>
            <dd className="metric-value-row traffic-summary-values">
              <p className="metric-value" title={format ? text : value.toLocaleString(locale)}>{text}</p>
              {change ? <span className="metric-change">
                {change.diff !== 0 ? <UIIcon name={change.diff > 0 ? "arrow-up-right" : "arrow-down-right"} className={`mb-0.5 mr-0.5 inline-block size-2.5 ${improved ? "text-green-500" : "text-red-500"}`} /> : null}
                {Math.abs(change.pct)}%
              </span> : null}
            </dd>
          </div>
        })}
      </dl>
      <div className="trend-section">
        <div className="analytics-heading"><h3>{t("访问趋势")}</h3></div>
        <TrendChart series={{visitors: seriesBy.visitors, visits: seriesBy.visits, pageviews: seriesBy.pageviews}} interval={interval} realtime={realtime} onZoom={onZoom} />
      </div>
    </Surface>
  )
}
