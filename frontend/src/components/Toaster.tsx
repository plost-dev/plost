import { UIIcon } from "./UIIcon"
import { useT } from "~/lib/i18n"
import { useToast } from "~/lib/toast"

/** Draws the message from showToast() just below the top bar; it never takes clicks. */
export function Toaster() {
  const { t } = useT()
  const toast = useToast()
  return <div className="toast-region" role="status" aria-live="polite">
    {toast ? <div key={toast.id} className={`squircle toast is-${toast.tone}`} style={{ animationDuration: `${toast.duration}ms` }}>
      <span className="toast-icon" aria-hidden="true"><UIIcon name={toast.tone === "error" ? "warn" : "check"} /></span>
      <span className="toast-text">{t(toast.text)}</span>
    </div> : null}
  </div>
}
