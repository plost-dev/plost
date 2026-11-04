import { ActionButton } from "~/components/ActionButton"
import { createFileRoute, getRouteApi, useRouter } from "@tanstack/react-router"
import { useRef, useState } from "react"
import { Toggle } from "~/components/Toggle"
import { Tile } from "~/components/ui"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { saveSettingsFn, setShareFn } from "~/lib/actions"

const settings = getRouteApi("/sites/$domain/settings")

export const Route = createFileRoute("/sites/$domain/settings/visibility")({ component: VisibilityPage })

/** The address with a copy button; dimmed while its switch is off, when it cannot be opened. */
function LinkRow({ url, enabled }: { url: string, enabled: boolean }) {
  const { t } = useT()
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  return <div className={`flex min-w-0 items-center gap-2${enabled ? "" : " opacity-50"}`}>
    <input className="squircle input min-w-0 flex-1" readOnly value={url} aria-label={t("链接地址")} onFocus={event => event.target.select()} />
    <ActionButton type="button" className="btn btn-secondary shrink-0" icon={copied ? "check" : "copy"} onClick={async () => {
      try { await navigator.clipboard.writeText(url); setCopied(true); clearTimeout(timer.current); timer.current = setTimeout(() => setCopied(false), 2000) }
      catch { showToast("无法复制，请手动选中链接", "error") }
    }}>{t(copied ? "已复制" : "复制")}</ActionButton>
  </div>
}

function VisibilityPage() {
  const { data } = settings.useLoaderData()
  const { domain } = settings.useParams()
  const router = useRouter()
  const { t } = useT()
  const share = data.shares[0]
  const [isPublic, setIsPublic] = useState(!!data.site.public)
  const [shared, setShared] = useState(!!share?.enabled)
  const [busy, setBusy] = useState(false)
  const origin = typeof window === "undefined" ? "" : window.location.origin

  // Each switch saves as soon as it is flipped and goes back if the save fails.
  async function flip(next: boolean, set: (value: boolean) => void, save: () => Promise<unknown>) {
    set(next); setBusy(true)
    try { await save(); await router.invalidate(); showToast(next ? "已开启" : "已关闭") }
    catch (error) { set(!next); showToast(error instanceof Error ? error.message : "保存失败", "error") }
    finally { setBusy(false) }
  }

  return (
    <div>
      <Tile icon="eye" title={t("公开仪表盘")} actions={<Toggle hideLabel checked={isPublic} disabled={busy}
        onChange={next => void flip(next, setIsPublic, () => saveSettingsFn({ data: { domain, public: next } }))}>{t("公开这个站点的仪表盘")}</Toggle>}>
        <LinkRow url={`${origin}/sites/${encodeURIComponent(domain)}`} enabled={isPublic} />
      </Tile>
      <Tile icon="plug" title={t("共享链接")} actions={<Toggle hideLabel checked={shared} disabled={busy || !share}
        onChange={next => void flip(next, setShared, () => setShareFn({ data: { domain, enabled: next } }))}>{t("开启共享链接")}</Toggle>}>
        {share ? <LinkRow url={`${origin}/share/${share.slug}`} enabled={shared} /> : null}
      </Tile>
    </div>
  )
}
