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
    </div>
  )
}
