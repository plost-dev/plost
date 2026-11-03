import { createFileRoute, getRouteApi, useRouter } from "@tanstack/react-router"
import { AccountPreferences } from "~/components/AccountPreferences"
import { SettingsTips } from "~/components/SettingsTips"

const settings = getRouteApi("/settings")

export const Route = createFileRoute("/settings/")({ component: ProfilePage })

function ProfilePage() {
  const { data } = settings.useLoaderData()
  const router = useRouter()
  return <><AccountPreferences user={data.user} onSaved={() => router.invalidate()} /><SettingsTips tips={[
    "昵称、头像和界面语言点「保存设置」后生效；邮箱在这里只读，更改邮箱请到「安全」页。",
  ]} /></>
}
