import { createFileRoute, useRouter } from "@tanstack/react-router"
import { MapServicesPage } from "~/components/MapServicesSettings"

export const Route = createFileRoute("/settings/map")({ component: MapPage })

function MapPage() {
  const router = useRouter()
  return <MapServicesPage onSaved={() => router.invalidate()} />
}
