import { TrackingSnippet } from "~/components/TrackingSnippet"
import { ActionButton, ActionLink } from "~/components/ActionButton"
import { Surface } from "~/components/Surface"
import { Navigate } from "@tanstack/react-router"
import { Link, createFileRoute } from "@tanstack/react-router"
import { useMemo, useState } from "react"
import { SettingsHeader } from "~/components/SettingsChrome"
import { Shell } from "~/components/Shell"
import { createSiteFn, meFn, siteReadyFn } from "~/lib/actions"
import { useT } from "~/lib/i18n"
import { timezoneOptions } from "~/lib/timezones"
import { cleanDomainInput } from "~/lib/domain-input"

export const Route = createFileRoute("/sites/new")({
  loader: async () => ({ me: await meFn() }),
  component: NewSitePage,
})


function NewSitePage() {
  const { me } = Route.useLoaderData()
  const { t } = useT()
  const [step, setStep] = useState(1)
  const [name, setName] = useState("")
  const [domain, setDomain] = useState("")
  const [timezone, setTimezone] = useState(Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Shanghai")
  const ZONES = useMemo(() => timezoneOptions(timezone), [timezone])
  const [created, setCreated] = useState("")
  const [error, setError] = useState("")
  const [pending, setPending] = useState(false)
  const [ready, setReady] = useState(false)
  const origin = typeof window === "undefined" ? "" : window.location.origin
  const site = created || domain
  const snippet = `<script defer data-domain="${site}" src="${origin}/script.js"></script>`

  if (!me) {
    return <Navigate to="/login" />
  }

  return (
    <Shell user={me}>
      <SettingsHeader
        title={t("site.new_title")}
        back={(
          <ActionLink to="/dashboard" className="btn-secondary settings-back">
            
            {t("settings.back_sites")}
          </ActionLink>
        )}
      />
      <div className="new-site-wizard mx-auto mt-6 min-w-0 w-full max-w-2xl pb-16">
        <div className="wizard-steps" role="list" aria-label={t("site.new_title")}>
          <div role="listitem" aria-current={step === 1 ? "step" : undefined} className={`wizard-step${step === 1 ? " is-active" : step > 1 ? " is-done" : ""}`}>
            <span className="wizard-index shrink-0">1</span>
            <span className="wizard-step-label min-w-0 break-words">{t("site.step_site")}</span>
          </div>
          <div className="wizard-line" />
          <div role="listitem" aria-current={step === 2 ? "step" : undefined} className={`wizard-step${step === 2 ? " is-active" : step > 2 ? " is-done" : ""}`}>
            <span className="wizard-index shrink-0">2</span>
            <span className="wizard-step-label min-w-0 break-words">{t("site.step_install")}</span>
          </div>
          <div className="wizard-line" />
          <div role="listitem" aria-current={step === 3 ? "step" : undefined} className={`wizard-step${step === 3 ? " is-active" : ""}`}>
            <span className="wizard-index shrink-0">3</span>
            <span className="wizard-step-label min-w-0 break-words">{t("site.step_done")}</span>
          </div>
        </div>
        <Surface className="setup-card bg-white shadow-sm">
        {step === 1 ? (
          <form
            className="space-y-6"
            onSubmit={async (e) => {
              e.preventDefault()
              setError("")
              setPending(true)
              try {
                const siteRow = await createSiteFn({ data: { domain: cleanDomainInput(domain), timezone, name } })
                setCreated(siteRow.domain)
                setStep(2)
              } catch (err) {
                setError(err instanceof Error ? err.message : t("无法创建站点"))
              } finally {
                setPending(false)
              }
            }}
          >
            <label className="block text-sm font-medium text-gray-900">
              {t("站点名称")}
              <input className="squircle input mt-2" title={t("在站点列表中显示，留空则默认使用域名。")} placeholder={t("如：我的博客")} value={name} onChange={(e) => setName(e.target.value)} />
            </label>
            <label className="block text-sm font-medium text-gray-900">
              {t("site.domain")}
              <span className="mt-1 block text-xs font-normal text-gray-500">{t("site.domain_hint")}</span>
              <input className="squircle input mt-2" autoFocus autoCapitalize="none" spellCheck={false} placeholder="example.com" value={domain} onChange={(e) => setDomain(e.target.value)}
                // A pasted address shows as its bare host at once; anything typed is tidied on leaving the field.
                onPaste={(e) => { e.preventDefault(); setDomain(cleanDomainInput(e.clipboardData.getData("text"))) }}
                onBlur={() => setDomain(cleanDomainInput(domain))} required />
            </label>
            <label className="block text-sm font-medium text-gray-900">
              {t("site.timezone")}
              <select className="squircle input mt-2" value={timezone} onChange={(e) => setTimezone(e.target.value)}>
                {ZONES.map((zone) => <option key={zone.value} value={zone.value}>{t(zone.label)}</option>)}
              </select>
            </label>
            {error ? <p role="alert" className="min-w-0 break-words text-sm text-red-500">{t(error || "")}</p> : null}
            <div className="wizard-actions flex min-w-0 flex-wrap justify-end gap-2">
              <ActionButton className="btn btn-primary"  disabled={pending} type="submit">{pending ? t("添加中…") : t("site.next")}</ActionButton>
            </div>
          </form>
        ) : null}
        {step === 2 ? (
          <div className="space-y-4">
            <p className="text-sm text-gray-600">{t("site.snippet")}</p>
            <TrackingSnippet code={snippet} />
            {error ? <p role="alert" className="min-w-0 break-words text-sm text-red-500">{t(error || "")}</p> : null}
            <div className="wizard-actions flex min-w-0 flex-wrap justify-end gap-2">
              <ActionButton type="button" className="btn btn-secondary" onClick={() => setStep(3)}>{t("site.skip")}</ActionButton>
              <ActionButton
                type="button"
                className="btn btn-primary"
                
                disabled={pending}
                onClick={async () => {
                  setPending(true)
                  setError("")
                  try {
                    const result = await siteReadyFn({ data: { domain: created } })
                    if (result.ready) {
                      setReady(true)
                      setStep(3)
                    } else {
                      setError(t("site.waiting"))
                    }
                  } catch (err) {
                    setError(err instanceof Error ? err.message : t("site.waiting"))
                  } finally {
                    setPending(false)
                  }
                }}
              >
                {pending ? t("检查中…") : t("site.check")}
              </ActionButton>
            </div>
          </div>
        ) : null}
        {step === 3 ? (
          <div className="space-y-4">
            <p className="text-sm text-gray-600">{ready ? t("site.detected") : t("site.snippet")}</p>
            <div className="wizard-actions flex min-w-0 flex-wrap justify-end gap-2">
              <Link to="/sites/$domain" params={{ domain: created }} className="squircle btn btn-primary">{t("site.done_go")}</Link>
            </div>
          </div>
        ) : null}
        </Surface>
      </div>
    </Shell>
  )
}
