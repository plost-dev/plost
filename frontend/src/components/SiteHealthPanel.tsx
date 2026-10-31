import { useEffect, useId, useRef, useState } from "react"
import { Surface } from "./Surface"
import { UIIcon } from "./UIIcon"
import { useT } from "~/lib/i18n"
import { checkSummary, expiryState, responseSegments } from "~/lib/site-check-view"
import { formatTimestamp } from "~/lib/format"
import type { SiteCheckReport } from "~/lib/site-checks"
import "~/styles/site-health.css"

function ExpiryCard({kind, date, error, stale, lastSuccessAt, now, registeredAt, registrar, validation}: {
  kind: "domain" | "certificate"; date?: string | null; error?: string | null; stale?: boolean; lastSuccessAt?: string; now: string;
  registeredAt?: string | null; registrar?: string | null; validation?: "DV" | "OV" | "EV" | null
}) {
  const {t, locale} = useT()
  const state = expiryState(date, now)
  const dateOnly = !!date && /^\d{4}-\d{2}-\d{2}$/.test(date)
  const tone = stale ? state.tone === "bad" ? "bad" : "warning" : error && kind === "certificate" ? "bad" : state.tone
  const formatDate = (value?: string | null) => value ? new Intl.DateTimeFormat(locale === "en" ? "en-GB" : "zh-CN", {timeZone: "UTC", year: "numeric", month: "short", day: "numeric"}).format(new Date(value)) : "—"
  const validationLabel = validation === "DV" ? t("域名验证") : validation === "OV" ? t("组织验证") : validation === "EV" ? t("扩展验证") : ""
  return <Surface className={`health-card health-tone-${tone}`}>
    <div className="health-card-heading"><span>{t(kind === "domain" ? "域名到期" : "证书到期")}</span><UIIcon name={kind === "domain" ? "globe" : "shield"}/></div>
    <div className="health-card-value">{state.days === null ? "—" : state.days}<small>{t("天")}</small></div>
    <div className="health-card-details">
      {kind === "domain" ? <>
        <p><span>{t("注册日期")}</span><span>{formatDate(registeredAt)}</span></p>
        <p><span>{t("到期日期")}</span><span>{formatDate(date)}{date && !dateOnly ? " UTC" : ""}</span></p>
        <p title={registrar || undefined}><span>{t("注册商")}</span><span>{registrar || "—"}</span></p>
      </> : <>
        <p><span>{t("证书类型")}</span><span>{validation ? `${validation} · ${validationLabel}` : t("未知")}</span></p>
        <p><span>{t("到期日期")}</span><span>{formatDate(date)}{date && !dateOnly ? " UTC" : ""}</span></p>
      </>}
    </div>
    {state.days === null ? <p className="health-note">{t(error ? "未能获取到期信息" : "尚无到期信息")}</p> : state.tone === "bad" ? <p className="health-note">{t("已到期")}</p> : null}
    {dateOnly ? <p className="health-note">{t("按公开日期估算剩余天数")}</p> : null}
    {error ? <p className="health-note">{t(error)}</p> : null}
    {stale ? <p className="health-note">{t("本次查询失败，显示上次结果")}{lastSuccessAt ? ` · ${t("上次获取")}: ${formatTimestamp(lastSuccessAt, locale)}` : ""}</p> : null}
  </Surface>
}

