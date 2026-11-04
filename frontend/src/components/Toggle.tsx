import type { ReactNode } from "react"

/** An on/off setting shown as a switch. The text is the label, so clicking it flips the switch too; with hideLabel
 * only screen readers get it (and the title shows it on hover). */
export function Toggle({ checked, onChange, children, disabled, title, hideLabel, labelFirst }: {
  checked: boolean
  onChange: (next: boolean) => void
  children: ReactNode
  disabled?: boolean
  title?: string
  hideLabel?: boolean
  /** The text before the switch, for a switch at the right end of a heading. */
  labelFirst?: boolean
}) {
  return <label className={labelFirst ? "toggle is-label-first" : "toggle"} title={title}>
    <input className="sr-only" type="checkbox" role="switch" checked={checked} disabled={disabled} onChange={event => onChange(event.target.checked)} />
    <span className="squircle toggle-track" aria-hidden="true"><span className="squircle toggle-thumb" /></span>
    <span className={hideLabel ? "sr-only" : "toggle-text"}>{children}</span>
  </label>
}
