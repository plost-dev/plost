import { createFileRoute, useRouter, type SearchSchemaInput } from "@tanstack/react-router"
import { useRef } from "react"
import { AnalyticsDashboard } from "~/components/AnalyticsDashboard"
import { AnalyticsDashboardSkeleton } from "~/components/DashboardSkeleton"
import { PageState } from "~/components/PageState"
import { Shell } from "~/components/Shell"
import { shareDashboardFn } from "~/lib/actions"
import { dashInput, parseDashSearch, type DashSearch } from "~/lib/range"
import { useRouterAutoRefresh } from "~/lib/router-refresh"
import { settle, useSettled } from "~/lib/settled"

export const Route = createFileRoute("/share/$slug")({
  validateSearch: (search: Partial<DashSearch> & SearchSchemaInput) => parseDashSearch(search),
  loaderDeps: ({ search }) => search,
  loader: ({ params, deps }) => ({
    // The page renders a skeleton at once; the report fills in when this settles.
    data: settle(shareDashboardFn({ data: { ...dashInput(params.slug, deps), slug: params.slug } })),
  }),
  component: SharePage,
  errorComponent: ({ error }) => (
    <PageState title="分享链接无效" description={error instanceof Error ? error.message : undefined} />
  ),
})

function SharePage() {
  const { slug } = Route.useParams()
  return <ShareContent key={slug} slug={slug} />
}

function ShareContent({ slug }: { slug: string }) {
  const { data } = Route.useLoaderData()
  const search = Route.useSearch()
  const router = useRouter()
  const view = useSettled(data, JSON.stringify(search))
  const loading = useRef(false)
  loading.current = view.pending
  const shownSearch = useRef(search)
  if (!view.switching) shownSearch.current = search
  useRouterAutoRefresh({ skip: () => loading.current })
  const onSearch = (next: DashSearch) => { void router.navigate({to: "/share/$slug", params: {slug}, search: next}) }
  const { result } = view
  if (result && !result.ok) return <PageState title="分享链接无效" description={result.message} />
  return <Shell layout="analytics">
    {result?.ok
      ? <AnalyticsDashboard data={result.value} domain={result.value.site.domain} search={search} dataSearch={view.switching ? shownSearch.current : search} loading={view.switching} onSearch={onSearch} />
      : <AnalyticsDashboardSkeleton domain="" search={search} onSearch={onSearch} />}
  </Shell>
}
