import { ActionButton } from "~/components/ActionButton"
import { redirect, Link, createFileRoute, useRouter } from "@tanstack/react-router"
import { useEffect, useRef, useState } from "react"
import { BrandMark } from "~/components/BrandMark"
import { SiteFooter } from "~/components/Shell"
import { instanceSetupFn, loginFn, meFn, updateProfileFn } from "~/lib/actions"
import { passkeyLoginFn, verifyLogin2faFn } from "~/lib/auth-actions"
import { useT } from "~/lib/i18n"
import { PasswordInput } from "~/components/PasswordInput"
import { UIIcon } from "~/components/UIIcon"
import { LoginBandEffect } from "~/components/LoginBandEffect"
import { LOGIN_EFFECTS, readLocalLoginEffect, resolveLoginEffect, saveLocalLoginEffect, type LoginEffect } from "~/lib/login-effect"

export const Route = createFileRoute("/login")({
  loader: async () => {
    if (await meFn()) throw redirect({ to: "/dashboard" })
    const setup = await instanceSetupFn()
    if (setup.needsSetup) throw redirect({ to: "/setup" })
    return { passkeyEnabled: setup.passkey, loginEffect: setup.loginEffect }
  },
  component: LoginPage,
})

function LoginPage() {
  const router = useRouter()
  const { t } = useT()
  const { passkeyEnabled, loginEffect } = Route.useLoaderData()
  const [effect, setEffect] = useState<LoginEffect>(() => resolveLoginEffect(loginEffect))
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [code, setCode] = useState("")
  const [needs2fa, setNeeds2fa] = useState(false)
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)


  // A choice made on this page before signing in becomes the account's choice once signed in.
  const enter = async () => {
    const chosen = readLocalLoginEffect()
    if (chosen && chosen !== loginEffect) await updateProfileFn({ data: { loginEffect: chosen } }).catch(() => undefined)
    await router.invalidate()
    await router.navigate({ to: "/dashboard" })
  }

  const passkeyLogin = async () => {
    setError("")
    setPending(true)
    try {
      await passkeyLoginFn()
      await enter()
    } catch (err) {
      setError(err instanceof Error ? err.message : t("Passkey 登录失败"))
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="auth-page">
      <main className="auth-main">
        <section className="auth-panel">
          <div className="login-shell">
          <LoginEffectMenu effect={effect} onChange={(next) => { setEffect(next); saveLocalLoginEffect(next) }} />
          <div data-radius="large" className="squircle ui-surface login-card">
            <header className={`login-band login-band--${effect}`}>
              <LoginBandEffect effect={effect} />
              <Link to="/" className="login-band-logo"><BrandMark animated /></Link>
              <h1>{needs2fa ? t("login.2fa_title") : t("login.welcome")}</h1>
              <p>{needs2fa ? t("login.2fa_sub") : t("login.subtitle")}</p>
            </header>
            {needs2fa ? (
              <form
                className="login-body"
                onSubmit={async (e) => {
                  e.preventDefault()
                  setPending(true)
                  setError("")
                  try {
                    await verifyLogin2faFn({ data: { code } })
                    await enter()
                  } catch (err) {
                    setError(err instanceof Error ? err.message : t("验证失败"))
                  } finally {
                    setPending(false)
                  }
                }}
              >
                <label className="login-field">
                  <span className="login-label">{t("security.verify_2fa")}</span>
                  <span className="login-input">
                    <UIIcon name="shield-check" className="login-input-icon" />
                    <input className="squircle input" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} required autoFocus />
                  </span>
                </label>
                {error ? <p role="alert" className="text-sm text-red-500">{t(error || "")}</p> : null}
                <ActionButton className="btn btn-primary w-full" loading={pending} type="submit">
                  {pending ? t("login.verifying") : t("login.2fa_submit")}
                </ActionButton>
                <ActionButton type="button" className="btn btn-ghost w-full" onClick={() => { setNeeds2fa(false); setCode("") }}>{t("login.back")}</ActionButton>
              </form>
            ) : (
              <form
                className="login-body"
                onSubmit={async (e) => {
                  e.preventDefault()
                  setPending(true)
                  setError("")
                  try {
                    const result = await loginFn({ data: { email, password } })
                    if ("needs2fa" in result && result.needs2fa) {
                      setNeeds2fa(true)
                      return
                    }
                    await enter()
                  } catch (err) {
                    setError(err instanceof Error ? err.message : t("登录失败"))
                  } finally {
                    setPending(false)
                  }
                }}
              >
                <label className="login-field">
                  <span className="login-label">{t("login.email")}</span>
                  <span className="login-input">
                    <UIIcon name="mail" className="login-input-icon" />
                    <input className="squircle input" type="email" autoComplete="email" placeholder="you@example.com" value={email} onChange={(e) => setEmail(e.target.value)} required />
                  </span>
                </label>
                <label className="login-field">
                  <span className="login-label">{t("login.password")}</span>
                  <span className="login-input">
                    <UIIcon name="lock" className="login-input-icon" />
                    <PasswordInput className="squircle input" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
                  </span>
                </label>
                {error ? <p role="alert" className="text-sm text-red-500">{t(error || "")}</p> : null}
                <ActionButton className="btn btn-primary w-full" loading={pending} type="submit">
                  {pending ? t("login.pending") : t("login.submit")}
                </ActionButton>
                {passkeyEnabled ? <>
                  <p className="login-or">{t("login.or")}</p>
                  <ActionButton type="button" className="btn btn-secondary login-passkey w-full" disabled={pending} onClick={passkeyLogin}>
                    <UIIcon name="key" className="login-passkey-icon" />{t("login.passkey")}
                  </ActionButton>
                </> : null}
              </form>
            )}
          </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  )
}

function LoginEffectMenu({ effect, onChange }: { effect: LoginEffect, onChange: (next: LoginEffect) => void }) {
  const { t } = useT()
  const [open, setOpen] = useState(false)
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
  const label = t("login.effect")
  return <div className="login-effect" ref={boxRef}>
    <button type="button" ref={triggerRef} className="squircle login-effect-trigger" aria-label={label} title={label}
      aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(value => !value)}>
      <UIIcon name="shirt" />
    </button>
    {open ? <div className="squircle login-effect-menu" role="menu" aria-label={label}>
      {LOGIN_EFFECTS.map(item => <button key={item.value} type="button" role="menuitemradio" aria-checked={item.value === effect}
        className={`squircle login-effect-item${item.value === effect ? " is-active" : ""}`}
        onClick={() => { onChange(item.value); setOpen(false) }}>
        <span className="login-effect-name">{t(item.label)}</span>
        {item.value === effect ? <UIIcon name="check" className="login-effect-check" /> : null}
      </button>)}
    </div> : null}
  </div>
}
