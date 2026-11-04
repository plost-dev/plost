import { createFileRoute, getRouteApi, useRouter } from "@tanstack/react-router"
import { AccountPreferences } from "~/components/AccountPreferences"

const settings = getRouteApi("/settings")

export const Route = createFileRoute("/settings/")({ component: ProfilePage })

function ProfilePage() {
  const { data } = settings.useLoaderData()
  const router = useRouter()
  return <><AccountPreferences user={data.user} onSaved={() => router.invalidate()} /></>
}
