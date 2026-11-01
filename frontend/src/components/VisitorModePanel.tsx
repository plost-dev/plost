import { useState } from "react"
import { useRouter } from "@tanstack/react-router"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { saveVisitorModeFn } from "~/lib/visitor-mode-actions"
import type { VisitorMode } from "~/lib/visitor-mode-preference"
import { Tile } from "./ui"

const OPTIONS: ReadonlyArray<{ id: VisitorMode, title: string, detail: string }> = [
  { id: "plost", title: "Plost 方式（默认）", detail: "本程序自己的算法。访客按浏览器里保存的匿名标识计数，同一个浏览器换了 IP 或代理仍算一位；30 分钟没有活动后算新的一次访问；只看了一页就离开算跳出。" },
  { id: "umami", title: "Umami 方式", detail: "与 Umami 一致。访客按 IP 地址、浏览器标识和月份去重，换 IP 或代理就多算一位；同一位访客每个小时算一次访问；只看了一页算跳出。数字通常最大。" },
  { id: "google", title: "Google Analytics 方式", detail: "近似 Google Analytics 4。“用户”按浏览器里的匿名标识计数，不看 IP；30 分钟没有活动后算新的“会话”；跳出率按“未互动的会话”计算：浏览不到 2 页且持续不到 10 秒。没有测量页面停留时间，所以只是近似。" },
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
  return <Tile icon="user" title={t("访客统计方式")} subtitle={t("决定首页、统计页、分享页和 API 里的访客、访问次数和跳出率怎么数。三种方式用的是同一份数据，随时可以切换对比，已有数据不会改动。")}>
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
    <p className="mt-4 text-xs leading-6 text-gray-500">{t("Umami 方式需要拿到真实的访客 IP：放在反向代理后面时，请用 -trust-proxy 启动并让代理覆盖 X-Forwarded-For，否则所有访客的 IP 相同，数字会偏小。升级之前已经记录的访问没有 IP 信息，仍按浏览器标识计数。")}</p>
    {error ? <p role="alert" className="mt-3 text-sm text-red-600">{t(error)}</p> : null}
  </Tile>
}
