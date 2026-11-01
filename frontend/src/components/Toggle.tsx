import type { ReactNode } from "react"

/** An on/off setting shown as a switch. The text is the label, so clicking it flips the switch too. */
export function Toggle({ checked, onChange, children, disabled, title }: {
  checked: boolean
  onChange: (next: boolean) => void
  children: ReactNode
  disabled?: boolean
  title?: string
}) {
  return <label className="toggle" title={title}>
    <input className="sr-only" type="checkbox" role="switch" checked={checked} disabled={disabled} onChange={event => onChange(event.target.checked)} />
    <span className="squircle toggle-track" aria-hidden="true"><span className="squircle toggle-thumb" /></span>
    <span className="toggle-text">{children}</span>
  </label>
}
