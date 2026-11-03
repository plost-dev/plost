import { createFileRoute, Link, redirect, useRouter } from "@tanstack/react-router"
import { useEffect } from "react"
import { AccentPicker } from "~/components/AccentPicker"
import { AccountPreferences } from "~/components/AccountPreferences"
import { AccountSecurity } from "~/components/AccountSecurity"
import { ActionLink } from "~/components/ActionButton"
import { ApiKeyPanel } from "~/components/ApiKeyPanel"
import { PasskeyPanel } from "~/components/PasskeyPanel"
import { BackupPanel } from "~/components/BackupPanel"
import { SparklineStylePicker } from "~/components/SparklineStylePicker"
import { SettingsTips } from "~/components/SettingsTips"
import { loadMapServices, MapServicesPage } from "~/components/MapServicesSettings"
import { SettingsHeader } from "~/components/SettingsChrome"
import { SettingsLayout, SettingsNav } from "~/components/SettingsLayout"
import { Shell } from "~/components/Shell"
import { Tile } from "~/components/ui"
import { UIIcon as TabIcon } from "~/components/UIIcon"
import { accountSettingsFn } from "~/lib/actions"
import { useT } from "~/lib/i18n"

const ACCOUNT_TABS = [
  { id: "preferences", label: "个人资料" },
  { id: "security", label: "安全" },
  { id: "api", label: "API" },
]
const SYSTEM_TABS = [
  { id: "system/appearance", label: "界面外观" },
  { id: "system/sparkline", label: "站点走势图" },
  { id: "system/mapbox", label: "访客地图" },
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
  // The loader does not depend on the tab, so switching tabs neither waits for the server nor remounts the page; each
  // tab loads its own data.
  loader: async () => {
    try {
      return { data: await accountSettingsFn() }
    } catch {
      throw redirect({ to: "/login" })
    }
  },
  component: AccountPage,
})

function AccountPage() {
  const { data } = Route.useLoaderData()
  const { tab } = Route.useSearch()
  const router = useRouter()
  const { t } = useT()
  const user = data.user
  useEffect(() => { void loadMapServices().catch(() => {}) }, [])

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
          {tab === "preferences" ? <><AccountPreferences user={user} onSaved={() => router.invalidate()} /><SettingsTips tips={[
            "昵称、头像和界面语言点「保存设置」后生效；邮箱在这里只读，更改邮箱请到「安全」页。",
          ]} /></> : null}
          {tab === "security" ? <><AccountSecurity user={user} onSaved={() => router.invalidate()} /><PasskeyPanel /><SettingsTips tips={[
            "更改邮箱或密码都需要输入当前密码。密码至少 8 个字符，不限制字符类型。",
            "两步验证开启后，登录时在密码之后还要输入验证器里的 6 位数字。恢复码在丢失验证器时使用，每条只能用一次，请另外妥善保存。",
            "Passkey 用本机指纹、面容或安全密钥登录，无需输入密码。浏览器只允许在域名或 localhost 上创建，用 IP 地址打开时不能添加。",
          ]} /></> : null}
          {tab === "api" ? <><ApiKeyPanel /><SettingsTips tips={[
            "API Key 只在创建时显示一次，请只保存在服务端的环境变量里，不要写进网页 JavaScript、公开仓库或网址参数。",
            "调用时在请求头加上 Authorization: Bearer <API Key>，请求 /api/v1/stats。完整的参数、返回字段和示例代码见官网 API 文档。",
            "不再使用时删除对应的 Key，调用会立即失效，不用改账号密码。删除站点时，它的 Key 也一起删除。",
          ]} /></> : null}
          {tab === "system/appearance" ? <><Tile icon="sliders" title="界面外观"><AccentPicker /></Tile><SettingsTips tips={[
            "界面配色立即生效，保存在当前浏览器，换一台电脑或浏览器需要重新选择。",
            "深浅色在页脚的主题按钮切换。",
          ]} /></> : null}
          {tab === "system/sparkline" ? <><Tile icon="activity" title="站点卡片走势图"><SparklineStylePicker /></Tile><SettingsTips tips={[
            "站点列表每张卡片下方的小走势图。样式和时间范围选择后自动保存到当前账号。",
            "走势图的 24 小时按小时、30 天按天显示访客。",
          ]} /></> : null}
          {tab === "system/mapbox" ? <MapServicesPage onSaved={() => router.invalidate()} /> : null}
          {tab === "system/backup" ? <BackupPanel /> : null}
        </SettingsLayout>
      </div>
    </Shell>
  )
}
