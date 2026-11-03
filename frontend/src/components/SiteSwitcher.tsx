import { UIIcon } from "~/components/UIIcon"
import { useT } from "~/lib/i18n"
import { Link } from "@tanstack/react-router"
import { useEffect, useRef, useState } from "react"
import { siteDomainsFn } from "~/lib/actions"
import { SiteFavicon } from "./SiteFavicon"

export function SiteSwitcher({
  domain,
  readonly,
  canManage = !readonly,
}: {
  domain: string
  readonly?: boolean
  canManage?: boolean
}) {
  const { t } = useT()
  const [open, setOpen] = useState(false)
  const [sites, setSites] = useState<Array<{ domain: string, name?: string | null }>>([])
  const box = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (!box.current?.contains(e.target as Node)) setOpen(false)
    }
    window.addEventListener("mousedown", onClick)
    return () => window.removeEventListener("mousedown", onClick)
  }, [])

  useEffect(() => {
    if (readonly) return
    void siteDomainsFn().then(setSites).catch(() => setSites([]))
  }, [readonly])

  useEffect(() => {
    if (readonly) return
    function onKey(e: KeyboardEvent) {
      const tag = (e.target as HTMLElement)?.tagName
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return
      if (e.metaKey || e.ctrlKey || e.altKey) return
      const n = Number(e.key)
      if (n >= 1 && n <= 8 && sites[n - 1]) {
        e.preventDefault()
        window.location.assign(`/sites/${encodeURIComponent(sites[n - 1].domain)}`)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [readonly, sites])

  const currentSite = sites.find((s) => s.domain === domain)
  const displayName = currentSite?.name || domain
  const fullTitle = currentSite?.name && currentSite.name.toLowerCase() !== domain.toLowerCase() ? `${currentSite.name} (${domain})` : domain

  if (readonly) {
    return (
      <div className="squircle control-trigger flex items-center gap-x-1.5 rounded-md pr-2.5 pl-0 text-sm font-medium text-gray-700" title={fullTitle}>
        <SiteFavicon domain={domain} className="size-4" />
        <span className="hidden truncate font-semibold sm:mr-1 sm:block lg:mr-0">{displayName}</span>
      </div>
    )
  }

  return (
    <div className="relative" ref={box}>
      <button
        type="button"
        title={fullTitle}
        aria-expanded={open}
        className="squircle control-trigger flex items-center gap-x-1.5 rounded-md !pl-1.5 pr-2.5 text-sm font-medium text-gray-700 hover:bg-gray-150/80"
        onClick={() => setOpen((v) => !v)}
      >
        <SiteFavicon domain={domain} className="size-4" />
        <span className="hidden truncate font-semibold sm:mr-1 sm:block lg:mr-0">{displayName}</span>
        <UIIcon name="chevron-down" className="ui-chevron hidden lg:block" />
      </button>
      {open ? (
        <div className="squircle dashboard-popover absolute left-0 z-20 mt-2 flex w-[min(calc(100vw-2rem),20rem)] origin-top-left flex-col gap-0.5 rounded-md bg-white p-1 font-medium text-gray-800 shadow-lg ring-1 ring-black/5">
          <div className="flex">
            <Link
              to="/dashboard"
              className="squircle mx-1 my-1 flex flex-1 items-center justify-center rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-all duration-200 hover:border-gray-400/70 hover:text-gray-900"
              onClick={() => setOpen(false)}
            >
              {t("返回站点")}</Link>
            {canManage ? <Link
              to="/sites/$domain/settings/{-$tab}"
              params={{ domain }}
              className="squircle mx-1 my-1 flex flex-1 items-center justify-center rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-all duration-200 hover:border-gray-400/70 hover:text-gray-900"
              onClick={() => setOpen(false)}
            >
              {t("站点设置")}</Link> : null}
          </div>
          <div className="my-1 h-px bg-gray-200" />
          {sites.map((site, index) => (
            <Link
              key={site.domain}
              to="/sites/$domain"
              params={{ domain: site.domain }}
              data-selected={site.domain === domain}
              aria-current={site.domain === domain ? "page" : undefined}
              className="squircle control-menu-item flex items-center justify-between rounded-md px-4 py-2.5 text-sm leading-tight hover:bg-gray-100 hover:text-gray-900"
              onClick={() => setOpen(false)}
            >
              <span className="flex min-w-0 items-center">
                <SiteFavicon domain={site.domain} className="mr-2 size-4" />
                <span className="truncate">{site.name || site.domain}</span>
                {site.name && site.name.toLowerCase() !== site.domain.toLowerCase() ? (
                  <span className="selection-detail ml-2 text-xs font-normal text-gray-400 truncate">{site.domain}</span>
                ) : null}
              </span>
              {sites.length > 1 && index < 8 ? <kbd className="ml-2 text-xs font-normal text-gray-400">{index + 1}</kbd> : null}
            </Link>
          ))}
        </div>
      ) : null}
    </div>
  )
}
