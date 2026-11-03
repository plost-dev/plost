import { createFileRoute } from "@tanstack/react-router"
import { GeoIPSettingsPage } from "~/components/GeoIPSettingsPanel"

export const Route = createFileRoute("/settings/geoip")({ component: GeoIPSettingsPage })
