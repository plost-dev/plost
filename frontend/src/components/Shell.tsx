import { UIIcon } from "./UIIcon"
import { UserAvatar } from "./UserAvatar"
import { Link, useRouter, useRouterState, type AnyRouter } from "@tanstack/react-router"
import { useEffect, useRef, useState, useSyncExternalStore } from "react"
import { BrandMark } from "./BrandMark"
import { logoutFn, trafficPulseFn, updateProfileFn } from "~/lib/actions"
import { numberShort } from "~/lib/format"
import { useT } from "~/lib/i18n"
import { createThemeUpdate } from "~/lib/theme-update"

type ShellUser = {
  isAdmin?: boolean
  name: string
  email: string
  avatar?: string | null
  gravatarUrl?: string
  theme?: string
  locale?: string
} | null | undefined

type TrafficPulse = {
  sites: { count: number, visitors: number, visits: number, pageviews: number }
}

function fill(template: string, n: number) {
  return template.replace("{n}", numberShort(n))
}

function FooterPulse({ user }: { user?: ShellUser }) {
  const { t } = useT()
  const [pulse, setPulse] = useState<TrafficPulse | null>(null)

  useEffect(() => {
    if (!user) return
    let alive = true
    const load = () => {
      void trafficPulseFn().then((data) => {
        if (alive && data) setPulse(data)
      }).catch(() => {})
    }
    load()
    const timer = window.setInterval(load, 30_000)
    return () => {
      alive = false
      window.clearInterval(timer)
    }
  }, [user])

  if (!user || !pulse) return <span />

  return (
    <Link to="/dashboard" className="site-footer-pulse">
      <span>{fill(t("footer.sites"), pulse.sites.count)}</span>
      <span className="site-footer-pulse-dot" aria-hidden="true">·</span>
      <span>{fill(t("footer.today_visitors"), pulse.sites.visitors)}</span>
      <span className="site-footer-pulse-dot" aria-hidden="true">·</span>
      <span>{fill(t("footer.today_visits"), pulse.sites.visits)}</span>
      <span className="site-footer-pulse-dot" aria-hidden="true">·</span>
      <span>{fill(t("footer.today_views"), pulse.sites.pageviews)}</span>
    </Link>
  )
}

export function SiteFooter({ user }: { user?: ShellUser }) {
  const { t } = useT()
  return (
    <footer className="site-footer">
      <div className="container-ls site-footer-inner">
        <p className="site-footer-meta">
          <span className="footer-copyright">© {new Date().getFullYear()} Plost</span>
          <span aria-hidden="true">·</span>
          <span>{t("footer.own")}</span>
        </p>
        <FooterPulse user={user} />
        <div className="site-footer-links">
          <a href="https://github.com/plost-dev" target="_blank" rel="noopener noreferrer" className="site-footer-github" aria-label="GitHub" title="GitHub">
            <svg className="site-footer-github-mark" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path fillRule="evenodd" d="M8 0C13.76 0 16 2.24 16 8C16 13.76 13.76 16 8 16C2.24 16 0 13.76 0 8C0 2.24 2.24 0 8 0ZM5.47 15.59C5.87 15.66 6.02 15.42 6.02 15.21C6.02 15.02 6.01 14.39 6.01 13.72C4 14.09 3.48 13.23 3.32 12.78C3.23 12.55 2.84 11.84 2.5 11.65C2.22 11.5 1.82 11.13 2.49 11.12C3.12 11.11 3.57 11.7 3.72 11.94C4.44 13.15 5.59 12.81 6.05 12.6C6.12 12.08 6.33 11.73 6.56 11.53C4.78 11.33 2.92 10.64 2.92 7.58C2.92 6.71 3.23 5.99 3.74 5.43C3.66 5.23 3.38 4.41 3.82 3.31C3.82 3.31 4.49 3.1 6.02 4.13C6.66 3.95 7.34 3.86 8.02 3.86C8.7 3.86 9.38 3.95 10.02 4.13C11.55 3.09 12.22 3.31 12.22 3.31C12.66 4.41 12.38 5.23 12.3 5.43C12.81 5.99 13.12 6.7 13.12 7.58C13.12 10.65 11.25 11.33 9.47 11.53C9.76 11.78 10.01 12.26 10.01 13.01C10.01 14.08 10 14.94 10 15.21C10 15.42 10.15 15.67 10.55 15.59Z" />
            </svg>
          </a>
          <a href="https://plo.st" target="_blank" rel="noopener noreferrer" className="site-footer-brand">
            <BrandMark />
          </a>
        </div>
      </div>
    </footer>
  )
}

