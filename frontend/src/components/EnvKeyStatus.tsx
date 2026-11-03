import { useT } from "~/lib/i18n"
import { Field } from "./SettingsForm"

/**
 * Whether a key is set in the service's env file. Keys are never entered on the page or kept in the database,
 * so they survive a database restore; changing one means editing the env file and restarting Plost.
 */
export function EnvKeyStatus({label, name, configured, wide}: {label: string; name: string; configured: boolean; wide?: boolean}) {
  const {t} = useT()
  return <Field label={label} wide={wide} hint={configured ? undefined : t("在服务的 env 文件里加入 {0}=…，然后重启 Plost。", {0: name})}>
    <p className={`squircle settings-feedback env-key-status${configured ? "" : " is-error"}`}>
      {configured ? t("已在 env 文件中设置 {0}", {0: name}) : t("未设置 {0}", {0: name})}
    </p>
  </Field>
}
