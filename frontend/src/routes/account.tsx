import { createFileRoute, redirect } from "@tanstack/react-router"

// The settings used to live at /account?tab=…; old links and bookmarks are sent on to the page's own address.
const LEGACY = {
  preferences: "/settings",
  security: "/settings/security",
  api: "/settings/api",
  "system/appearance": "/settings/appearance",
  "system/sparkline": "/settings/sparkline",
  "system/mapbox": "/settings/map",
  "system/geoip": "/settings/geoip",
  "system/backup": "/settings/backup",
  backup: "/settings/backup",
} as const

export const Route = createFileRoute("/account")({
  validateSearch: (search: Record<string, unknown>): { tab?: string; section?: string } => ({
    ...(typeof search.tab === "string" ? { tab: search.tab } : {}),
    ...(typeof search.section === "string" ? { section: search.section } : {}),
  }),
  beforeLoad: ({ search }) => {
    const tab = search.tab ?? search.section ?? ""
    throw redirect({ to: tab in LEGACY ? LEGACY[tab as keyof typeof LEGACY] : "/settings", replace: true })
  },
})
