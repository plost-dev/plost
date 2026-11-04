import { UIIcon } from "./UIIcon"
import { useT } from "~/lib/i18n"
import { useToast } from "~/lib/toast"

/** Draws the message from showToast() just below the top bar; it never takes clicks. */
export function Toaster() {
  const { t } = useT()
  const toast = useToast()
  return <div className="toast-region" role="status" aria-live="polite">
    {toast ? <div key={toast.id} className={`squircle toast is-${toast.tone}`} style={{ animationDuration: `${toast.duration}ms` }}>
      <span className="toast-icon" aria-hidden="true">
        {/* A drawn tick with round ends sits truer in the small circle than the icon font's check. */}
        {toast.tone === "error" ? <UIIcon name="warn" /> : <svg viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round"><path d="M5.5 10.4l3 3 6-6.4" /></svg>}
      </span>
      <span className="toast-text">{t(toast.text)}</span>
    </div> : null}
  </div>
}
