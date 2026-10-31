import { Navigate, createFileRoute, redirect, useRouter, type SearchSchemaInput } from "@tanstack/react-router"
import { useRef } from "react"
import { AnalyticsDashboard } from "~/components/AnalyticsDashboard"
import { AnalyticsDashboardSkeleton } from "~/components/DashboardSkeleton"
import { PageState } from "~/components/PageState"
import { Shell } from "~/components/Shell"
import { dashboardFn, meFn } from "~/lib/actions"
import { compactDashSearch, dashInput, parseDashSearch, type DashSearch } from "~/lib/range"
import { useRouterAutoRefresh } from "~/lib/router-refresh"
import { settle, useSettled } from "~/lib/settled"

export const Route = createFileRoute("/sites/$domain/")({
  validateSearch: (search: Partial<DashSearch> & SearchSchemaInput) => parseDashSearch(search),
  search: { middlewares: [({search,next}) => compactDashSearch(next(search))] },
  beforeLoad: ({params,location}) => {
    const raw = location.search as Record<string, unknown>
    const clean = compactDashSearch(raw)
    if (Object.keys(raw).some(key => raw[key] !== clean[key])) {
      throw redirect({to:"/sites/$domain",params:{domain:params.domain},search:clean,replace:true})
    }
  },
  loaderDeps: ({ search }) => search,
  loader: async ({ params, deps }) => {
    // Start the heavy data without waiting for it: the page renders a skeleton at once and fills in when it settles.
    const data = settle(dashboardFn({ data: dashInput(params.domain, deps) }))
    return { me: await meFn(), data }
  },
  component: Dashboard,
  errorComponent: ({ error }) => (
    <PageState title="页面出错" description={error instanceof Error ? error.message : undefined} />
  ),
})

function Dashboard() {
  const { domain } = Route.useParams()
  // Another site must start from a skeleton instead of dimming the previous site's numbers.
  return <DashboardContent key={domain} domain={domain} />
}

function DashboardContent({ domain }: { domain: string }) {
  const { me, data } = Route.useLoaderData()
  const search = Route.useSearch()
  const router = useRouter()
  const view = useSettled(data, JSON.stringify(search))
  const loading = useRef(false)
  loading.current = view.pending
  // The live visitor count and today's totals follow quickly; the other pages keep the slower default.
  useRouterAutoRefresh({ intervalMs: 10_000, skip: () => loading.current })
  const onSearch = (next: DashSearch) => { void router.navigate({to: "/sites/$domain", params: {domain}, search: next}) }
  const { result } = view
  if (result && !result.ok) return <PageState title="页面出错" description={result.message} />
  if (result?.ok && !me && !result.value.site.public) return <Navigate to="/login" />
  return <Shell user={me} layout="analytics">
    {result?.ok
      ? <AnalyticsDashboard data={result.value} domain={domain} search={search} loading={view.switching}
        readonly={!result.value.access.member} canManage={result.value.access.canManage} onSearch={onSearch} />
      : <AnalyticsDashboardSkeleton domain={domain} search={search} readonly={!me} onSearch={onSearch} />}
  </Shell>
}
