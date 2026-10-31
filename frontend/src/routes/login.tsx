import { ActionButton } from "~/components/ActionButton"
import { redirect, Link, createFileRoute, useRouter } from "@tanstack/react-router"
import { useState } from "react"
import { BrandMark } from "~/components/BrandMark"
import { SiteFooter } from "~/components/Shell"
import { instanceSetupFn, loginFn, meFn } from "~/lib/actions"
import { passkeyLoginFn, verifyLogin2faFn } from "~/lib/auth-actions"
import { useT } from "~/lib/i18n"
import { PasswordInput } from "~/components/PasswordInput"

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


  return (
    <div className="auth-page">
      <main className="auth-main">
        <section className="auth-panel">
          <div className="auth-brand"><Link to="/"><BrandMark /></Link></div>
          {needs2fa ? <header className="text-center">
            <h1 className="auth-heading">{t("login.2fa_title")}</h1>
            <p className="mt-2 text-pretty text-sm leading-6 text-gray-500">{t("login.2fa_sub")}</p>
          </header> : <h1 className="sr-only">{t("login.title")}</h1>}
          {needs2fa ? (
            <form
              data-radius="large" className="squircle ui-surface auth-card flex w-full flex-col gap-y-5 p-6"
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
              <label className="flex flex-col gap-y-2 text-sm font-semibold text-gray-800">
                {t("security.verify_2fa")}
                <input className="squircle input" value={code} onChange={(e) => setCode(e.target.value)} required autoFocus />
              </label>
              {error ? <p role="alert" className="text-sm text-red-500">{t(error || "")}</p> : null}
              <ActionButton className="btn btn-primary w-full"  loading={pending} type="submit">
                {pending ? t("login.verifying") : t("login.2fa_submit")}
              </ActionButton>
              <ActionButton type="button" className="btn btn-ghost w-full" onClick={() => { setNeeds2fa(false); setCode("") }}>{t("login.back")}</ActionButton>
            </form>
          ) : (
            <form
              data-radius="large" className="squircle ui-surface auth-card flex w-full flex-col gap-y-5 p-6"
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
              <label className="flex flex-col gap-y-2 text-sm font-semibold text-gray-800">
                {t("login.email")}
                <input className="squircle input" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
              </label>
              <label className="flex flex-col gap-y-2 text-sm font-semibold text-gray-800">
                {t("login.password")}
                <PasswordInput className="squircle input" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
              </label>
              {error ? <p role="alert" className="text-sm text-red-500">{t(error || "")}</p> : null}
              <ActionButton className="btn btn-primary w-full"  loading={pending} type="submit">
                {pending ? t("login.pending") : t("login.submit")}
              </ActionButton>
              {passkeyEnabled ? <ActionButton
                type="button"
                className="btn btn-secondary w-full"
                disabled={pending}
                onClick={async () => {
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
                }}
              >
                {t("login.passkey")}
              </ActionButton> : null}
            </form>
          )}
        </section>
      </main>
      <SiteFooter />
    </div>
  )
}
