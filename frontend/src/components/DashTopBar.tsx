import { isCalendarDate, isDateRange } from "~/lib/date-time"
import { Link } from "@tanstack/react-router"
import { UIIcon } from "~/components/UIIcon"
import { useT } from "~/lib/i18n"
import { useEffect, useRef, useState, type ReactNode } from "react"
import { SiteSwitcher } from "~/components/SiteSwitcher"
import { ReportFilter } from "./ReportFilter"
import { expiryState } from "~/lib/site-check-view"
import type { SiteCheckReport } from "~/lib/site-checks"
import { INTERVAL_LABEL, PERIOD_LABEL, availableIntervals, intervalStorageKey, periodLabel, type DashSearch, type Interval, type Period } from "~/lib/range"

const GROUPS: Period[][] = [
  ["today", "yesterday", "realtime"],
  ["24h", "7d", "28d", "91d"],
  ["month", "last_month"],
  ["year", "12mo"],
  ["all", "custom"],
]

const HINT: Partial<Record<Period, string>> = {
  today: "D",
  yesterday: "E",
  realtime: "R",
  "24h": "H",
  "7d": "W",
  "28d": "F",
  "91d": "N",
  month: "M",
  last_month: "P",
  year: "Y",
  "12mo": "L",
  all: "A",
  custom: "C",
}

const FILTERS: Array<{ key: keyof DashSearch, label: string }> = [
  { key: "page", label: "页面" },
  { key: "source", label: "来源" },
  { key: "country", label: "国家/地区" },
  { key: "device", label: "设备" },
  { key: "browser", label: "浏览器" },
  { key: "os", label: "操作系统" },
]

