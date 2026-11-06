// The animation behind the logo on the sign-in page. The choice made on this device is kept in local
// storage; once signed in it is also saved to the account, which the sign-in page reads on other devices.
export const LOGIN_EFFECTS = [
  { value: "aurora", label: "极光" },
  { value: "wave", label: "波浪" },
  { value: "dots", label: "星点上浮" },
  { value: "bars", label: "数据柱" },
  { value: "line", label: "折线绘制" },
  { value: "shift", label: "渐变流动" },
] as const

export type LoginEffect = typeof LOGIN_EFFECTS[number]["value"]

export const DEFAULT_LOGIN_EFFECT: LoginEffect = "aurora"
const STORAGE_KEY = "plost:login-effect"

export function isLoginEffect(value: unknown): value is LoginEffect {
  return typeof value === "string" && LOGIN_EFFECTS.some(effect => effect.value === value)
}

export function readLocalLoginEffect(): LoginEffect | null {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    return isLoginEffect(saved) ? saved : null
  } catch {
    return null
  }
}

export function saveLocalLoginEffect(value: LoginEffect) {
  try { localStorage.setItem(STORAGE_KEY, value) } catch { /* private mode: the choice lasts for this page only */ }
}

/** This device's choice first, then the account's, then the default. */
export function resolveLoginEffect(account?: string | null): LoginEffect {
  return readLocalLoginEffect() ?? (isLoginEffect(account) ? account : DEFAULT_LOGIN_EFFECT)
}
