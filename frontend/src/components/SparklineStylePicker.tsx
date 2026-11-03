import { useEffect, useState } from "react"
import { useRouter } from "@tanstack/react-router"
import { SiteSparkline, useSparkline } from "./SiteSparkline"
import { UIIcon } from "./UIIcon"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { SPARKLINE_RANGES, SPARKLINE_STYLES, saveSparklineFn, type SparklinePreference } from "~/lib/sparkline-styles"

// The same made-up traffic for every preview, so the styles compare like for like.
const SAMPLE: Record<SparklinePreference["range"], number[]> = {
  "24h": [32, 28, 22, 18, 16, 15, 19, 27, 38, 47, 52, 55, 58, 54, 57, 61, 66, 70, 64, 58, 52, 47, 41, 36],
  "30d": [420, 470, 505, 530, 520, 490, 455, 430, 445, 480, 540, 600, 640, 655, 630, 590, 560, 575, 620, 690, 740, 770, 760, 720, 690, 700, 735, 790, 830, 805],
}

export function SparklineStylePicker() {
  const { t } = useT()
  const router = useRouter()
  const saved = useSparkline()
  const [selected, setSelected] = useState(saved)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  useEffect(() => setSelected(saved), [saved])
  async function choose(next: SparklinePreference) {
    if (pending || (next.style === saved.style && next.range === saved.range)) return
    setSelected(next); setPending(true); setError("")
    try {
      setSelected(await saveSparklineFn(next))
      await router.invalidate({ sync: true })
      showToast("走势图样式已保存，站点列表将使用此样式。")
    } catch (err) {
      setSelected(saved)
      setError(err instanceof Error ? err.message : t("保存失败"))
    } finally { setPending(false) }
  }
  return <fieldset className="map-style-picker sparkline-style-picker" disabled={pending} aria-busy={pending}>
    <legend>{t("走势图样式")}</legend>
    <div className="sparkline-range" role="radiogroup" aria-label={t("时间范围")}>
      <span>{t("时间范围")}</span>
      <div className="squircle segmented-control flex rounded-md bg-gray-100">
        {SPARKLINE_RANGES.map(range => <button key={range.id} type="button" role="radio" aria-checked={selected.range === range.id} aria-pressed={selected.range === range.id}
          className="squircle rounded px-2.5 py-1 text-xs font-medium text-gray-600 hover:text-gray-900" onClick={() => void choose({ ...selected, range: range.id })}>{t(range.label)}</button>)}
      </div>
    </div>
    <div className="map-style-options sparkline-style-options">
      {SPARKLINE_STYLES.map(style => <label key={style.id} className={`squircle map-style-option${selected.style === style.id ? " is-selected" : ""}`}>
        <input className="sr-only" type="radio" name="sparkline-style" value={style.id} checked={selected.style === style.id} onChange={() => void choose({ ...selected, style: style.id })} />
        <span className="squircle sparkline-style-preview"><SiteSparkline values={SAMPLE[selected.range]} style={style.id} className="h-10 w-full" /></span>
        <span className="map-style-option-caption"><span>{t(style.label)}</span>{selected.style === style.id ? <UIIcon name="check" className="size-3.5" /> : null}</span>
      </label>)}
    </div>
    {error ? <p className="text-sm text-red-600" role="alert">{t(error)}</p> : null}
  </fieldset>
}