export function DashTopBar({
  domain,
  filters,
  search,
  onPeriod,
  readonly,
  canManage = !readonly,
  onExport,
  filterOptions = {},
  health = null,
}: {
  domain: string
  filters?: ReactNode
  search: DashSearch
  onPeriod: (next: Partial<DashSearch>) => void
  readonly?: boolean
  canManage?: boolean
  filterOptions?: Partial<Record<keyof DashSearch, Array<{name:string;label?:string}>>>
  onExport?: () => void
  health?: SiteCheckReport | null
}) {
  const { t } = useT()
  const [open, setOpen] = useState<"filter" | "period" | "more" | null>(null)
  const [custom, setCustom] = useState(false)
  const [customFrom, setCustomFrom] = useState("")
  const [customTo, setCustomTo] = useState("")
  useEffect(() => {
    if(custom) {setCustomFrom(isCalendarDate(search.from) ? search.from : "");setCustomTo(isCalendarDate(search.to) ? search.to : "")}
  }, [custom])
  const [filterKey, setFilterKey] = useState<keyof DashSearch>("source")
  const box = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (!box.current?.contains(e.target as Node)) {
        setOpen(null)
        setCustom(false)
      }
    }
    window.addEventListener("mousedown", onClick)
    return () => window.removeEventListener("mousedown", onClick)
  }, [])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.isComposing || e.keyCode === 229 || document.querySelector('dialog[open], [aria-modal="true"]')) return
      if (e.key === "Escape") {
        setOpen(null)
        setCustom(false)
        return
      }
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || (e.target as HTMLElement)?.isContentEditable) return
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (e.key.toLowerCase() === "i" && search.period !== "realtime") {
        const options = availableIntervals(search.period, { from: search.from, to: search.to })
        if (options.length > 1) {
          e.preventDefault()
          const idx = options.indexOf(search.interval)
          const next = options[(idx >= 0 ? idx + 1 : 1) % options.length]
          localStorage.setItem(intervalStorageKey(domain, search.period), next)
          onPeriod({ interval: next })
        }
        return
      }
      const hit = (Object.entries(HINT) as Array<[Period, string]>).find(([, k]) => k === e.key.toUpperCase())
      if (!hit) return
      e.preventDefault()
      if (hit[0] === "custom") {
        setOpen("period")
        setCustom(true)
        return
      }
      onPeriod({ period: hit[0] })
      setOpen(null)
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [onPeriod, search.period, search.interval, search.from, search.to, domain])

  const label = periodLabel(search.period, { from: search.from, to: search.to })
  const latest = health?.latest
  const fresh = !!latest && Date.parse(health!.now) - Date.parse(latest.checkedAt) <= 10 * 60_000
  const healthState = !health?.enabled || !fresh ? "unknown" : latest?.ok ? "good" : "bad"
  const httpLabel = !health?.enabled ? t("已暂停") : fresh ? `HTTP ${latest?.status ?? "—"}` : t("等待检查")
  const certificateDays = health?.expiry ? expiryState(health.expiry.certificate.expiresAt, health.now).days : null

  return (
    <div className="dash-toolbar col-span-full flex w-full min-w-0 flex-wrap items-center gap-x-1 gap-y-2 py-1 md:gap-x-2.5">
      <div className="dash-identity">
        <SiteSwitcher domain={domain} readonly={readonly} canManage={canManage} />
        {filters}
        {!readonly && health ? <Link to="/sites/$domain/health" params={{domain}} className="squircle dashboard-health-inline" title={t("站点检查")}>
          <span className={`health-inline-state is-${healthState}`}><i/>{httpLabel}</span>
          <span>SSL {certificateDays ?? "—"}{certificateDays === null ? "" : ` ${t("天")}`}</span>
        </Link> : null}
      </div>
      <div className="dash-actions relative ml-auto flex shrink-0 items-center gap-x-1 md:gap-x-2.5" ref={box}>
        {!readonly ? <Link to="/sites/$domain/health" params={{domain}} className="squircle activity-entry">{t("站点检查")}</Link> : null}
        {!readonly ? <Link to="/sites/$domain/activity" params={{domain}} search={{window:"24h"}} className="squircle activity-entry">{t("活动日志")}</Link> : null}
        <div>
          <button
            type="button"
            className="squircle control-trigger flex items-center gap-x-1.5 rounded-md px-2.5 text-sm font-medium text-gray-700 hover:bg-gray-150/80"
            aria-expanded={open === "filter"}
            onClick={() => setOpen(open === "filter" ? null : "filter")}
          >
            
            <span>{t("筛选")}</span>
            <UIIcon name="chevron-down" className="ui-chevron" />
          </button>
          {open === "filter" ? (
            <div className="squircle dashboard-popover absolute right-0 z-20 mt-2 w-72 origin-top-right rounded-md bg-white p-1 font-medium text-gray-800 shadow-lg ring-1 ring-black/5">
              <div className="grid grid-cols-2 gap-0.5 p-1">
                {FILTERS.map((f) => (
                  <button
                    key={f.key}
                    type="button"
                    aria-pressed={filterKey === f.key}
                    data-selected={filterKey === f.key}
                    className="squircle control-menu-item rounded-md px-3 py-2 text-left text-sm hover:bg-gray-100"
                    onClick={() => setFilterKey(f.key)}
                  >
                    {t(f.label)}
                  </button>
                ))}
              </div>
              <p className="px-3 pt-1 text-xs text-gray-500">{t("精确匹配；可选择已有值或输入完整值。")}</p>
              <ReportFilter key={filterKey} kind={filterKey} options={filterOptions[filterKey] || []} initialValue={String(search[filterKey] || "")} onApply={name=>{onPeriod({[filterKey]:name});setOpen(null)}}/>
            </div>
          ) : null}
        </div>

        <div className="relative">
          <button
            type="button"
            className="squircle control-trigger flex items-center gap-x-1.5 rounded-md px-2.5 text-sm font-medium text-gray-700 hover:bg-gray-150/80"
            aria-expanded={open === "period"}
            onClick={() => { setOpen(open === "period" ? null : "period"); setCustom(false) }}
          >
            
            <span className="truncate">{t(label || "")}</span>
            <UIIcon name="chevron-down" className="ui-chevron" />
          </button>
          {open === "period" ? (
            <div className="squircle dashboard-popover absolute right-0 z-20 mt-2 w-56 origin-top-right rounded-md bg-white p-1 font-medium text-gray-800 shadow-lg ring-1 ring-black/5">
              {custom ? (
                <form className="space-y-2 p-2" onSubmit={event=>{event.preventDefault();if(!isDateRange(customFrom,customTo))return;onPeriod({period:"custom",from:customFrom,to:customTo});setOpen(null);setCustom(false)}}>
                  <label className="block text-xs">{t("开始日期")}<input required aria-label={t("开始日期")} className="squircle input mt-1" type="date" value={customFrom} onInput={e=>setCustomFrom(e.currentTarget.value)} onChange={e=>setCustomFrom(e.target.value)} /></label>
                  <label className="block text-xs">{t("结束日期")}<input required aria-label={t("结束日期")} className="squircle input mt-1" type="date" min={customFrom || undefined} value={customTo} onInput={e=>setCustomTo(e.currentTarget.value)} onChange={e=>setCustomTo(e.target.value)} /></label>
                  {customFrom && customTo && customFrom>customTo ? <p role="alert" className="text-xs text-red-600">{t("结束日期不能早于开始日期")}</p> : null}
                  <button type="submit" className="squircle btn btn-primary w-full" disabled={!isDateRange(customFrom,customTo)}>{t("应用日期")}</button>
                </form>
              ) : GROUPS.map((group, i) => (
                <div key={i}>
                  {i > 0 ? <div className="my-1 h-px bg-gray-200" /> : null}
                  {group.map((id) => (
                    <button
                      key={id}
                      type="button"
                      aria-pressed={search.period === id}
                      data-selected={search.period === id}
                      className="squircle control-menu-item flex w-full items-center justify-between rounded-md px-4 py-2.5 text-left text-sm leading-tight hover:bg-gray-100 hover:text-gray-900"
                      onClick={() => {
                        if (id === "custom") { setCustom(true); return }
                        onPeriod({ period: id })
                        setOpen(null)
                      }}
                    >
                      {t(PERIOD_LABEL[id])}
                      {HINT[id] ? <kbd className="text-xs font-normal text-gray-400">{HINT[id]}</kbd> : null}
                    </button>
                  ))}
                </div>
              ))}
            </div>
          ) : null}
        </div>

        {search.period === "realtime" ? null : (
          <div className="relative">
            <button
              type="button"
              className="squircle control-trigger flex w-10 items-center justify-center rounded-md text-gray-700 hover:bg-gray-150/80"
              aria-label={t("更多选项")}
              aria-expanded={open === "more"}
              onClick={() => setOpen(open === "more" ? null : "more")}
            >
              <UIIcon name="ellipsis-vertical" className="size-5" />
            </button>
            {open === "more" ? (
              <div className="squircle dashboard-popover absolute right-0 z-20 mt-2 w-72 rounded-md bg-white p-1 font-medium text-gray-800 shadow-lg ring-1 ring-black/5">
                {(() => {
                  const options = availableIntervals(search.period, { from: search.from, to: search.to })
                  if (options.length <= 1) return null
                  return (
                    <div className="flex w-full items-center justify-between gap-x-2 py-1 pr-2 pl-4">
                      <span className="shrink-0 text-sm font-medium text-gray-700">{t("图表间隔")}</span>
                      <div className="squircle segmented-control flex rounded-md bg-gray-100">
                        {options.map((id) => (
                          <button
                            key={id}
                            type="button"
                            aria-pressed={search.interval === id}
                            className="squircle rounded px-2 py-1 text-xs font-medium text-gray-600 hover:text-gray-900"
                            onClick={() => {
                              localStorage.setItem(intervalStorageKey(domain, search.period), id)
                              onPeriod({ interval: id })
                            }}
                          >
                            {t(INTERVAL_LABEL[id as Interval])}
                          </button>
                        ))}
                      </div>
                    </div>
                  )
                })()}

                <button
                  type="button"
                  className="squircle control-menu-item flex w-full items-center justify-between rounded-md px-4 py-2.5 text-sm leading-tight hover:bg-gray-100 hover:text-gray-900"
                  onClick={() => {
                    onExport?.()
                    setOpen(null)
                  }}
                >
                  <span className="text-sm">{t("导出统计")}</span>
                  
                </button>
              </div>
            ) : null}
          </div>
        )}
      </div>
    </div>
  )
}
