import { Surface } from "./Surface"
import { UIIcon } from "./UIIcon"
import { useT } from "~/lib/i18n"
import { expiryState } from "~/lib/site-check-view"
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
    {error ? <p className="health-note">{t(error)}</p> : null}
    {stale ? <p className="health-note">{t("本次查询失败，显示上次结果")}{lastSuccessAt ? ` · ${t("上次获取")}: ${formatTimestamp(lastSuccessAt, locale)}` : ""}</p> : null}
  </Surface>
}

function uptimeTone(percent: number | null) {
  return percent === null ? "unknown" : percent >= 99.9 ? "good" : percent >= 95 ? "warning" : "bad"
}

/** A full-width row of the last 90 days, oldest on the left; each square is one calendar day in the site's timezone. */
export function SiteUptimeStrip({report}: {report: SiteCheckReport}) {
  const {t} = useT()
  const days = report.uptime?.days || []
  if (!days.length) return null
  const total = days.reduce((sum, day) => sum + day.total, 0)
  const failed = days.reduce((sum, day) => sum + day.failed, 0)
  const overall = total ? (total - failed) / total * 100 : null
  const percent = (value: number) => `${value >= 99.995 ? "100" : value.toFixed(2)}%`
  return <Surface className="health-uptime col-span-full">
    <div className="health-uptime-heading">
      <div><h2>{t("近 90 天在线")}</h2><p>{t("每 5 分钟检查一次 HTTP 状态，按天汇总")}</p></div>
      <strong>{overall === null ? "—" : percent(overall)}</strong>
    </div>
    <div className="health-uptime-days" role="list" aria-label={t("近 90 天在线")}>
      {days.map(day => <span key={day.date} role="listitem" className={`squircle health-tone-${uptimeTone(day.percent)}`}
        title={`${day.date} · ${day.percent === null ? t("未检查") : `${percent(day.percent)} · ${t("{0} 次检查，{1} 次失败", {0: day.total, 1: day.failed})}`}`}/>)}
    </div>
    <div className="health-chart-legend"><span><i className="squircle health-tone-good"/>{t("正常")}</span><span><i className="squircle health-tone-warning"/>{t("部分异常")}</span><span><i className="squircle health-tone-bad"/>{t("异常")}</span><span><i className="squircle health-tone-unknown"/>{t("未检查")}</span><span className="health-check-count">{t("共 {0} 次检查", {0: total})}</span></div>
  </Surface>
}

/** Domain and certificate expiry, shown in the site's settings. */
export function SiteInfoPanel({report}: {report: SiteCheckReport}) {
  return <div className="health-cards health-cards-two">
    <ExpiryCard kind="domain" {...report.expiry?.domain} date={report.expiry?.domain.expiresAt} now={report.now}/>
    <ExpiryCard kind="certificate" {...report.expiry?.certificate} date={report.expiry?.certificate.expiresAt} now={report.now}/>
  </div>
}

/** who.ga fills in domain expiry when the registry has none, so the settings heading credits it. */
export function WhoGaCredit() {
  const {t} = useT()
  return <a className="settings-heading-link" href="https://who.ga" target="_blank" rel="noreferrer">{t("域名查询由 {0} 赞助", {0: "who.ga"})}</a>
}
