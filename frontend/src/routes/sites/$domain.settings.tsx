import { Surface } from "~/components/Surface"
import { ActionLink } from "~/components/ActionButton"
import { SettingsLayout, SettingsNav } from "~/components/SettingsLayout"
import { Link, Outlet, createFileRoute, redirect, useLocation, useRouter } from "@tanstack/react-router"
import { useEffect } from "react"
import { SettingsHeader } from "~/components/SettingsChrome"
import { Shell } from "~/components/Shell"
import { useT } from "~/lib/i18n"
import { meFn, siteSettingsFn } from "~/lib/actions"

// Each page has its own static address; the general page is /settings itself.
const PAGES = [
  { to: "/sites/$domain/settings", label: "常规" },
  { to: "/sites/$domain/settings/visibility", label: "可见性" },
  { to: "/sites/$domain/settings/installation", label: "安装脚本" },
  { to: "/sites/$domain/settings/danger-zone", label: "危险操作" },
] as const
export const Route = createFileRoute("/sites/$domain/settings")({
  // Switching pages keeps the site for a minute instead of asking again; saving calls router.invalidate().
  staleTime: 60_000,
  preloadStaleTime: 60_000,
  loader: async ({ params }) => {
    // Both at once: each is a round trip to the server.
    const settings = siteSettingsFn({ data: { domain: params.domain } })
    settings.catch(() => {})
    const me = await meFn()
    if (!me) throw redirect({ to: "/login" })
    return { me, data: await settings }
  },
  component: SettingsPage,
})

function SettingsPage() {
  const { me, data } = Route.useLoaderData()
  const { domain } = Route.useParams()
  const pathname = useLocation({ select: location => location.pathname.replace(/\/+$/, "") })
  const { t } = useT()
  const router = useRouter()
  // Every page's code is fetched once the layout is up, so switching only swaps the content.
  useEffect(() => {
    for (const route of Object.values(router.routesById)) if (route.id.startsWith("/sites/$domain/settings/")) void router.loadRouteChunk(route)?.catch(() => {})
  }, [router])
  const page = PAGES.find(item => pathname === item.to.replace("$domain", encodeURIComponent(domain))) ?? PAGES[0]

  return (
    <Shell user={me}>
      <Surface radius="large" className="site-settings-workspace">
        <SettingsHeader
          title={t("settings.site_for")}
          domain={domain}
          name={data.site.name || ""}
          back={(
            <ActionLink to="/sites/$domain" params={{ domain }} className="btn-secondary settings-back">
              {t("settings.back_stats")}
            </ActionLink>
          )}
        />
        <SettingsLayout navigationTitle={t(page.label)} sidebar={<SettingsNav label={t("站点设置")}>
          {PAGES.map(item => <Link key={item.to} to={item.to} params={{ domain }} aria-current={page === item ? "page" : undefined} className={`squircle settings-nav-link${page === item ? " is-active" : ""}`}>{t(item.label)}</Link>)}
        </SettingsNav>}>
          <Outlet />
        </SettingsLayout>
      </Surface>
    </Shell>
  )
}
