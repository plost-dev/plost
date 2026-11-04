import { ActionButton } from "~/components/ActionButton"
import { BrandMark } from "~/components/BrandMark"
import { SiteFooter } from "~/components/Shell"
import { instanceSetupFn, meFn, setupAdminFn } from "~/lib/actions"
import { useT } from "~/lib/i18n"
import { createFileRoute, Link, redirect, useRouter } from "@tanstack/react-router"
import { useState } from "react"
import { PasswordInput } from "~/components/PasswordInput"

export const Route = createFileRoute("/setup")({
  loader: async () => {
    if (await meFn()) throw redirect({to: "/dashboard", replace: true})
    if (!(await instanceSetupFn()).needsSetup) throw redirect({to: "/login", replace: true})
    return {}
  },
  component: SetupPage,
})

function SetupPage() {
  const { t } = useT()
  const router = useRouter()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)

  return (
    <div className="auth-page">
      <main className="auth-main">
        <div className="auth-panel">
          <div className="auth-brand"><Link to="/" aria-label="Plost"><BrandMark /></Link></div>
          <header className="flex flex-col gap-y-2 text-center">
            <h1 className="auth-heading">{t("创建管理员")}</h1>
            <p className="text-pretty text-sm leading-6 text-gray-500">{t("创建此实例的管理员账号。完成后将关闭公开注册。")}</p>
          </header>
          <form
            data-radius="large" className="squircle ui-surface auth-card flex w-full flex-col gap-y-5 p-6"
            onSubmit={async event => {
              event.preventDefault()
              setPending(true)
              setError("")
              try {
                await setupAdminFn({data: {name, email, password}})
                await router.invalidate()
                await router.navigate({to: "/dashboard", replace: true})
              } catch (failure) {
                setError(failure instanceof Error ? failure.message : t("创建管理员失败，请重试。"))
              } finally {
                setPending(false)
              }
            }}
          >
            <label className="flex flex-col gap-y-2 text-sm font-semibold text-gray-800">
              {t("名字")}<input className="squircle input" name="name" autoComplete="name" value={name} onChange={event => setName(event.target.value)} maxLength={255} required />
            </label>
            <label className="flex flex-col gap-y-2 text-sm font-semibold text-gray-800">
              {t("邮箱")}<input className="squircle input" name="email" type="email" autoComplete="email" value={email} onChange={event => setEmail(event.target.value)} maxLength={255} required />
            </label>
            <label className="flex flex-col gap-y-2 text-sm font-semibold text-gray-800">
              {t("密码")}<PasswordInput className="squircle input" name="password" autoComplete="new-password" value={password} onChange={event => setPassword(event.target.value)} minLength={8} maxLength={72} required />
            </label>
            {error ? <p role="alert" className="text-sm text-red-500">{t(error)}</p> : null}
            <ActionButton className="btn btn-primary w-full"  disabled={pending} type="submit">
              {pending ? t("创建中…") : t("创建管理员")}
            </ActionButton>
          </form>
        </div>
      </main>
      <SiteFooter />
    </div>
  )
}
