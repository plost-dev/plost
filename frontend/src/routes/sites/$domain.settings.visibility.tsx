import { ActionButton } from "~/components/ActionButton"
import { SettingsForm, Field, FormActions } from "~/components/SettingsForm"
import { Link, createFileRoute, getRouteApi, useRouter } from "@tanstack/react-router"
import { useState } from "react"
import { Toggle } from "~/components/Toggle"
import { Tile } from "~/components/ui"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { createShareFn, removeShareFn, saveSettingsFn } from "~/lib/actions"

const settings = getRouteApi("/sites/$domain/settings")

export const Route = createFileRoute("/sites/$domain/settings/visibility")({ component: VisibilityPage })

function VisibilityPage() {
  const { data } = settings.useLoaderData()
  const { domain } = settings.useParams()
  const router = useRouter()
  const { t } = useT()
  const [isPublic, setIsPublic] = useState(!!data.site.public)
  const [shareName, setShareName] = useState(t("公开报表"))

  return (
    <div>
      <Tile icon="eye" title={t("公开仪表盘")}>
        <SettingsForm onSubmit={async event => {
          event.preventDefault()
          await saveSettingsFn({data:{domain,public:isPublic}})
          await router.invalidate()
          showToast("设置已保存")
        }}>
          <Toggle checked={isPublic} onChange={setIsPublic}>{t("公开这个站点的仪表盘")}</Toggle>
          <FormActions><ActionButton className="btn btn-primary" type="submit">{t("保存设置")}</ActionButton></FormActions>
        </SettingsForm>
      </Tile>
      <Tile icon="plug" title={t("共享链接")}>
        <p className="mb-4 text-sm"><Link to="/settings/api" className="text-indigo-600 hover:underline">{t("API")} · {t("在线人数")} / UV / PV</Link></p>
        <SettingsForm
          className="mb-4"
          onSubmit={async event => {
            event.preventDefault()
            await createShareFn({ data: { domain, name: shareName } })
            setShareName(t("公开报表"))
            await router.invalidate()
          }}
        >
          <Field label={t("共享链接名称")}><input className="squircle input" value={shareName} onChange={event => setShareName(event.target.value)} /></Field>
          <FormActions><ActionButton className="btn btn-primary" type="submit">{t("创建")}</ActionButton></FormActions>
        </SettingsForm>
        <ul className="settings-share-list space-y-2 text-sm">
          {data.shares.map(share => (
            <li key={share.id} className="settings-list-row flex min-w-0 flex-wrap justify-between gap-3">
              {share.slug
                ? <a className="min-w-0 flex-1 text-indigo-600 hover:text-indigo-500" href={`/share/${share.slug}`}><span className="min-w-0 [overflow-wrap:anywhere]">{share.name} · /share/{share.slug}</span></a>
                : <span className="min-w-0 flex-1">{share.name}</span>}
              <ActionButton className="shrink-0 text-red-600" type="button" onClick={async () => { await removeShareFn({ data: { domain, id: share.id } }); await router.invalidate() }}>{t("删除")}</ActionButton>
            </li>
          ))}
        </ul>
      </Tile>
    </div>
  )
}