export function Shell({
  user,
  children,
  layout = "management",
}: {
  user?: ShellUser
  children: React.ReactNode
  layout?: "management" | "analytics"
}) {
  // The URL changes before the next loader completes; keep the visible page's
  // layout until its component is replaced instead of styling it as the target.
  return (
    <div className={`flex min-h-full flex-col${layout === "management" ? " management-shell" : ""}`}>
      <SiteHeader user={user} />
      {/* The space under the last card equals the space between the header and the first one. */}
      <main className={`container-ls flex-1 ${layout === "management" ? "pb-8" : "pb-4"}`}>{children}</main>
      <SiteFooter user={user} />
    </div>
  )
}

function headerNav(t: (key: string) => string): HeaderNavItem[] {
  const items: HeaderNavItem[] = [
    { key: "sites", to: "/dashboard", label: t("nav.sites") },
    { key: "account", to: "/account", search: { tab: "preferences" }, label: t("settings.title") },
  ]
  return items
}

function navActive(key: string, pathname: string) {
  if (key === "sites") return pathname === "/dashboard" || pathname.startsWith("/sites/") || pathname === "/sites" || pathname.startsWith("/share/")
  if (key === "account") return pathname === "/account"
  if (key === "backup") return pathname === "/backup"
  if (key === "api") return pathname === "/docs/api"
  return false
}

type HeaderNavItem = {
  key: string
  to: "/dashboard" | "/account" | "/backup" | "/docs/api"
  search?: { tab: string }
  label: string
}

function HeaderNav({
  items,
  pathname,
}: {
  items: HeaderNavItem[]
  pathname: string
}) {
  const { t } = useT()
  return (
    <nav className="site-header-nav" aria-label={t("主导航")}>
      {items.map((item) => (
        <Link
          key={item.key}
          to={item.to}
          search={item.search}
          aria-current={navActive(item.key, pathname) ? "page" : undefined}
          className={`squircle site-header-nav-link${navActive(item.key, pathname) ? " is-active" : ""}`}
        >
          {t(item.label)}
        </Link>
      ))}
    </nav>
  )
}

function ThemeSwitch({ theme, onChange, disabled }: { theme: string, disabled?: boolean, onChange: (next: string) => void }) {
  const { t } = useT()
  const [systemDark, setSystemDark] = useState(false)

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)")
    const sync = () => setSystemDark(mq.matches)
    sync()
    mq.addEventListener("change", sync)
    return () => mq.removeEventListener("change", sync)
  }, [])

  const resolved = theme === "dark" || (theme !== "light" && systemDark) ? "dark" : "light"

  const dark = resolved === "dark"
  return <button type="button" className={`squircle theme-toggle${dark ? " is-dark" : ""}`} disabled={disabled}
    aria-label={t(dark ? "切换到浅色模式" : "切换到深色模式")}
    title={t(dark ? "切换到浅色模式" : "切换到深色模式")}
    onClick={() => onChange(dark ? "light" : "dark")}>
    <UIIcon name="sun" className="theme-toggle-sun"/>
    <UIIcon name="moon" className="theme-toggle-moon"/>
  </button>
}

const themeUpdates = new WeakMap<AnyRouter, ReturnType<typeof createThemeUpdate>>()
function themeUpdateFor(router: AnyRouter) {
  let update = themeUpdates.get(router)
  if (!update) {
    update = createThemeUpdate(
      theme => updateProfileFn({data: {theme}}),
      async () => { router.clearCache(); await router.invalidate({sync: true}) },
    )
    themeUpdates.set(router, update)
  }
  return update
}

