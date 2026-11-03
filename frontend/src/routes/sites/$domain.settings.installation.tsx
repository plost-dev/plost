import { SettingsTips } from "~/components/SettingsTips"
import { TrackingDocs } from "~/components/TrackingDocs"
import { TrackingSnippet } from "~/components/TrackingSnippet"
import { Link, createFileRoute, getRouteApi } from "@tanstack/react-router"
import { useEffect, useState } from "react"
import { Tile } from "~/components/ui"
import { useT } from "~/lib/i18n"

const settings = getRouteApi("/sites/$domain/settings")

export const Route = createFileRoute("/sites/$domain/settings/installation")({ component: InstallationPage })

function InstallationPage() {
  const { domain } = settings.useParams()
  const { t } = useT()
  const [origin, setOrigin] = useState("")
  useEffect(() => { setOrigin(window.location.origin) }, [])
  const snippet = `<script defer data-domain="${domain}" src="${origin}/script.js"></script>`

  return (
    <div>
      <Tile icon="code" title={t("埋点脚本")}>
        <TrackingSnippet code={snippet} />
        <TrackingDocs domain={domain} />
        <p className="mt-4 text-sm"><Link to="/settings/api" className="text-indigo-600 hover:underline">{t("API")} · {t("在线人数")} / UV / PV</Link></p>
      </Tile>
      <SettingsTips tips={[
        "把这段代码放到网站的 <head> 里。默认会记录页面浏览、外链点击、文件下载，以及标题、语言和屏幕分辨率。",
        "页面完全加载完才计一次浏览，单页应用切换路由、后退前进也会计入；自动化浏览器和常见爬虫不计。",
        "不论直连，还是经过反向代理、Cloudflare 或其他 CDN，访客的 IP 和国家都会自动识别，不用额外设置。",
      ]} />
    </div>
  )
}
