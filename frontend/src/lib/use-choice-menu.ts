import { useEffect, useRef, useState } from "react"

/** A trigger with a menu of choices: an outside press or Escape closes it (Escape returns focus to the trigger),
 *  and opening it focuses the checked item. Put boxRef on the element that holds both. */
export function useChoiceMenu() {
  const [open, setOpen] = useState(false)
  const boxRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (!open) return
    const onClick = (event: MouseEvent) => { if (!boxRef.current?.contains(event.target as Node)) setOpen(false) }
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") { setOpen(false); triggerRef.current?.focus() } }
    window.addEventListener("mousedown", onClick)
    window.addEventListener("keydown", onKey)
    boxRef.current?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus()
    return () => { window.removeEventListener("mousedown", onClick); window.removeEventListener("keydown", onKey) }
  }, [open])
  return { open, setOpen, boxRef, triggerRef }
}
