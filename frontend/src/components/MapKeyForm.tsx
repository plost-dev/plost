import { useState } from "react"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { ActionButton } from "./ActionButton"
import { Field, FormActions, SettingsForm } from "./SettingsForm"

/**
 * A map key's field. The saved key shows only by its start; a new one is written into the service's env file and
 * used at once. Without an env file (-env-file) the field only says how to set the key.
 */
export function MapKeyForm({label, name, example, masked, editable, save}: {
  label: string; name: string; example: string; masked: string; editable: boolean; save: (key: string) => Promise<unknown>
}) {
  const {t} = useT()
  const [key, setKey] = useState("")
  return <SettingsForm onSubmit={async () => {
    await save(key.trim())
    setKey("")
    showToast("已写入 env 文件，立即生效。")
  }}>
    <Field label={label} hint={editable ? undefined : t("启动 Plost 时没有指定 env 文件（-env-file），请直接在 env 文件里修改 {0}。", {0: name})}>
      <input className="squircle input" aria-label={label} autoComplete="off" spellCheck={false} required disabled={!editable}
        value={key} placeholder={masked || example} onChange={event => setKey(event.target.value)}/>
    </Field>
    {editable ? <FormActions><ActionButton type="submit" className="btn btn-primary">{t("保存")}</ActionButton></FormActions> : null}
  </SettingsForm>
}
