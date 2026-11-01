import { useState } from "react"
import { useRouter } from "@tanstack/react-router"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { saveVisitorModeFn } from "~/lib/visitor-mode-actions"
import type { VisitorMode } from "~/lib/visitor-mode-preference"
import { Tile } from "./ui"

const OPTIONS: ReadonlyArray<{ id: VisitorMode, title: string, detail: string }> = [
  { id: "persistent", title: "独立访客", detail: "按浏览器里保存的匿名标识计数。同一个浏览器换了 IP 或代理，仍然算一位访客；访问在 30 分钟没有活动后结束。" },
  { id: "sessions", title: "会话访客", detail: "按 IP 地址、浏览器标识和月份去重，一位访客在同一个小时内算一次访问，与 Umami 的算法一致。换 IP 或代理会重新计数，数字通常更大。" },
]

/** One choice for the whole instance: the site list, reports, shared pages and the API all follow it. */
export function VisitorModePanel({ initial }: { initial: VisitorMode }) {
  const { t } = useT()
  const router = useRouter()
  const [saved, setSaved] = useState(initial)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  async function choose(mode: VisitorMode) {
    if (pending || mode === saved) return
    setPending(true); setError("")
    try {
      const result = await saveVisitorModeFn({ data: { mode } })
      setSaved(result.mode)
      await router.invalidate({ sync: true })
      showToast("统计方式已保存，所有页面立即按新方式计数。")
    } catch (err) {
      setError(err instanceof Error ? err.message : t("保存失败"))
    } finally { setPending(false) }
  }
  return <Tile icon="user" title={t("访客统计方式")} subtitle={t("决定首页、统计页、分享页和 API 里的“访客”怎么数。随时可以切换，已有数据不会改动。")}>
    <fieldset className="visitor-mode-picker" disabled={pending} aria-busy={pending}>
      <legend className="sr-only">{t("访客统计方式")}</legend>
      <div className="visitor-mode-options">
        {OPTIONS.map(option => <label key={option.id} className={`squircle visitor-mode-option${saved === option.id ? " is-selected" : ""}`}>
          <input className="sr-only" type="radio" name="visitor-mode" value={option.id} checked={saved === option.id} onChange={() => void choose(option.id)} />
          <strong>{t(option.title)}</strong>
          <span>{t(option.detail)}</span>
        </label>)}
      </div>
    </fieldset>
    <p className="mt-4 text-xs leading-6 text-gray-500">{t("会话访客需要拿到真实的访客 IP：放在反向代理后面时，请用 -trust-proxy 启动并让代理覆盖 X-Forwarded-For，否则所有访客的 IP 相同，数字会偏小。切换之前已经记录的访问没有 IP 信息，仍按浏览器标识计数。")}</p>
    {error ? <p role="alert" className="mt-3 text-sm text-red-600">{t(error)}</p> : null}
  </Tile>
}
