import { getRouteApi, useRouter } from "@tanstack/react-router"
import { useState } from "react"
import { LoginBandEffect } from "./LoginBandEffect"
import { UIIcon } from "./UIIcon"
import { updateProfileFn } from "~/lib/actions"
import { useT } from "~/lib/i18n"
import { LOGIN_EFFECTS, resolveLoginEffect, saveLocalLoginEffect, type LoginEffect } from "~/lib/login-effect"
import { showToast } from "~/lib/toast"

const settingsRoute = getRouteApi("/settings")

/** Saves the sign-in animation to the account and to this device, so the sign-in page shows it right away. */
export function LoginEffectPicker() {
  const { t } = useT()
  const router = useRouter()
  const { data } = settingsRoute.useLoaderData()
  const [selected, setSelected] = useState<LoginEffect>(() => resolveLoginEffect(data.user.loginEffect))
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  async function choose(next: LoginEffect) {
    if (pending || next === selected) return
    const previous = selected
    setSelected(next); setPending(true); setError("")
    try {
      await updateProfileFn({ data: { loginEffect: next } })
      saveLocalLoginEffect(next)
      await router.invalidate({ sync: true })
      showToast("登录页动效已保存")
    } catch (err) {
      setSelected(previous)
      setError(err instanceof Error ? err.message : t("保存失败"))
    } finally { setPending(false) }
  }
  return <fieldset className="map-style-picker login-effect-picker" disabled={pending} aria-busy={pending}>
    <legend>{t("login.effect")}</legend>
    <div className="map-style-options">
      {LOGIN_EFFECTS.map(effect => <label key={effect.value} className={`squircle map-style-option${selected === effect.value ? " is-selected" : ""}`}>
        <input className="sr-only" type="radio" name="login-effect" value={effect.value} checked={selected === effect.value} onChange={() => void choose(effect.value)} />
        <span className={`squircle login-band login-band--${effect.value} login-effect-preview`}><LoginBandEffect effect={effect.value} /></span>
        <span className="map-style-option-caption"><span>{t(effect.label)}</span>{selected === effect.value ? <UIIcon name="check" className="size-3.5" /> : null}</span>
      </label>)}
    </div>
    {error ? <p className="text-sm text-red-600" role="alert">{t(error)}</p> : null}
  </fieldset>
}
