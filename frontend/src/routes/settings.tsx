import { createFileRoute, Link, Outlet, redirect, useLocation } from "@tanstack/react-router"
import { useEffect } from "react"
import { ActionLink } from "~/components/ActionButton"
import { loadMapServices } from "~/components/MapServicesSettings"
import { SettingsHeader } from "~/components/SettingsChrome"
import { SettingsLayout, SettingsNav } from "~/components/SettingsLayout"
import { Shell } from "~/components/Shell"
import { UIIcon as TabIcon } from "~/components/UIIcon"
import { accountSettingsFn } from "~/lib/actions"
import { useT } from "~/lib/i18n"

// Each page has its own static address; the profile page is /settings itself.
const ACCOUNT_PAGES = [
  { to: "/settings", label: "个人资料" },
  { to: "/settings/security", label: "安全" },
  { to: "/settings/api", label: "API" },
] as const
const SYSTEM_PAGES = [
  { to: "/settings/appearance", label: "界面外观" },
  { to: "/settings/sparkline", label: "站点走势图" },
  { to: "/settings/map", label: "访客地图" },
  { to: "/settings/geoip", label: "IP 数据库" },
  { to: "/settings/backup", label: "备份与恢复" },
] as const

export const Route = createFileRoute("/settings")({
  // The pages share this loader, so switching between them neither waits for the server nor remounts the layout; each
  // page loads its own data.
  loader: async () => {
    try {
      return { data: await accountSettingsFn() }
    } catch {
      throw redirect({ to: "/login" })
    }
  },
  component: SettingsPage,
})

function SettingsPage() {
  const { data } = Route.useLoaderData()
  const pathname = useLocation({ select: location => location.pathname.replace(/\/+$/, "") || "/" })
  const { t } = useT()
  useEffect(() => { void loadMapServices().catch(() => {}) }, [])
  const navLink = (item: { to: typeof ACCOUNT_PAGES[number]["to"] | typeof SYSTEM_PAGES[number]["to"]; label: string }) => {
    const active = pathname === item.to
    return <Link key={item.to} to={item.to} aria-current={active ? "page" : undefined} className={`squircle settings-nav-link${active ? " is-active" : ""}`}>{t(item.label)}</Link>
  }

  return (
    <Shell user={data.user}>
      <div>
        <SettingsHeader
          title={t("settings.title")}
          back={(
            <ActionLink to="/dashboard" className="btn-secondary settings-back">
              {t("settings.back_sites")}
            </ActionLink>
          )}
        />
        <SettingsLayout navigationTitle={t([...ACCOUNT_PAGES, ...SYSTEM_PAGES].find(item => item.to === pathname)?.label || "设置")} sidebar={<>
          <h3 className="settings-nav-group-title"><TabIcon name="user" />{t("账户")}</h3>
          <SettingsNav label={t("账户设置")}>{ACCOUNT_PAGES.map(navLink)}</SettingsNav>
          <div className="mt-6">
            <h3 className="settings-nav-group-title"><TabIcon name="grid" />{t("系统设置")}</h3>
            <SettingsNav label={t("系统设置")}>{SYSTEM_PAGES.map(navLink)}</SettingsNav>
          </div>
        </>}>
          <Outlet />
        </SettingsLayout>
      </div>
    </Shell>
  )
}
