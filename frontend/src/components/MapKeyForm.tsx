import { useEffect, useRef, useState } from "react"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { ActionButton } from "./ActionButton"
import { Field, FormActions, SettingsForm } from "./SettingsForm"

/**
 * A map key's field. The saved key shows only by its start; a new one is written into the service's env file and
 * used at once. Without an env file (-env-file) the field only says how to set the key.
 *
 * Once a key is saved the field is locked and Save stays off: Edit unlocks it (and becomes Cancel), and Save turns on
 * once something is typed. With no key yet the field is open, and Save waits for something to save.
 */
export function MapKeyForm({label, name, example, masked, editable, save}: {
  label: string; name: string; example: string; masked: string; editable: boolean; save: (key: string) => Promise<unknown>
}) {
  const {t} = useT()
  const [key, setKey] = useState("")
  const [editing, setEditing] = useState(false)
  const input = useRef<HTMLInputElement | null>(null)
  const saved = Boolean(masked)
  const locked = !editable || (saved && !editing)
  // Edit puts the cursor in the field right away.
  useEffect(() => { if (editing) input.current?.focus() }, [editing])
  return <SettingsForm onSubmit={async () => {
    await save(key.trim())
    setKey("")
    setEditing(false)
    showToast("已写入 env 文件，立即生效。")
  }}>
    <Field label={label} hint={editable ? undefined : t("启动 Plost 时没有指定 env 文件（-env-file），请直接在 env 文件里修改 {0}。", {0: name})}>
      <input ref={input} className="squircle input" aria-label={label} autoComplete="off" spellCheck={false} required disabled={locked}
        value={key} placeholder={masked || example} onChange={event => setKey(event.target.value)}/>
    </Field>
    {editable ? <FormActions>
      <ActionButton type="submit" className="btn btn-primary" disabled={locked || !key.trim()}>{t("保存")}</ActionButton>
      {saved ? <ActionButton className="btn btn-secondary" onClick={() => { setEditing(!editing); setKey("") }}>{t(editing ? "取消" : "修改")}</ActionButton> : null}
    </FormActions> : null}
  </SettingsForm>
}
