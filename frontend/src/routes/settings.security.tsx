import { createFileRoute, getRouteApi, useRouter } from "@tanstack/react-router"
import { AccountSecurity } from "~/components/AccountSecurity"
import { PasskeyPanel } from "~/components/PasskeyPanel"

const settings = getRouteApi("/settings")

export const Route = createFileRoute("/settings/security")({ component: SecurityPage })

function SecurityPage() {
  const { data } = settings.useLoaderData()
  const router = useRouter()
  return <><AccountSecurity user={data.user} onSaved={() => router.invalidate()} /><PasskeyPanel /></>
}
