import { UIIcon } from "./UIIcon"
import { useEffect, useState } from "react"
import { useRouter } from "@tanstack/react-router"
import { MAPBOX_STYLES, type MapboxStyleId } from "~/lib/mapbox-styles"
import { useMapStyle } from "~/lib/map-preferences"
import { saveMapStyleFn } from "~/lib/mapbox-actions"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"

export function MapStylePicker() {
  const {t} = useT()
  const router = useRouter()
  const saved = useMapStyle()
  const [selected, setSelected] = useState(saved)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  useEffect(() => setSelected(saved), [saved])
  async function choose(styleId: MapboxStyleId) {
    if (pending || styleId === saved) return
    setSelected(styleId); setPending(true); setError("")
    let persisted = false
    try {
      const result = await saveMapStyleFn({data: {styleId}})
      persisted = true
      setSelected(result.styleId)
      await router.invalidate({sync: true})
      showToast("地图样式已保存，统计页将使用此样式。")
    } catch (err) {
      if (!persisted) setSelected(saved)
      setError(persisted ? t("样式已保存，请刷新页面载入最新设置。") : err instanceof Error ? err.message : t("保存失败"))
    } finally {setPending(false)}
  }
  return <fieldset className="map-style-picker" disabled={pending} aria-busy={pending}>
    <legend>{t("地图样式")}{pending ? <svg className="button-spinner map-style-spinner" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="2.5" opacity=".25"/>
      <path d="M12 3a9 9 0 0 1 9 9" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"/>
    </svg> : null}<span className="sr-only" role="status">{pending ? t("保存中…") : ""}</span></legend>
    <div className="map-style-options">
      {MAPBOX_STYLES.map(style => <label key={style.id} className={`squircle map-style-option${selected === style.id ? " is-selected" : ""}`}>
        <input className="sr-only" type="radio" name="mapbox-style" value={style.id} checked={selected === style.id} onChange={() => void choose(style.id)}/>
        <img className="squircle map-style-preview" src={`/map-previews/${style.id}.webp`} alt={t("{0}地图预览", {0: t(style.label)})} width="360" height="220" loading="lazy" decoding="async"/>
        <span className="map-style-option-caption"><span>{t(style.label)}</span>{selected === style.id ? <UIIcon name="check" className="size-3.5"/> : null}</span>
      </label>)}
    </div>
    {error ? <p className="text-sm text-red-600" role="alert">{t(error)}</p> : null}
  </fieldset>
}
