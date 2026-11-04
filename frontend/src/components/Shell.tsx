import { UserAvatar } from "./UserAvatar"
import { Link, useRouter, useRouterState, type AnyRouter } from "@tanstack/react-router"
import { useEffect, useRef, useState, useSyncExternalStore } from "react"
import { BrandMark } from "./BrandMark"
import { GithubGlyph, LanguageGlyph, MoonGlyph, SunGlyph } from "./FooterGlyphs"
import { logoutFn, trafficPulseFn, updateProfileFn } from "~/lib/actions"
import { clearSettingsCache } from "~/lib/settings-cache"
import { numberShort } from "~/lib/format"
import { LOCALES, useT, type Locale } from "~/lib/i18n"
import { CountryFlag } from "./icons"
import { UIIcon } from "./UIIcon"
import { createThemeUpdate } from "~/lib/theme-update"
import { showToast } from "~/lib/toast"

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
          <FooterLanguage user={user} />
          {user ? <FooterTheme user={user} /> : null}
          <a href="https://github.com/plost-dev" target="_blank" rel="noopener noreferrer" className="squircle site-footer-tool site-footer-github" aria-label="GitHub" title="GitHub">
            <GithubGlyph />
          </a>
          <a href="https://plo.st" target="_blank" rel="noopener noreferrer" className="squircle site-footer-tool site-footer-brand">
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
  headerSearch,
}: {
  user?: ShellUser
  children: React.ReactNode
  layout?: "management" | "analytics"
  /** Page-owned search field shown in the header, left of the account menu. */
  headerSearch?: React.ReactNode
}) {
  // The URL changes before the next loader completes; keep the visible page's
  // layout until its component is replaced instead of styling it as the target.
  return (
    <div className={`flex min-h-full flex-col${layout === "management" ? " management-shell" : ""}`}>
      <SiteHeader user={user} search={headerSearch} />
      {/* The space under the last card equals the space between the header and the first one. */}
      <main className={`container-ls flex-1 ${layout === "management" ? "pb-8" : "pb-4"}`}>{children}</main>
      <SiteFooter user={user} />
    </div>
  )
}

function headerNav(t: (key: string) => string): HeaderNavItem[] {
  const items: HeaderNavItem[] = [
    { key: "sites", to: "/dashboard", label: t("nav.sites") },
    { key: "account", to: "/settings", label: t("settings.title") },
  ]
  return items
}

function navActive(key: string, pathname: string) {
  if (key === "sites") return pathname === "/dashboard" || pathname.startsWith("/sites/") || pathname === "/sites" || pathname.startsWith("/share/")
  if (key === "account") return pathname === "/settings" || pathname.startsWith("/settings/")
  if (key === "api") return pathname === "/docs/api"
  return false
}

type HeaderNavItem = {
  key: string
  to: "/dashboard" | "/settings" | "/docs/api"
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
  return <button type="button" className={`squircle site-footer-tool theme-toggle${dark ? " is-dark" : ""}`} disabled={disabled}
    aria-label={t(dark ? "切换到浅色模式" : "切换到深色模式")}
    title={t(dark ? "切换到浅色模式" : "切换到深色模式")}
    onClick={() => onChange(dark ? "light" : "dark")}>
    <SunGlyph className="theme-toggle-sun"/>
    <MoonGlyph className="theme-toggle-moon"/>
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

// A failed appearance save is reported once, even if the page is remounted while the failure is still the current state.
const reportedFailures = new WeakSet<object>()

function FooterTheme({ user }: { user: NonNullable<ShellUser> }) {
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
  useEffect(() => {
    if (change?.failed && !reportedFailures.has(change)) {
      reportedFailures.add(change)
      showToast("外观保存失败，请重试", "error")
    }
  }, [change])
  return <ThemeSwitch theme={theme} disabled={change?.pending} onChange={next => { void update.change(user.email, theme, next) }} />
}

/** Opens the language menu. A signed-in person keeps the choice on the account; otherwise this browser does. */
function FooterLanguage({ user }: { user?: ShellUser }) {
  const { locale, setLocale, t } = useT()
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const boxRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (!open) return
    const onClick = (event: MouseEvent) => { if (!boxRef.current?.contains(event.target as Node)) setOpen(false) }
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") { setOpen(false); triggerRef.current?.focus() } }
    window.addEventListener("mousedown", onClick)
    window.addEventListener("keydown", onKey)
    boxRef.current?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus()
    return () => { window.removeEventListener("mousedown", onClick); window.removeEventListener("keydown", onKey) }
  }, [open])
  const choose = async (next: Locale) => {
    setOpen(false)
    if (next === locale) return
    setPending(true)
    try {
      if (user) await updateProfileFn({ data: { locale: next } })
      else localStorage.setItem("plost:locale", next)
      await setLocale(next)
      if (user) await router.invalidate()
    } catch {
      showToast("保存失败", "error")
    } finally {
      setPending(false)
    }
  }
  const label = t("切换语言")
  return <div className="site-footer-language" ref={boxRef}>
    <button type="button" ref={triggerRef} className="squircle site-footer-tool" disabled={pending} aria-label={label} title={label}
      aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(value => !value)}>
      <LanguageGlyph />
    </button>
    {open ? <div className="squircle language-menu" role="menu" aria-label={label}>
      {LOCALES.map(item => <button key={item.code} type="button" role="menuitemradio" aria-checked={item.code === locale}
        className={`squircle language-menu-item${item.code === locale ? " is-active" : ""}`} onClick={() => void choose(item.code)}>
        <CountryFlag code={item.flag} shape="rect" className="language-menu-flag" />
        <span className="language-menu-name" lang={item.code}>{item.name}</span>
        {item.code === locale ? <UIIcon name="check" className="language-menu-check" /> : null}
      </button>)}
    </div> : null}
  </div>
}

function SiteHeader({ user, search }: { user?: ShellUser, search?: React.ReactNode }) {
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
                {search ? <div className="site-header-search">{search}</div> : null}
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
        className="squircle site-header-account"
        ref={triggerRef}
        aria-label={t("账户菜单")}
        aria-expanded={open}
        aria-haspopup="menu"
        onClick={() => setOpen((v) => !v)}
      >
        <UserAvatar avatar={user.avatar} gravatarUrl={user.gravatarUrl} name={user.name || user.email} className="squircle site-header-avatar" />
      </button>
      {open ? (
        <div className="squircle site-header-menu" role="menu">
          {/* The menu bar is hidden on narrow screens, so its pages move in here; settings is the profile link below. */}
          <div className="site-header-menu-mobile">
            {items.filter(item => item.key !== "account").map((item) => (
              <Link
                key={item.key}
                to={item.to}
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
            to="/settings"
            aria-current={pathname === "/settings" ? "page" : undefined}
            className={`squircle site-header-menu-item${pathname === "/settings" ? " is-active" : ""}`}
            onClick={() => setOpen(false)}
          >
            {t("个人资料设置")}
          </Link>
          <div className="site-header-menu-line" />
          <button
            type="button"
            className="squircle site-header-menu-item w-full text-start"
            onClick={async () => {
              await logoutFn()
              clearSettingsCache()
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
