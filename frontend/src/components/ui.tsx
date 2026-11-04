import { UIIcon, BrandIcon, type BrandName } from "./UIIcon"
import { Surface } from "~/components/Surface"
import { useT } from "~/lib/i18n"
import { useEffect, useId, useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react"
import { createPortal } from "react-dom"
import { CountryFlag, type RowIconKind } from "~/components/icons"
import { numberShort, percentShort } from "~/lib/format"
import { delta } from "~/lib/range"
import { ReportValue } from "./ReportValue"
import { reportSearchText } from "~/lib/report-values"

const BAR: Record<string, string> = {
  source: "bg-blue-50 group-hover/row:bg-blue-100",
  channel: "bg-blue-50 group-hover/row:bg-blue-100",
  campaign: "bg-blue-50 group-hover/row:bg-blue-100",
  page: "bg-orange-50 group-hover/row:bg-orange-100",
  country: "bg-orange-50 group-hover/row:bg-orange-100",
  region: "bg-orange-50 group-hover/row:bg-orange-100",
  city: "bg-orange-50 group-hover/row:bg-orange-100",
  browser: "bg-green-50 group-hover/row:bg-green-100",
  os: "bg-green-50 group-hover/row:bg-green-100",
  device: "bg-green-50 group-hover/row:bg-green-100",
  language: "bg-green-50 group-hover/row:bg-green-100",
  screen: "bg-green-50 group-hover/row:bg-green-100",
  title: "bg-orange-50 group-hover/row:bg-orange-100",
  query: "bg-orange-50 group-hover/row:bg-orange-100",
  keyword: "bg-blue-50 group-hover/row:bg-blue-100",
  goal: "bg-red-50 group-hover/row:bg-red-100",
  event: "bg-orange-50 group-hover/row:bg-orange-100",
  property: "bg-orange-50 group-hover/row:bg-orange-100",
  download: "bg-orange-50 group-hover/row:bg-orange-100",
  outbound: "bg-orange-50 group-hover/row:bg-orange-100",
  notfound: "bg-orange-50 group-hover/row:bg-orange-100",
  ai: "bg-blue-50 group-hover/row:bg-blue-100",
}

export { formatDuration } from "~/lib/format"

export function Card({
  label,
  value,
  suffix = "",
  prev,
  selected,
  onSelect,
  text,
  index = 0,
  invertChange = false,
}: {
  label: string
  value: number
  suffix?: string
  prev?: number
  selected?: boolean
  onSelect?: () => void
  text?: string
  index?: number
  invertChange?: boolean
}) {
  const { t } = useT()
  const d = delta(value, prev)
  const up = invertChange ? (d ? d.diff < 0 : false) : (d ? d.diff > 0 : false)
  const down = invertChange ? (d ? d.diff > 0 : false) : (d ? d.diff < 0 : false)
  return (
    <div className={`metric-item group my-2 w-1/2 select-none px-4 lg:w-auto lg:flex-1 ${index > 0 ? "lg:border-l border-gray-200" : ""} ${index % 2 === 0 ? "border-r lg:border-r-0" : ""}`}>
      <button
        type="button"
        aria-pressed={selected}
        disabled={!onSelect}
        onClick={onSelect}
        className={`squircle -mx-2 flex w-full flex-col gap-y-1 rounded-md p-2 text-left ${onSelect ? "cursor-pointer hover:bg-gray-100/80" : "cursor-default"}`}
      >
        <div className={`flex w-fit text-xs uppercase whitespace-nowrap ${selected ? "font-bold tracking-[-.01em] text-gray-900" : "font-semibold text-gray-500 group-hover:text-gray-900"}`}>
          {t(label || "")}
        </div>
        <span className="metric-value-row flex items-baseline whitespace-nowrap">
          <p className="metric-value text-[1.2rem] font-semibold text-gray-900">
            {text ?? `${numberShort(value)}${suffix}`}
          </p>
          {d ? (
            <span className="metric-change ml-2 text-xs font-medium text-gray-500">
              {d.diff !== 0 ? (
                <UIIcon name={d.diff > 0 ? "arrow-up-right" : "arrow-down-right"} className={`mb-0.5 mr-0.5 inline-block size-2.5 ${up ? "text-green-500" : down ? "text-red-500" : "text-gray-400"}`} />
              ) : null}
              {Math.abs(d.pct)}%
            </span>
          ) : null}
        </span>
      </button>
    </div>
  )
}

const DIM_LABEL: Record<string, string> = {
  source: "来源",
  channel: "渠道",
  campaign: "活动",
  page: "页面",
  country: "国家/地区",
  region: "地区",
  city: "城市",
  browser: "浏览器",
  os: "操作系统",
  device: "设备",
  language: "语言",
  screen: "屏幕",
  title: "标题",
  query: "查询",
  keyword: "搜索词",
  goal: "目标",
  hostname: "主机名",
  event: "事件",
  property: "事件属性",
  download: "下载文件",
  outbound: "外链",
  notfound: "404 页面",
  ai: "AI 来源",
}

type ListKind = RowIconKind | "page" | "goal" | "channel" | "campaign" | "region" | "city" | "hostname" | "title" | "query" | "language" | "screen" | "keyword" | "event" | "property" | "download" | "outbound" | "notfound" | "ai"

const MAX_ITEMS = 9
const ROW_HEIGHT = 32
const ROW_GAP = 4
const LIST_MIN_HEIGHT = 356
const DATA_HEIGHT = (ROW_HEIGHT + ROW_GAP) * (MAX_ITEMS - 1) + ROW_HEIGHT

export type ListRow = {
  name: string
  value: number
  label?: string
  href?: string
  code?: string
  metrics?: Record<string, number>
}

export type ListColumn = {
  key: string
  label: string
  width?: string
  format?: (n: number, row: ListRow) => string
}

export function List({
  title,
  rows,
  onPick,
  kind,
  plain,
  columns,
  maxItems = MAX_ITEMS,
  compact = false,
  paginate = false,
  expanded = false,
  referenceRows,
  valueLabel = "访客",
  percentageTotal,
  showPercent = true,
  emptyMessage = "暂无数据",
}: {
  title?: string
  rows: ListRow[]
  onPick?: (name: string) => void
  kind?: ListKind
  plain?: boolean
  columns?: ListColumn[]
  maxItems?: number
  compact?: boolean
  paginate?: boolean
  expanded?: boolean
  /** Searching narrows visible rows without changing percentages or bar scale. */
  referenceRows?: ListRow[]
  valueLabel?: string
  /** Full aggregate count, including rows outside the query limit. */
  percentageTotal?: number
  showPercent?: boolean
  emptyMessage?: string
}) {
  const { t } = useT()
  const [page, setPage] = useState(1)
  const pages = Math.max(1, Math.ceil(rows.length / 10))
  const currentPage = Math.min(page, pages)
  const rowIdentity = rows.map(row=>row.name).join("\u0000")
  useEffect(()=>setPage(1),[rowIdentity])
  const shown = paginate ? rows.slice((currentPage - 1) * 10, currentPage * 10) : maxItems > 0 ? rows.slice(0, maxItems) : rows
  const max = Math.max(1, ...(referenceRows || (paginate ? rows : shown)).map((r) => r.value))
  const total = percentageTotal ?? (referenceRows || rows).reduce((n, r) => n + r.value, 0)
  const bar = BAR[kind || ""] || "bg-indigo-50 group-hover/row:bg-indigo-100"
  const flagKind = kind === "region" || kind === "city"
  const extraCols = columns || []
  const bundlePct = extraCols.length === 0
  const valueWidth = expanded ? "w-24 min-w-24 sm:w-32 sm:min-w-32" : "w-16 min-w-16 md:w-32 md:min-w-32"

  const emptyMin = maxItems > 0 && !compact ? LIST_MIN_HEIGHT : 160
  const body = shown.length === 0 ? (
    <div className="report-list-empty flex h-full w-full flex-col justify-center" style={{ "--report-empty-height": `${emptyMin}px` } as CSSProperties}>
      <div className="mx-auto max-w-full px-4 text-center text-sm font-medium text-gray-500">{t(emptyMessage)}</div>
    </div>
  ) : (
    <div className={`flex flex-col ${expanded ? "" : "h-full"}`} style={extraCols.length ? {minWidth: 160 + extraCols.length * 72} : undefined}>
      <div className="flex w-full items-center pt-3 text-xs font-medium text-gray-500" style={{ height: ROW_HEIGHT }}>
        <div className="w-full min-w-0 grow truncate">{t(DIM_LABEL[kind || ""] || title || "")}</div>
        {bundlePct ? (
          <div className={`${valueWidth} shrink-0 text-right`}>{t(valueLabel)}</div>
        ) : extraCols.map((col) => (
          <div key={col.key} className={`${col.width || "w-16 min-w-16 md:w-[5.5rem] md:min-w-[5.5rem]"} shrink-0 text-right`}>
            {t(col.label)}
          </div>
        ))}
      </div>
      <div className="report-list-rows group/report" style={maxItems > 0 && !compact ? { "--report-rows-height": `${DATA_HEIGHT}px` } as CSSProperties : undefined}>
        {shown.map((r) => {
          const width = Math.max(2, (r.value / max) * 100)
          const pct = total > 0 ? (r.value / total) * 100 : 0
          return (
            <div key={r.name} style={{ minHeight: ROW_HEIGHT }}>
              <div
                className={`squircle group/row flex w-full items-center rounded-sm hover:bg-gray-100/60 ${onPick ? "cursor-pointer" : ""}`}
                style={{ marginTop: ROW_GAP }}
                onClick={onPick ? () => onPick(r.name) : undefined}
                role={onPick ? "button" : undefined}
                tabIndex={onPick ? 0 : undefined}
                onKeyDown={onPick ? event => {if (event.target === event.currentTarget && (event.key === "Enter" || event.key === " ")) {event.preventDefault();onPick(r.name)}} : undefined}
              >
                <div className={`w-full min-w-0 grow ${expanded ? "" : "md:truncate"}`}>
                  <div className="relative h-full w-full">
                    <div className={`squircle absolute top-0 left-0 h-full rounded-sm ${bar}`} style={{ width: `${width}%` }} />
                    <div className={`relative z-9 px-2 py-1.5 text-sm ${expanded ? "[overflow-wrap:anywhere]" : ""}`}>
                      <div className="flex w-full items-center justify-start gap-x-1.5">
                        <span className={`flex w-full min-w-0 max-w-full items-center gap-x-2 ${expanded || kind === "language" ? "" : "overflow-hidden"}`}>
                          {flagKind && r.code ? <CountryFlag code={r.code}/> : null}
                          <ReportValue kind={kind} name={r.name} label={r.label} code={r.code} wrap={expanded} interactive={Boolean(onPick || r.href)}/>
                        </span>
                        {r.href ? (
                          <a
                            href={r.href}
                            target="_blank"
                            rel="noreferrer"
                            aria-label={t("在新标签打开")}
                            className={expanded ? "shrink-0" : "invisible md:group-hover/row:visible"}
                            onClick={(e) => e.stopPropagation()}
                          >
                            <UIIcon name="external" className="mb-0.5 inline-flex size-3.5 text-gray-600 hover:text-gray-800" />
                          </a>
                        ) : null}
                      </div>
                    </div>
                  </div>
                </div>
                {bundlePct ? (
                  <div className={`${valueWidth} shrink-0 overflow-hidden text-right text-sm font-medium text-gray-800`}>
                    <div className="flex w-full">
                      <div className={!showPercent ? "w-full" : expanded ? "w-1/2" : "w-full md:w-1/2"}>
                        <span className={`block w-full tabular-nums ${expanded || !showPercent ? "" : "transition-all duration-150 md:translate-x-[100%] md:group-hover/report:translate-x-0"}`}>{numberShort(r.value)}</span>
                      </div>
                      {showPercent ? <div className={expanded ? "w-1/2" : "hidden w-1/2 md:block"}>
                        <span className={`block w-full text-gray-500 tabular-nums ${expanded ? "" : "translate-x-[100%] opacity-0 transition-all duration-150 md:group-hover/report:translate-x-0 md:group-hover/report:opacity-100"}`}>{percentShort(pct)}</span>
                      </div> : null}
                    </div>
                  </div>
                ) : extraCols.map((col) => (
                  <div key={col.key} className={`${col.width || "w-16 min-w-16 md:w-[5.5rem] md:min-w-[5.5rem]"} shrink-0 text-right text-sm font-medium text-gray-800 tabular-nums`}>
                    {col.format ? col.format(r.metrics?.[col.key] ?? r.value, r) : numberShort(r.metrics?.[col.key] ?? r.value)}
                  </div>
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
  const content = extraCols.length && !expanded ? <div className="report-list-scroll">{body}</div> : body
  if (plain) return <>{content}{paginate && rows.length > 10 ? <nav className="report-list-pagination" aria-label={t("分页")}><span>{t("每页 10 条")} · {rows.length}</span><div><button className="squircle" type="button" disabled={currentPage <= 1} onClick={()=>setPage(currentPage-1)}>{t("上一页")}</button><span>{currentPage} / {pages}</span><button className="squircle" type="button" disabled={currentPage >= pages} onClick={()=>setPage(currentPage+1)}>{t("下一页")}</button></div></nav> : null}</>
  return (
    <Surface as="section" className="report-card relative flex w-full flex-col overflow-x-hidden bg-white shadow-sm">
      {title ? <h2 className="mb-3 text-xs font-bold uppercase tracking-[-.01em] text-gray-900">{t(title || "")}</h2> : null}
      {content}
    </Surface>
  )
}

export type ReportTab = {
  id: string
  label: string
  dropdown?: Array<{ id: string, label: string, onSelect?: () => void, selected?: boolean }>
}

function MoreLinkIcon() {
  return (
    <UIIcon name="expand" className="size-4" />
  )
}

export type DetailsSpec = {
  title: string
  kind?: ListKind
  rows: ListRow[]
  columns?: ListColumn[]
  description?: string
  valueLabel?: string
  percentageTotal?: number
  showPercent?: boolean
  emptyMessage?: string
  onPick?: (name: string) => void
}

function DetailsModal({
  spec,
  onClose,
}: {
  spec: DetailsSpec
  onClose: () => void
}) {
  const { t, locale } = useT()
  const dialog = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  const [q, setQ] = useState("")
  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return spec.rows
    return spec.rows.filter((r) => reportSearchText(spec.kind, r, locale).toLowerCase().includes(needle))
  }, [q, spec.rows, spec.kind, locale])

  useEffect(() => {
    const prev = document.body.style.overflow
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const element = dialog.current
    element?.showModal()
    document.body.style.overflow = "hidden"
    return () => {
      element?.close()
      document.body.style.overflow = prev
      if (trigger?.isConnected) trigger.focus()
    }
  }, [])

  return createPortal(
    <dialog ref={dialog} className="squircle report-details-dialog" aria-labelledby={titleId} onCancel={event=>{event.preventDefault();onClose()}} onKeyDown={event=>{if(event.key === "Escape") {event.preventDefault();event.stopPropagation();if(!event.nativeEvent.isComposing && event.nativeEvent.keyCode !== 229) onClose()}}} onClick={event=>{if(event.target===event.currentTarget)onClose()}}>
          <div className="report-details flex min-h-0 flex-col overflow-hidden">
            <div className="mb-3 flex items-center justify-between gap-3">
              <h2 id={titleId} className="dialog-heading min-w-0 text-gray-900">{t(spec.title)}</h2>
              <button type="button" className="squircle ui-icon-button text-gray-400 hover:text-gray-600" aria-label={t("关闭")} onClick={onClose}>
                <UIIcon name="close" className="size-[1.125rem]" />
              </button>
            </div>
            {spec.description ? <p className="mb-3 text-xs leading-relaxed text-gray-500 [overflow-wrap:anywhere]">{t(spec.description)}</p> : null}
            <input
              type="search"
              aria-label={t("搜索")}
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder={t("搜索")}
              className="squircle input mb-2 placeholder:text-gray-400"
            />
            <div className="report-details-scroll min-h-0 min-w-0 flex-1 overflow-auto">
              <List plain expanded maxItems={0} kind={spec.kind} rows={filtered} referenceRows={spec.rows} columns={spec.columns} valueLabel={spec.valueLabel} percentageTotal={spec.percentageTotal} showPercent={spec.showPercent} emptyMessage={spec.emptyMessage} onPick={spec.onPick ? (name) => { spec.onPick?.(name); onClose() } : undefined} />
            </div>
          </div>
    </dialog>,
    document.body,
  )
}

export function ReportCard({
  title,
  tabs = [],
  active,
  onChange,
  more = true,
  extra,
  details,
  children,
  className = "",
  autoRevealActive = false,
}: {
  title?: string
  tabs?: ReportTab[]
  active?: string
  onChange?: (id: string) => void
  more?: boolean
  extra?: ReactNode
  details?: DetailsSpec
  children: ReactNode
  className?: string
  /** Reveal a selected tab horizontally without moving the surrounding page. */
  autoRevealActive?: boolean
}) {
  const { t } = useT()
  const [open, setOpen] = useState<string | null>(null)
  const [detailsOpen, setDetailsOpen] = useState(false)
  const tabsElement = useRef<HTMLDivElement>(null)
  const showMore = more && !!details?.rows.length

  useEffect(() => {
    const element = tabsElement.current
    if (!autoRevealActive || !element) return
    function reveal() {
      const selected = element!.querySelector<HTMLElement>('button[aria-pressed="true"]')
      if (!selected) return
      const viewport = element!.getBoundingClientRect()
      const tab = selected.getBoundingClientRect()
      if (tab.left < viewport.left) element!.scrollLeft -= viewport.left - tab.left
      else if (tab.right > viewport.right) element!.scrollLeft += tab.right - viewport.right
    }
    reveal()
    const observer = new ResizeObserver(reveal)
    observer.observe(element)
    return () => observer.disconnect()
  }, [active, autoRevealActive, t])

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (!(e.target as HTMLElement).closest("[data-report-dropdown]")) setOpen(null)
    }
    window.addEventListener("mousedown", onDoc)
    return () => window.removeEventListener("mousedown", onDoc)
  }, [])

  return (
    <Surface as="section" className={`report-card relative flex min-w-0 w-full flex-col bg-white shadow-sm ${className}`}>
      <div className="report-heading flex w-full justify-between border-b border-gray-200">
        {title ? <h3 className="pb-3 text-sm font-bold text-gray-900">{t(title || "")}</h3> : null}
        {tabs.length ? (
          <div ref={tabsElement} className="report-tabs flex items-baseline gap-x-3.5 text-xs font-medium text-gray-500">
            {tabs.map((reportTab) => {
              const selected = reportTab.dropdown
                ? reportTab.id === active || reportTab.dropdown.some((d) => d.id === active)
                : reportTab.id === active
              const label = reportTab.dropdown?.find((d) => d.id === active)?.label || reportTab.label
              if (reportTab.dropdown) {
                return (
                  <div key={reportTab.id} className="relative" data-report-dropdown>
                    <div className="report-tab">
                      <button
                        type="button"
                        className="squircle report-tab-trigger group/tab relative inline-flex items-center rounded-sm"
                        aria-pressed={selected}
                        aria-expanded={open === reportTab.id}
                        onClick={() => setOpen(open === reportTab.id ? null : reportTab.id)}
                      >
                        <span className={`truncate text-left text-xs uppercase ${selected ? "font-bold tracking-[-.01em] text-gray-900" : "font-semibold text-gray-500 group-hover/tab:text-gray-800"}`}>
                          {t(label || "")}
                        </span>
                        <UIIcon name="chevron-down" className="ui-chevron ml-1" />
                      </button>
                    </div>
                    {open === reportTab.id ? (
                      <div className="squircle report-menu absolute top-full left-0 z-20 mt-2 min-w-56 origin-top-left rounded-md bg-white p-1 font-medium text-gray-800 shadow-lg ring-1 ring-black/5">
                        {reportTab.dropdown.map((d) => (
                          <button
                            key={d.id}
                            type="button"
                            data-selected={d.selected || d.id === active}
                            aria-pressed={Boolean(d.selected || d.id === active)}
                            className="squircle control-menu-item flex w-full items-center justify-between rounded-md px-4 py-2.5 text-left text-sm hover:bg-gray-100"
                            onClick={() => {
                              if (d.onSelect) d.onSelect()
                              else onChange?.(d.id)
                              setOpen(null)
                            }}
                          >
                            {t(d.label)}
                            {d.selected || d.id === active ? <UIIcon name="check" className="selection-detail size-3.5" /> : null}
                          </button>
                        ))}
                      </div>
                    ) : null}
                  </div>
                )
              }
              return (
                <div key={reportTab.id} className="report-tab">
                  <button
                    type="button"
                    className="squircle report-tab-trigger group/tab relative rounded-sm"
                    aria-pressed={selected}
                    onClick={() => onChange?.(reportTab.id)}
                  >
                    <span className={`truncate text-left text-xs uppercase ${selected ? "font-bold tracking-[-.01em] text-gray-900" : "font-semibold text-gray-500 group-hover/tab:text-gray-800"}`}>
                      {t(reportTab.label)}
                    </span>
                  </button>
                </div>
              )
            })}
          </div>
        ) : null}
        <div className="flex items-start gap-x-3">
          {showMore ? (
            <button
              type="button"
              className="squircle ui-icon-button relative rounded text-gray-500 transition-colors duration-150 hover:text-gray-600"
              title={t("查看详情")}
              aria-label={t("查看详情")}
              onClick={() => { if (details) setDetailsOpen(true) }}
            >
              <MoreLinkIcon />
            </button>
          ) : null}
          {extra}
        </div>
      </div>
      <div className="min-h-0 flex-1">{children}</div>
      {detailsOpen && details ? <DetailsModal spec={details} onClose={() => setDetailsOpen(false)} /> : null}
    </Surface>
  )
}

export function ReportMenu({
  title = "按此拆分",
  options,
  value,
  onChange,
}: {
  title?: string
  options: Array<{ id: string, label: string }>
  value: string
  onChange: (id: string) => void
}) {
  const { t } = useT()
  const [open, setOpen] = useState(false)

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (!(e.target as HTMLElement).closest("[data-report-menu]")) setOpen(false)
    }
    window.addEventListener("mousedown", onDoc)
    return () => window.removeEventListener("mousedown", onDoc)
  }, [])

  return (
    <div className="relative" data-report-menu>
      <button
        type="button"
        className="squircle ui-icon-button relative rounded text-gray-500 transition-colors duration-150 hover:text-gray-600"
        aria-label={t("拆分选项")}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <UIIcon name="ellipsis-vertical" className="size-[1.125rem]" />
      </button>
      {open ? (
        <div className="squircle absolute top-full right-0 z-20 mt-2 min-w-48 origin-top-right rounded-md bg-white p-1 font-medium text-gray-800 shadow-lg ring-1 ring-black/5">
          <p className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-gray-500 uppercase">{t(title || "")}</p>
          {options.map((o) => (
            <button
              key={o.id}
              type="button"
              data-selected={o.id === value}
              aria-pressed={o.id === value}
              className="squircle control-menu-item flex w-full items-center justify-between rounded-md px-4 py-2.5 text-left text-sm hover:bg-gray-100"
              onClick={() => { onChange(o.id); setOpen(false) }}
            >
              {t(o.label)}
              {o.id === value ? <UIIcon name="check" className="selection-detail size-3.5" /> : null}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

export function Tile({
  icon = "settings",
  brand,
  id,
  title,
  subtitle,
  titleAddon,
  actions,
  logo,
  children,
}: {
  icon?: string
  brand?: BrandName
  id?: string
  title: string
  subtitle?: string
  /** Shown right after the title, such as a link. */
  titleAddon?: React.ReactNode
  /** Shown at the right end of the heading, such as a switch. */
  actions?: React.ReactNode
  /** A brand logo drawn in the text colour in place of the icon and the title, which stays for screen readers. */
  logo?: { src: string; width: number }
  children: React.ReactNode
}) {
  const { t } = useT()
  return (
    <Surface id={id} className="settings-card scroll-mt-24 bg-white shadow-sm">
      <header className="settings-card-heading">
        {logo ? null : <span className="squircle settings-card-icon">{brand ? <BrandIcon name={brand}/> : <UIIcon name={icon}/>}</span>}
        <div className="settings-heading-copy min-w-0">{(() => {
          const heading = <h2 className="text-lg leading-7 font-medium text-gray-900">{logo ? <><span className="settings-brand-logo" aria-hidden="true" style={{width: logo.width, maskImage: `url(${logo.src})`, WebkitMaskImage: `url(${logo.src})`}}/><span className="sr-only">{t(title || "")}</span></> : t(title || "")}</h2>
          return titleAddon ? <div className="settings-heading-title-row">{heading}{titleAddon}</div> : heading
        })()}
        {subtitle ? <div className="mt-px text-sm leading-5 text-gray-500">{t(subtitle || "")}</div> : null}</div>
        {actions ? <div className="settings-heading-actions">{actions}</div> : null}
      </header>
      <div className="mx-6 border-b border-gray-200" />
      <div className="relative p-4 sm:p-6">{children}</div>
    </Surface>
  )
}
