import { ActionButton } from "~/components/ActionButton"
import { redirect, Link, createFileRoute, useRouter } from "@tanstack/react-router"
import { useState } from "react"
import { BrandMark } from "~/components/BrandMark"
import { SiteFooter } from "~/components/Shell"
import { instanceSetupFn, loginFn, meFn } from "~/lib/actions"
import { passkeyLoginFn, verifyLogin2faFn } from "~/lib/auth-actions"
import { useT } from "~/lib/i18n"
import { PasswordInput } from "~/components/PasswordInput"
import { UIIcon } from "~/components/UIIcon"

export const Route = createFileRoute("/login")({
  loader: async () => {
    if (await meFn()) throw redirect({ to: "/dashboard" })
    const setup = await instanceSetupFn()
    if (setup.needsSetup) throw redirect({ to: "/setup" })
    return { passkeyEnabled: setup.passkey }
  },
  component: LoginPage,
})

function LoginPage() {
  const router = useRouter()
  const { t } = useT()
  const { passkeyEnabled } = Route.useLoaderData()
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [code, setCode] = useState("")
  const [needs2fa, setNeeds2fa] = useState(false)
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)


  const passkeyLogin = async () => {
    setError("")
    setPending(true)
    try {
      await passkeyLoginFn()
      await router.invalidate()
      await router.navigate({ to: "/dashboard" })
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
          <div data-radius="large" className="squircle ui-surface login-card">
            <header className="login-band">
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
                    await router.invalidate()
                    await router.navigate({ to: "/dashboard" })
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
                    await router.invalidate()
                    await router.navigate({ to: "/dashboard" })
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
        </section>
      </main>
      <SiteFooter />
    </div>
  )
}
