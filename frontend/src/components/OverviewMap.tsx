import { useEffect, useRef, useState } from "react"
import { UIIcon } from "~/components/UIIcon"
import { OverviewGlobe, type OverviewMapMode, type OverviewPlace } from "~/components/OverviewGlobe"
import { OverviewGoogleMap, OverviewMapboxMap } from "~/components/OverviewServiceMap"
import type { OverviewMapConfig } from "~/lib/map-usage-actions"
import { useT } from "~/lib/i18n"

/**
 * The map beside the overview's figures: the built-in dotted globe, or Mapbox or Google Maps when the visitor map
 * settings send the chosen service here. A service that fails to load falls back to the built-in globe. Right-clicking
 * (or the context-menu key while it has focus) switches globe and flat map; Google Maps is only ever flat.
 */
export function OverviewMap({ places, config }: { places: OverviewPlace[], config: OverviewMapConfig | null }) {
  const { t } = useT()
  const [mode, setMode] = useState<OverviewMapMode>("globe")
  const [menu, setMenu] = useState<{ x: number, y: number } | null>(null)
  const [failed, setFailed] = useState(false)
  const frameRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const service = !failed && config?.key ? config.service : ""
  const switchable = service !== "google"

  useEffect(() => {
    if (!menu) return
    menuRef.current?.querySelector<HTMLButtonElement>("[aria-checked='true']")?.focus()
    const close = (event: Event) => { if (!(event.target instanceof Node && menuRef.current?.contains(event.target))) setMenu(null) }
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return
      setMenu(null)
      frameRef.current?.focus()
    }
    document.addEventListener("pointerdown", close, true)
    document.addEventListener("scroll", close, true)
    document.addEventListener("keydown", onKey)
    window.addEventListener("blur", close)
    return () => {
      document.removeEventListener("pointerdown", close, true)
      document.removeEventListener("scroll", close, true)
      document.removeEventListener("keydown", onKey)
      window.removeEventListener("blur", close)
    }
  }, [menu])

  return (
    <div ref={frameRef} className={`overview-globe is-${service === "google" ? "map" : mode}${service ? ` is-${service}` : ""}`} tabIndex={0}
      title={switchable ? t("右键切换地球或平面地图") : undefined}
      onContextMenu={(event) => {
        if (!switchable) return
        event.preventDefault()
        // Kept inside the map, since the card clips anything past its edge.
        const rect = event.currentTarget.getBoundingClientRect()
        setMenu({ x: Math.min(event.clientX - rect.left, rect.width - 136), y: Math.min(event.clientY - rect.top, rect.height - 88) })
      }}
      onKeyDown={(event) => {
        if (!switchable || (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10"))) return
        event.preventDefault()
        setMenu({ x: event.currentTarget.clientWidth / 2, y: event.currentTarget.clientHeight / 2 })
      }}>
      {service === "mapbox" ? <OverviewMapboxMap token={config!.key!} places={places} mode={mode} onFailure={() => setFailed(true)} />
        : service === "google" ? <OverviewGoogleMap apiKey={config!.key!} places={places} onFailure={() => setFailed(true)} />
        : <OverviewGlobe places={places} mode={mode} />}
      {menu ? (
        <div ref={menuRef} role="menu" aria-label={t("地图样式")} className="squircle overview-globe-menu" style={{ left: menu.x, top: menu.y }}>
          {(["globe", "map"] as const).map((value) => (
            <button key={value} type="button" role="menuitemradio" aria-checked={mode === value} className="squircle"
              onClick={() => { setMode(value); setMenu(null); frameRef.current?.focus() }}>
              {t(value === "globe" ? "地球" : "平面")}
              {mode === value ? <UIIcon name="check" /> : null}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}