function ResponseChart({report}: {report: SiteCheckReport}) {
  const {t, locale} = useT()
  const id = useId()
  const chart = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(320)
  useEffect(() => {
    const element = chart.current
    if (!element) return
    const measure = () => setWidth(Math.max(160, element.clientWidth || 320))
    measure()
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", measure)
      return () => window.removeEventListener("resize", measure)
    }
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  const [selected, setSelected] = useState<number | null>(null)
  const summary = checkSummary(report.checks, report.now)
  const W = width, H = 220, right = 8, top = 22, bottom = 32
  const end = Date.parse(report.now), start = end - 86400_000
  const high = Math.max(100, ...report.checks.map(check => check.durationMs || 0))
  const max = Math.ceil(high / 100) * 100
  const left = Math.max(42, String(max).length * 7 + 12)
  const x = (time: string) => left + (Date.parse(time) - start) / 86400_000 * (W - left - right)
  const y = (value: number) => top + (1 - value / max) * (H - top - bottom)
  const segments = responseSegments(report.checks)
  const point = selected === null ? null : report.checks[selected]
  const dateLabel = (date: string) => new Intl.DateTimeFormat(locale === "en" ? "en-GB" : "zh-CN", {timeZone: "UTC", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23"}).format(new Date(date)) + " UTC"
  function selectPoint(event: React.PointerEvent<HTMLDivElement>) {
    if (!report.checks.length) return
    const bounds = event.currentTarget.getBoundingClientRect()
    if (!bounds.width) return
    const position = (event.clientX - bounds.left) / bounds.width * W
    let nearest = 0
    report.checks.forEach((check, index) => {
      if (Math.abs(x(check.checkedAt) - position) < Math.abs(x(report.checks[nearest].checkedAt) - position)) nearest = index
    })
    setSelected(nearest)
  }
  return <Surface className="health-chart analytics-panel">
    <div className="health-chart-heading"><div><h2>{t("网站响应")}</h2><p>{t("最近 24 小时 · 每 5 分钟检查")}</p></div>
      <div className="health-chart-stats"><span>{t("检查成功率")}<strong>{summary.rate === null ? "—" : `${summary.rate.toFixed(1)}%`}</strong></span><span>{t("响应中位数")}<strong>{summary.median === null ? "—" : `${summary.median} ms`}</strong></span></div>
    </div>
    <div ref={chart} className="health-response-chart" tabIndex={report.checks.length ? 0 : undefined} role="group" aria-label={t("响应时间图，使用左右方向键查看")}
      onKeyDown={event => {if (report.checks.length && ["ArrowLeft", "ArrowRight"].includes(event.key)) {event.preventDefault(); setSelected(index => Math.min(report.checks.length - 1, Math.max(0, (index ?? report.checks.length - 1) + (event.key === "ArrowRight" ? 1 : -1))))} else if (event.key === "Escape") setSelected(null)}}
      onPointerDown={selectPoint} onPointerMove={selectPoint} onPointerLeave={event => {if (event.pointerType !== "touch") setSelected(null)}}>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={t("HTTP 响应时间，单位毫秒")}>
        <defs><linearGradient id={id} x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stopColor="var(--health-accent)" stopOpacity=".16"/><stop offset="100%" stopColor="var(--health-accent)" stopOpacity=".01"/></linearGradient></defs>
        {[0, .5, 1].map(ratio => <g key={ratio}><line x1={left} x2={W - right} y1={y(max * ratio)} y2={y(max * ratio)} className="health-grid-line"/><text x={left - 10} y={y(max * ratio) + 4} textAnchor="end">{Math.round(max * ratio)}</text></g>)}
        {segments.map((segment, index) => {
          const path = segment.map((check, i) => `${i ? "L" : "M"}${x(check.checkedAt)},${y(check.durationMs!)}`).join(" ")
          return <g key={index}><path d={`${path} L${x(segment.at(-1)!.checkedAt)},${y(0)} L${x(segment[0].checkedAt)},${y(0)}Z`} fill={`url(#${id})`}/><path d={path} fill="none" stroke="var(--health-accent)" strokeWidth="2.5" strokeLinejoin="round" strokeLinecap="round"/>{segment.length === 1 ? <circle cx={x(segment[0].checkedAt)} cy={y(segment[0].durationMs!)} r="3" fill="var(--health-accent)"/> : null}</g>
        })}
        {report.checks.filter(check => !check.ok).map((check, index) => <circle key={index} cx={x(check.checkedAt)} cy={y(0)} r="3" fill="var(--health-bad)"/>)}
        {(width < 500 ? [0, 12, 24] : [0, 6, 12, 18, 24]).map(hour => <text key={hour} x={left + hour / 24 * (W - left - right)} y={H - 7} textAnchor={hour === 0 ? "start" : hour === 24 ? "end" : "middle"}>{hour === 24 ? t("现在") : t("{0} 小时前", {0: 24 - hour})}</text>)}
        {point ? <line x1={x(point.checkedAt)} x2={x(point.checkedAt)} y1={top} y2={y(0)} className="health-cursor"/> : null}
      </svg>
      {!report.checks.length ? <div className="health-chart-empty">{t(report.enabled ? "等待首次检查" : "开启后开始记录网站状态")}</div> : null}
      <div className="health-chart-caption" aria-live="polite">{point ? `${dateLabel(point.checkedAt)} · ${point.status ? `HTTP ${point.status}` : t(point.error || "检查失败")} · ${point.durationMs === null ? "—" : `${point.durationMs} ms`}` : t("悬停或轻触查看详情，空白表示没有检查记录")}</div>
    </div>
    <div className="health-timeline" aria-label={t("最近 24 小时的检查状态")}>{summary.buckets.map(bucket => <span key={bucket.from} className={`squircle health-tone-${bucket.tone}`} title={`${dateLabel(bucket.from)} · ${bucket.total ? t("{0} 次检查，{1} 次失败", {0: bucket.total, 1: bucket.failed}) : t("未检查")}`}/>)}</div>
    <div className="health-chart-legend"><span><i className="squircle health-tone-good"/>{t("正常")}</span><span><i className="squircle health-tone-bad"/>{t("异常")}</span><span><i className="squircle health-tone-unknown"/>{t("未检查")}</span><span className="health-check-count">{t("共 {0} 次检查", {0: summary.total})}</span></div>
  </Surface>
}

export function SiteHealthCards({report}: {report: SiteCheckReport}) {
  const {t} = useT()
  const latest = report.latest
  const stale = !latest || Date.parse(report.now) - Date.parse(latest.checkedAt) > 10 * 60_000
  const state = !report.enabled ? "paused" : stale ? "unknown" : latest.ok ? "good" : "bad"
  return <div className="health-cards">
    <Surface className={`health-card health-tone-${state}`}><div className="health-card-heading"><span>{t("网站可用性")}</span><UIIcon name="activity"/></div>
      <div className="health-status"><span className="health-dot"/>{t(state === "paused" ? "已暂停" : state === "unknown" ? "等待检查" : state === "good" ? "正常访问" : "访问异常")}</div>
      <p>{latest && !stale ? latest.error ? t(latest.error) : `HTTP ${latest.status}` : t("以最新检查结果为准")}</p>
      <div className="health-card-foot"><span>{latest && !stale && latest.durationMs !== null ? `${latest.durationMs} ms` : "—"}</span><span>{t("预期 HTTP 200")}</span></div>
    </Surface>
    <ExpiryCard kind="certificate" {...report.expiry?.certificate} date={report.expiry?.certificate.expiresAt} now={report.now}/>
    <ExpiryCard kind="domain" {...report.expiry?.domain} date={report.expiry?.domain.expiresAt} now={report.now}/>
  </div>
}

export function SiteHealthPanel({report}: {report: SiteCheckReport}) {
  const {t, locale} = useT()
  const latest = report.latest
  return <div className="site-health">
    <SiteHealthCards report={report}/>
    <ResponseChart report={report}/>
    <div className="health-footer"><span>{t("最近检查")}: {latest ? formatTimestamp(latest.checkedAt, locale) : "—"}</span><span>{t("到期信息每日更新，临时查询失败后 15 分钟重试")}{report.expiry ? ` · ${formatTimestamp(report.expiry.checkedAt, locale)}` : ""}</span></div>
    {report.expiry?.domain.domain ? <p className="health-registration-note">{t("域名到期信息对应 {0}", {0: report.expiry.domain.domain})}</p> : null}
  </div>
}
