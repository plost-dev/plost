import { createFileRoute, getRouteApi, useRouter } from "@tanstack/react-router"
import { AccountSecurity } from "~/components/AccountSecurity"
import { PasskeyPanel } from "~/components/PasskeyPanel"
import { SettingsTips } from "~/components/SettingsTips"

const settings = getRouteApi("/settings")

export const Route = createFileRoute("/settings/security")({ component: SecurityPage })

function SecurityPage() {
  const { data } = settings.useLoaderData()
  const router = useRouter()
  return <><AccountSecurity user={data.user} onSaved={() => router.invalidate()} /><PasskeyPanel /><SettingsTips tips={[
    "更改邮箱或密码都需要输入当前密码。密码至少 8 个字符，不限制字符类型。",
    "两步验证开启后，登录时在密码之后还要输入验证器里的 6 位数字。恢复码在丢失验证器时使用，每条只能用一次，请另外妥善保存。",
    "Passkey 用本机指纹、面容或安全密钥登录，无需输入密码。浏览器只允许在域名或 localhost 上创建，用 IP 地址打开时不能添加。",
  ]} /></>
}
