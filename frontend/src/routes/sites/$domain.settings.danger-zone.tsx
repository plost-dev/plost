import { ActionButton } from "~/components/ActionButton"
import { Field, ConfirmModal } from "~/components/SettingsForm"
import { createFileRoute, getRouteApi, useRouter } from "@tanstack/react-router"
import { useState } from "react"
import { Tile } from "~/components/ui"
import { useT } from "~/lib/i18n"
import { removeSiteFn } from "~/lib/actions"

const settings = getRouteApi("/sites/$domain/settings")

export const Route = createFileRoute("/sites/$domain/settings/danger-zone")({ component: DangerZonePage })

function DangerZonePage() {
  const { domain } = settings.useParams()
  const router = useRouter()
  const { t } = useT()
  const [remove, setRemove] = useState(false)
  const [confirmation, setConfirmation] = useState("")

  return (
    <div>
      <div className="squircle settings-danger-notice">
        <h3 className="text-sm font-medium text-gray-900">{t("危险操作")}</h3>
        <p className="mt-1 text-sm text-gray-600">{t("下面的操作会造成无法恢复的数据丢失，请谨慎。")}</p>
      </div>
      <Tile icon="warn" title={t("删除站点")} subtitle={t("删除后会移除此站点及访问历史，无法撤销。")}>
        <ActionButton
          className="btn btn-danger"
          type="button"
          onClick={() => { setConfirmation(""); setRemove(true) }}
        >{t("删除站点")}</ActionButton>
      </Tile>
      <ConfirmModal
        open={remove}
        title={t("删除站点")}
        confirmLabel={t("确认删除")}
        danger
        onClose={() => setRemove(false)}
        onSubmit={async () => {
          if (confirmation !== domain) throw Error(t("请输入完整域名以确认删除"))
          await removeSiteFn({ data: { domain } })
          await router.navigate({ to: "/dashboard" })
        }}
      >
        <Field label={t("请输入完整域名以确认删除")}><input className="squircle input" value={confirmation} onChange={event => setConfirmation(event.target.value)} required /></Field>
      </ConfirmModal>
    </div>
  )
}
