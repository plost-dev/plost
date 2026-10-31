import { createFileRoute, redirect, useRouter } from "@tanstack/react-router"
import { useState } from "react"
import { Shell } from "~/components/Shell"
import { ManagementHeader } from "~/components/SettingsChrome"
import { PageState } from "~/components/PageState"
import { ActionButton, ActionLink } from "~/components/ActionButton"
import { SiteHealthPanel } from "~/components/SiteHealthPanel"
import { meFn, siteCheckFn, runSiteCheckFn } from "~/lib/actions"
import { useT } from "~/lib/i18n"
import { useRouterAutoRefresh } from "~/lib/router-refresh"

export const Route = createFileRoute("/sites/$domain/health")({
  loader: async ({params}) => {
    const me = await meFn()
    if (!me) throw redirect({to: "/login"})
    return {me, data: await siteCheckFn({data: {domain: params.domain}})}
  },
  component: SiteHealthPage,
  errorComponent: ({error}) => <PageState title="页面出错" description={error instanceof Error ? error.message : undefined}/>,
})

function SiteHealthPage() {
  const {me, data} = Route.useLoaderData()
  const {domain} = Route.useParams()
  const {t} = useT()
  const router = useRouter()
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  useRouterAutoRefresh({enabled: !pending, onError: () => setError(t("刷新失败，请重试"))})
  async function perform(action: () => Promise<unknown>) {
    setPending(true); setError("")
    try { await action(); await router.invalidate() }
    catch (error) {setError(error instanceof Error ? error.message : t("检查失败，请稍后重试"))}
    finally {setPending(false)}
  }
  return <Shell user={me}>
    <ManagementHeader title={t("站点检查")} description={domain} icon="shield">
      <ActionLink to="/sites/$domain" params={{domain}}>{t("返回统计")}</ActionLink>
      {data.canManage && data.report.enabled ? <ActionButton className="btn-primary" disabled={pending} onClick={() => void perform(() => runSiteCheckFn({data: {domain}}))}>{t(pending ? "检查中…" : "立即检查")}</ActionButton> : null}
      {data.canManage && !data.report.enabled ? <ActionLink to="/sites/$domain/settings" params={{domain}} search={{tab:"general"}} className="btn-primary">{t("开启检查")}</ActionLink> : null}
    </ManagementHeader>
    {error ? <p role="alert" className="squircle settings-feedback is-error">{t(error)}</p> : null}
    <SiteHealthPanel report={data.report}/>
  </Shell>
}
