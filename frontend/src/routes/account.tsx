import { createFileRoute, Link, redirect, useRouter } from "@tanstack/react-router"
import { AccountPreferences } from "~/components/AccountPreferences"
import { AccountSecurity } from "~/components/AccountSecurity"
import { ActionLink } from "~/components/ActionButton"
import { ApiDocsContent } from "~/components/ApiDocs"
import { PasskeyPanel } from "~/components/PasskeyPanel"
import { BackupPanel } from "~/components/BackupPanel"
import { MapboxSettingsPanel } from "~/components/MapboxSettingsPanel"
import { ClientIPPanel } from "~/components/ClientIPPanel"
import { GoogleMapsSettingsPanel } from "~/components/GoogleMapsSettingsPanel"
import { MapServiceTips } from "~/components/MapServiceTips"
import { SettingsHeader } from "~/components/SettingsChrome"
import { SettingsLayout, SettingsNav } from "~/components/SettingsLayout"
import { Shell } from "~/components/Shell"
import { UIIcon as TabIcon } from "~/components/UIIcon"
import { accountSettingsFn } from "~/lib/actions"
import { useT } from "~/lib/i18n"
import { mapboxSettingsFn } from "~/lib/mapbox-actions"
import { clientIPCheckFn } from "~/lib/client-ip-actions"
import { googleMapsSettingsFn } from "~/lib/google-maps-actions"

const ACCOUNT_TABS = [
  { id: "preferences", label: "个人资料" },
  { id: "security", label: "安全" },
  { id: "api", label: "API" },
]
const SYSTEM_TABS = [
  { id: "system/mapbox", label: "访客地图" },
  { id: "system/client-ip", label: "访客 IP 识别" },
  { id: "system/backup", label: "备份与恢复" },
]

function accountTab(search: Record<string, unknown>) {
  const legacy = search.section === "backup" ? "system/backup" : search.section
  const selected = search.tab || legacy
  const value = selected === "backup" ? "system/backup" : selected
  return [...ACCOUNT_TABS, ...SYSTEM_TABS].find(item => item.id === value)?.id || "preferences"
}

export const Route = createFileRoute("/account")({
  validateSearch: (search: Record<string, unknown>) => ({ tab: accountTab(search) }),
  loaderDeps: ({ search }) => search,
  loader: async ({ deps }) => {
    let data: Awaited<ReturnType<typeof accountSettingsFn>>
    try {
      data = await accountSettingsFn()
    } catch {
      throw redirect({ to: "/login" })
    }
    const [mapbox, googleMaps] = deps.tab === "system/mapbox" ? await Promise.all([mapboxSettingsFn(), googleMapsSettingsFn()]) : [null, null]
    const clientIP = deps.tab === "system/client-ip" ? await clientIPCheckFn() : null
    return { data, mapbox, googleMaps, clientIP }
  },
  component: AccountPage,
})

function AccountPage() {
  const { data, mapbox, googleMaps, clientIP } = Route.useLoaderData()
  const { tab } = Route.useSearch()
  const router = useRouter()
  const { t } = useT()
  const user = data.user

  return (
    <Shell user={user}>
      <div>
        <SettingsHeader
          title={t("settings.title")}
          back={(
            <ActionLink to="/dashboard" className="btn-secondary settings-back">
              {t("settings.back_sites")}
            </ActionLink>
          )}
        />
        <SettingsLayout navigationTitle={t([...ACCOUNT_TABS, ...SYSTEM_TABS].find(item => item.id === tab)?.label || "设置")} sidebar={<>
          <h3 className="settings-nav-group-title"><TabIcon name="user" />{t("账户")}</h3>
          <SettingsNav label={t("账户设置")}>
            {ACCOUNT_TABS.map(item => <Link key={item.id} to="/account" search={{ tab: item.id }} aria-current={tab === item.id ? "page" : undefined} className={`squircle settings-nav-link${tab === item.id ? " is-active" : ""}`}>{t(item.label)}</Link>)}
          </SettingsNav>
          <div className="mt-6">
            <h3 className="settings-nav-group-title"><TabIcon name="grid" />{t("系统设置")}</h3>
            <SettingsNav label={t("系统设置")}>
              {SYSTEM_TABS.map(item => <Link key={item.id} to="/account" search={{ tab: item.id }} aria-current={tab === item.id ? "page" : undefined} className={`squircle settings-nav-link${tab === item.id ? " is-active" : ""}`}>{t(item.label)}</Link>)}
            </SettingsNav>
          </div>
        </>}>
          {tab === "preferences" ? <AccountPreferences user={user} onSaved={() => router.invalidate()} /> : null}
          {tab === "security" ? <><AccountSecurity user={user} onSaved={() => router.invalidate()} /><PasskeyPanel /></> : null}
          {tab === "api" ? <ApiDocsContent embedded /> : null}
          {/* Turning one map service on turns the other off, so both cards reload after a save. */}
          {tab === "system/mapbox" && mapbox ? <MapboxSettingsPanel key={`mapbox-${mapbox.enabled}`} initial={mapbox} onSaved={() => router.invalidate()} /> : null}
          {tab === "system/mapbox" && googleMaps ? <GoogleMapsSettingsPanel key={`google-${googleMaps.enabled}`} initial={googleMaps} onSaved={() => router.invalidate()} /> : null}
          {tab === "system/mapbox" ? <MapServiceTips /> : null}
          {tab === "system/client-ip" && clientIP ? <ClientIPPanel initial={clientIP} /> : null}
          {tab === "system/backup" ? <BackupPanel /> : null}
        </SettingsLayout>
      </div>
    </Shell>
  )
}
