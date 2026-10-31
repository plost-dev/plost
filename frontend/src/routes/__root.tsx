import { createRootRoute, Outlet, useRouterState } from "@tanstack/react-router"
import { useEffect, type ReactNode } from "react"
import { I18nProvider, isLocale, useT } from "~/lib/i18n"
import { rootPreferencesFn } from "~/lib/auth-actions"
import { MapStyleProvider } from "~/lib/map-preferences"
import { DEFAULT_MAPBOX_STYLE } from "~/lib/mapbox-styles"
import { pageTitle } from "~/lib/page-title"
import { syncAccentFavicon, subscribeAccentColor } from "~/lib/accent-preference"
import { PageState } from "~/components/PageState"
import { RouteErrorPage } from "~/components/RouteErrorPage"
import { Toaster } from "~/components/Toaster"

export const Route = createRootRoute({
  loader: rootPreferencesFn,
  shellComponent: RootShell,
  component: Root,
  errorComponent: RouteErrorPage,
  notFoundComponent: () => <PageState title="页面不存在" />,
})

function Root() {
  useEffect(() => {
    syncAccentFavicon()
    return subscribeAccentColor(() => {})
  }, [])
  return <Outlet />
}

function PageTitle() {
  const pathname = useRouterState({ select: state => state.location.pathname })
  const { locale } = useT()
  useEffect(() => {
    document.title = pageTitle(pathname, locale)
  }, [pathname, locale])
  return null
}

export function RootShell({ children }: { children: ReactNode }) {
  // The shell also wraps errors raised before the root loader has preferences.
  // index.html owns the SPA document; this shell only supplies shared providers.
  const preferences = Route.useLoaderData()
  const locale = isLocale(preferences?.locale) ? preferences.locale : "zh-CN"
  return (
    <I18nProvider locale={locale}>
      <MapStyleProvider styleId={preferences?.mapboxStyle ?? DEFAULT_MAPBOX_STYLE}>
        <PageTitle />
        {children}
        <Toaster />
      </MapStyleProvider>
    </I18nProvider>
  )
}
