import { useState, type InputHTMLAttributes } from "react"
import { UIIcon } from "./UIIcon"
import { useT } from "~/lib/i18n"

/** A password field with an eye at its end: press it to see what was typed, press it again to hide it. */
export function PasswordInput({ className = "", ...props }: Omit<InputHTMLAttributes<HTMLInputElement>, "type">) {
  const { t } = useT()
  const [shown, setShown] = useState(false)
  const label = t(shown ? "隐藏密码" : "显示密码")
  return <span className="password-field">
    <input {...props} className={className} type={shown ? "text" : "password"} />
    <button type="button" className="password-toggle" aria-label={label} title={label} aria-pressed={shown} onClick={() => setShown(value => !value)}>
      <UIIcon name={shown ? "eye-off" : "eye"} />
    </button>
  </span>
}
