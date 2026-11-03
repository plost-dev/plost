import { UIIcon } from "./UIIcon"
import { useT } from "~/lib/i18n"
import { useState, type CSSProperties, type KeyboardEvent } from "react"
import { axisScale, formatAxisTick } from "~/lib/axis"

const DAYS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"]
export type HeatCell = { dow: number, hour: number, value: number }

export function TrafficHeatmap({ cells }: { cells: HeatCell[] }) {
  const { t, locale } = useT()
  const [selected, setSelected] = useState<{ dow: number, hour: number } | null>(null)
  const [chosenDay, setChosenDay] = useState<number | null>(null)
  const values = new Map(cells.map(cell => [`${cell.dow}-${cell.hour}`, cell.value]))
  const max = Math.max(1, ...cells.map(cell => cell.value))
  const dayTotals = DAYS.map((_, index) => cells.filter(cell => cell.dow === index + 1).reduce((sum, cell) => sum + cell.value, 0))
  const day = chosenDay ?? dayTotals.indexOf(Math.max(...dayTotals)) + 1
  const hours = Array.from({ length: 24 }, (_, hour) => values.get(`${day}-${hour}`) || 0)
  const hasData = hours.some(value => value > 0)
  // The axis uses the same whole steps as the trend chart (0, 1k), so the top is a round number, not the busiest hour.
  const scale = axisScale(Math.max(1, ...hours))
  const gridlines: CSSProperties = {backgroundImage: scale.ticks.slice(1).map(tick => {
    const at = `${tick / scale.max * 100}%`
    return `linear-gradient(to top, transparent calc(${at} - 1px), var(--color-gray-100) calc(${at} - 1px), var(--color-gray-100) ${at}, transparent ${at})`
  }).join(",")}
  const hourLabel = (hour: number) => `${String(hour).padStart(2, "0")}:00`
  const cellLabel = (dow: number, hour: number) => t("{0} {1} · {2} 访客", {0: t(DAYS[dow - 1]), 1: hourLabel(hour), 2: (values.get(`${dow}-${hour}`) || 0).toLocaleString(locale)})
  function selectCell(dow: number, hour: number) {
    setChosenDay(dow)
    setSelected({ dow, hour })
  }
  function moveCell(event: KeyboardEvent<HTMLButtonElement>, dow: number, hour: number, hourly = false) {
    const delta = {ArrowLeft:[0,-1], ArrowRight:[0,1], ArrowUp:[-1,0], ArrowDown:[1,0]}[event.key]
    if (!delta && event.key !== "Home" && event.key !== "End") return
    event.preventDefault()
    const nextDay = hourly ? dow : Math.max(1, Math.min(7, dow + (delta?.[0] || 0)))
    const nextHour = event.key === "Home" ? 0 : event.key === "End" ? 23 : Math.max(0, Math.min(23, hour + (delta?.[1] || 0)))
    event.currentTarget.closest(hourly ? ".traffic-hourly-bars" : ".traffic-grid")?.querySelector<HTMLButtonElement>(`[data-dow="${nextDay}"][data-hour="${nextHour}"]`)?.focus()
  }
  return (
    <div className="traffic-panel">
      <div className="analytics-heading"><h3 title={t("颜色越深，代表这个时段的访客越多。")}><UIIcon name="clock" className="analytics-heading-icon" />{t("访问时段")}</h3><span>{t("星期 × 小时")}</span></div>
      <div className="traffic-scroll"><div className="traffic-grid">
        <span />
        {Array.from({length: 24}, (_, hour) => <span className="traffic-hour" key={hour}>{hour % 6 === 0 ? `${String(hour).padStart(2, "0")}` : ""}</span>)}
        {DAYS.map((dayName, index) => <div className="traffic-day" key={dayName}>
          <span className="traffic-day-label">{t(dayName)}</span>
          {Array.from({length: 24}, (_, hour) => {
            const value = values.get(`${index + 1}-${hour}`) || 0
            const label = cellLabel(index + 1, hour)
            return <button key={hour} type="button" className="squircle traffic-cell" aria-label={label} title={label}
              data-dow={index + 1} data-hour={hour} tabIndex={(selected?.dow ?? day) === index + 1 && (selected?.hour ?? 0) === hour ? 0 : -1}
              onKeyDown={event=>moveCell(event,index+1,hour)}
              style={{background: value ? `color-mix(in srgb, var(--analytics-accent) ${Math.round(20 + 80 * Math.sqrt(value / max))}%, var(--analytics-tint))` : "var(--color-gray-100)"}}
              onMouseEnter={() => selectCell(index + 1, hour)} onFocus={() => selectCell(index + 1, hour)} onClick={() => selectCell(index + 1, hour)} />
          })}
        </div>)}
      </div></div>
      <div className="traffic-meta">
        <div className="traffic-caption" aria-live="polite">{selected ? cellLabel(selected.dow, selected.hour) : t("悬停或点选色块，查看访客数")}</div>
        <div className="traffic-legend"><span>{t("少")}</span>{[0,25,50,75,100].map(value => <i className="squircle" key={value} style={{background: `color-mix(in srgb, var(--analytics-accent) ${value}%, var(--color-gray-100))`}} />)}<span>{t("多")}</span></div>
      </div>
      <div className="traffic-hourly">
        <div className="traffic-hourly-heading">
          <h4>{t("小时分布")}</h4>
          <select className="squircle" aria-label={t("选择星期")} value={day} onChange={event => {
            setChosenDay(Number(event.target.value))
            setSelected(null)
          }}>
            {DAYS.map((name, index) => <option key={name} value={index + 1}>{t(name)}</option>)}
          </select>
        </div>
        <div className="traffic-scroll"><div className="traffic-hourly-chart" role="group" aria-label={t("{0}每小时访客", {0: t(DAYS[day - 1])})}>
          <div className="traffic-hourly-axis" aria-hidden="true">{hasData ? scale.ticks.map(tick => <span key={tick} style={{bottom: `${tick / scale.max * 100}%`}}>{formatAxisTick(tick)}</span>) : null}</div>
          <div className="traffic-hourly-bars" style={hasData ? gridlines : undefined}>
            {hours.map((value, hour) => <button key={hour} type="button" className="traffic-hourly-slot"
              aria-label={cellLabel(day, hour)} title={cellLabel(day, hour)}
              data-dow={day} data-hour={hour} tabIndex={(selected?.hour ?? 0) === hour ? 0 : -1}
              onKeyDown={event=>moveCell(event,day,hour,true)}
              data-selected={selected?.dow === day && selected.hour === hour}
              onMouseEnter={() => selectCell(day, hour)} onFocus={() => selectCell(day, hour)} onClick={() => selectCell(day, hour)}>
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
