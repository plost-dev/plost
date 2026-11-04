import { useT } from "~/lib/i18n"
import { useState, type CSSProperties, type KeyboardEvent } from "react"
import { axisScale, formatAxisTick } from "~/lib/axis"

const DAYS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"]
export type HeatCell = { date: string, dow: number, hour: number, value: number }
export type HeatDay = { date: string, dow: number }

// Rows are the last seven real days ending today (from the server, in the site's timezone),
// so the grid fills in as the week goes on and today is always the bottom row.
/** `bare` drops the title when the heatmap sits under a report card tab that already names it. */
export function TrafficHeatmap({ cells, days, bare = false }: { cells: HeatCell[], days: HeatDay[], bare?: boolean }) {
  const { t, locale } = useT()
  const [selected, setSelected] = useState<{ row: number, hour: number } | null>(null)
  const [chosenDate, setChosenDate] = useState<string | null>(null)
  const values = new Map(cells.map(cell => [`${cell.date}-${cell.hour}`, cell.value]))
  const max = Math.max(1, ...cells.map(cell => cell.value))
  const today = days.length - 1
  const chosen = days.findIndex(item => item.date === chosenDate)
  const row = chosen >= 0 ? chosen : today
  const valueAt = (index: number, hour: number) => days[index] ? values.get(`${days[index].date}-${hour}`) || 0 : 0
  const hours = Array.from({ length: 24 }, (_, hour) => valueAt(row, hour))
  const hasData = hours.some(value => value > 0)
  // The axis uses the same whole steps as the trend chart (0, 1k), so the top is a round number, not the busiest hour.
  const scale = axisScale(Math.max(1, ...hours))
  const gridlines: CSSProperties = {backgroundImage: scale.ticks.slice(1).map(tick => {
    const at = `${tick / scale.max * 100}%`
    return `linear-gradient(to top, transparent calc(${at} - 1px), var(--color-gray-100) calc(${at} - 1px), var(--color-gray-100) ${at}, transparent ${at})`
  }).join(",")}
  const hourLabel = (hour: number) => `${String(hour).padStart(2, "0")}:00`
  const dayName = (index: number) => days[index] ? t(DAYS[days[index].dow - 1]) : ""
  const dateLabel = (index: number) => {
    const date = days[index]?.date
    if (!date) return ""
    const short = `${Number(date.slice(5, 7))}/${Number(date.slice(8, 10))}`
    return index === today ? `${dayName(index)} ${short} · ${t("今天")}` : `${dayName(index)} ${short}`
  }
  const cellLabel = (index: number, hour: number) => t("{0} {1} · {2} 访客", {0: dateLabel(index), 1: hourLabel(hour), 2: valueAt(index, hour).toLocaleString(locale)})
  function selectCell(index: number, hour: number) {
    setChosenDate(days[index]?.date ?? null)
    setSelected({ row: index, hour })
  }
  function moveCell(event: KeyboardEvent<HTMLButtonElement>, index: number, hour: number, hourly = false) {
    const delta = {ArrowLeft:[0,-1], ArrowRight:[0,1], ArrowUp:[-1,0], ArrowDown:[1,0]}[event.key]
    if (!delta && event.key !== "Home" && event.key !== "End") return
    event.preventDefault()
    const nextRow = hourly ? index : Math.max(0, Math.min(days.length - 1, index + (delta?.[0] || 0)))
    const nextHour = event.key === "Home" ? 0 : event.key === "End" ? 23 : Math.max(0, Math.min(23, hour + (delta?.[1] || 0)))
    event.currentTarget.closest(hourly ? ".traffic-hourly-bars" : ".traffic-grid")?.querySelector<HTMLButtonElement>(`[data-row="${nextRow}"][data-hour="${nextHour}"]`)?.focus()
  }
  return (
    <div className="traffic-panel">
      {bare ? null : <div className="analytics-heading"><h3 title={t("颜色越深，代表这个时段的访客越多。")}>{t("访问时段")}</h3><span>{t("最近 7 天 × 小时")}</span></div>}
      <div className="traffic-scroll"><div className="traffic-grid">
        <span />
        {Array.from({length: 24}, (_, hour) => <span className="traffic-hour" key={hour}>{hour % 6 === 0 ? `${String(hour).padStart(2, "0")}` : ""}</span>)}
        {days.map((item, index) => <div className="traffic-day" key={item.date}>
          <span className="traffic-day-label" title={dateLabel(index)} data-today={index === today || undefined}>{index === today ? t("今天") : dayName(index)}</span>
          {Array.from({length: 24}, (_, hour) => {
            const value = valueAt(index, hour)
            const label = cellLabel(index, hour)
            return <button key={hour} type="button" className="squircle traffic-cell" aria-label={label} title={label}
              data-row={index} data-hour={hour} tabIndex={(selected?.row ?? row) === index && (selected?.hour ?? 0) === hour ? 0 : -1}
              onKeyDown={event=>moveCell(event,index,hour)}
              style={{background: value ? `color-mix(in srgb, var(--analytics-accent) ${Math.round(20 + 80 * Math.sqrt(value / max))}%, var(--analytics-tint))` : "var(--color-gray-100)"}}
              onMouseEnter={() => selectCell(index, hour)} onFocus={() => selectCell(index, hour)} onClick={() => selectCell(index, hour)} />
          })}
        </div>)}
      </div></div>
      <div className="traffic-meta">
        <div className="traffic-caption" aria-live="polite">{selected ? cellLabel(selected.row, selected.hour) : t("悬停或点选色块，查看访客数")}</div>
        <div className="traffic-legend"><span>{t("少")}</span>{[0,25,50,75,100].map(value => <i className="squircle" key={value} style={{background: `color-mix(in srgb, var(--analytics-accent) ${value}%, var(--color-gray-100))`}} />)}<span>{t("多")}</span></div>
      </div>
      <div className="traffic-hourly">
        <div className="traffic-hourly-heading">
          <h4>{t("小时分布")}</h4>
          <select className="squircle" aria-label={t("选择日期")} value={days[row]?.date ?? ""} onChange={event => {
            setChosenDate(event.target.value)
            setSelected(null)
          }}>
            {days.map((item, index) => <option key={item.date} value={item.date}>{dateLabel(index)}</option>)}
          </select>
        </div>
        <div className="traffic-scroll"><div className="traffic-hourly-chart" role="group" aria-label={t("{0}每小时访客", {0: dateLabel(row)})}>
          <div className="traffic-hourly-axis" aria-hidden="true">{hasData ? scale.ticks.map(tick => <span key={tick} style={{bottom: `${tick / scale.max * 100}%`}}>{formatAxisTick(tick)}</span>) : null}</div>
          <div className="traffic-hourly-bars" style={hasData ? gridlines : undefined}>
            {hours.map((value, hour) => <button key={hour} type="button" className="traffic-hourly-slot"
              aria-label={cellLabel(row, hour)} title={cellLabel(row, hour)}
              data-row={row} data-hour={hour} tabIndex={(selected?.hour ?? 0) === hour ? 0 : -1}
              onKeyDown={event=>moveCell(event,row,hour,true)}
              data-selected={selected?.row === row && selected.hour === hour}
              onMouseEnter={() => selectCell(row, hour)} onFocus={() => selectCell(row, hour)} onClick={() => selectCell(row, hour)}>
              <span className="traffic-hourly-bar" style={{height: `${value / scale.max * 100}%`}} />
            </button>)}
            {!hasData ? <span className="traffic-hourly-empty">{t("暂无数据")}</span> : null}
          </div>
          <div className="traffic-hourly-ticks" aria-hidden="true">
            {hours.map((_, hour) => <span key={hour}>{hour % 6 === 0 || hour === 23 ? String(hour).padStart(2, "0") : ""}</span>)}
          </div>
        </div></div>
      </div>
    </div>
  )
}