function HeaderTools({ user }: { user: NonNullable<ShellUser> }) {
  const { t } = useT()
  const update = themeUpdateFor(useRouter())
  const snapshot = useSyncExternalStore(update.subscribe, update.getSnapshot, () => undefined)
  const change = snapshot?.email === user.email ? snapshot : undefined
  const theme = change?.theme || user.theme || "system"
  useEffect(() => {
    const query = window.matchMedia("(prefers-color-scheme: dark)")
    const apply = () => { document.documentElement.dataset.theme = theme === "system" ? (query.matches ? "dark" : "light") : theme }
    apply()
    query.addEventListener("change", apply)
    return () => query.removeEventListener("change", apply)
  }, [theme])
  return (
    <div className="site-header-tools">
      <ThemeSwitch theme={theme} disabled={change?.pending} onChange={next => { void update.change(user.email, theme, next) }} />
      {change?.failed ? <span className="squircle header-feedback" role="alert">{t("外观保存失败，请重试")}</span> : null}
    </div>
  )
}

function SiteHeader({ user }: { user?: ShellUser }) {
  const { t } = useT()
  const location = useRouterState({ select: (s) => s.location })
  const pathname = location.pathname
  const items = user ? headerNav(t) : []

  return (
    <header className="site-header">
      <div className="container-ls">
        <div className="site-header-bar">
          <Link to={user ? "/dashboard" : "/"} className="site-header-brand">
            <BrandMark />
          </Link>
          {user ? <HeaderNav items={items} pathname={pathname} /> : <span className="hidden md:block" />}
          <div className="site-header-actions">
            {user ? (
              <>
                <HeaderTools user={user} />
                <UserMenu user={user} items={items} pathname={pathname} />
              </>
            ) : (
              <Link to="/login" className="squircle site-header-link">{t("nav.login")}</Link>
            )}
          </div>
        </div>
      </div>
    </header>
  )
}

function UserMenu({
  user,
  items,
  pathname,
}: {
  user: NonNullable<ShellUser>
  items: HeaderNavItem[]
  pathname: string
}) {
  const router = useRouter()
  const { t } = useT()
  const [open, setOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (!menuRef.current?.contains(e.target as Node)) setOpen(false)
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && menuRef.current?.querySelector('[role="menu"]')) {setOpen(false);triggerRef.current?.focus()}
    }
    window.addEventListener("mousedown", onClick)
    window.addEventListener("keydown", onKey)
    return () => {
      window.removeEventListener("mousedown", onClick)
      window.removeEventListener("keydown", onKey)
    }
  }, [])

  return (
    <div className="relative" ref={menuRef}>
      <button
        type="button"
        className="site-header-account"
        ref={triggerRef}
        aria-label={t("账户菜单")}
        aria-expanded={open}
        aria-haspopup="menu"
        onClick={() => setOpen((v) => !v)}
      >
        <UserAvatar avatar={user.avatar} gravatarUrl={user.gravatarUrl} name={user.name || user.email} className="site-header-avatar" />
      </button>
      {open ? (
        <div className="squircle site-header-menu" role="menu">
          {/* The menu bar is hidden on narrow screens, so its pages move in here; settings is the profile link below. */}
          <div className="site-header-menu-mobile">
            {items.filter(item => item.key !== "account").map((item) => (
              <Link
                key={item.key}
                to={item.to}
                search={item.search}
                aria-current={navActive(item.key, pathname) ? "page" : undefined}
                className={`squircle site-header-menu-item${navActive(item.key, pathname) ? " is-active" : ""}`}
                onClick={() => setOpen(false)}
              >
                {t(item.label)}
              </Link>
            ))}
            <div className="site-header-menu-line" />
          </div>
          <Link
            to="/account"
            search={{ tab: "preferences" }}
            aria-current={pathname === "/account" ? "page" : undefined}
            className={`squircle site-header-menu-item${pathname === "/account" ? " is-active" : ""}`}
            onClick={() => setOpen(false)}
          >
            {t("个人资料设置")}
          </Link>
          <div className="site-header-menu-line" />
          <button
            type="button"
            className="squircle site-header-menu-item w-full text-left"
            onClick={async () => {
              await logoutFn()
              await router.navigate({ to: "/login" })
            }}
          >
            {t("nav.logout")}
          </button>
        </div>
      ) : null}
    </div>
  )
}
