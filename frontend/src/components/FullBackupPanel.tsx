import { useState } from "react"
import { Tile } from "./ui"
import { ActionButton } from "./ActionButton"
import { RestoreForm } from "./RestoreForm"
import { Field, SettingsForm } from "./SettingsForm"
import { useT } from "~/lib/i18n"
import { download } from "~/lib/rpc"
import { PasswordInput } from "~/components/PasswordInput"

export function FullBackupPanel() {
  const { t } = useT()
  const [create, setCreate] = useState(false)
  const [restore, setRestore] = useState(false)
  const [current, setCurrent] = useState("")
  const [password, setPassword] = useState("")
  const [notice, setNotice] = useState("")
  const clear = () => { setPassword(""); setCurrent(""); setNotice("") }

  return <Tile id="full-backup" icon="shield" title={t("完整备份与恢复")} subtitle={t("统一保存账号、站点配置与全部访问数据")}>
    <p className="settings-hint">{t("在这里创建、下载或上传完整加密备份。恢复前自动保存当前数据，还原到新数据库并校验，成功后切换；旧数据库保留。")}</p>
    <p className="settings-hint">{t("备份和恢复期间会短暂暂停采集与后台访问，完成后自动恢复。")}</p>
    <div className="backup-run-actions">
      <ActionButton className="btn btn-primary" onClick={() => { clear(); setCreate(true); setRestore(false) }}>{t("创建完整备份")}</ActionButton>
      <ActionButton onClick={() => { clear(); setRestore(true); setCreate(false) }}>{t("上传备份文件")}</ActionButton>
    </div>
    {notice ? <p role="status" className="squircle settings-feedback">{t(notice)}</p> : null}
    {create ? <SettingsForm className="squircle full-backup-confirm" onSubmit={async () => {
      await download("/api/backup", { currentPassword: current, password })
      setCreate(false); clear(); setNotice("备份已下载，请将备份密码另外妥善保存。")
    }}>
      <Field label={t("当前管理员密码")}><PasswordInput className="squircle input" required autoComplete="current-password" value={current} onChange={e => setCurrent(e.target.value)} /></Field>
      <Field label={t("备份密码")} hint={t("至少 12 个字符。此密码用于恢复备份，不能找回。")}><PasswordInput className="squircle input" required minLength={12} maxLength={256} autoComplete="new-password" value={password} onChange={e => setPassword(e.target.value)} /></Field>
      <div className="backup-run-actions">
        <ActionButton className="btn btn-primary" type="submit">{t("创建并下载")}</ActionButton>
        <ActionButton onClick={() => { clear(); setCreate(false) }}>{t("取消")}</ActionButton>
      </div>
    </SettingsForm> : null}
    {restore ? <RestoreForm onCancel={() => { clear(); setRestore(false) }} /> : null}
    {!create && !restore ? <div className="backup-empty"><p>{t("创建并下载完整备份，或上传已有备份进行校验和恢复。")}</p></div> : null}
  </Tile>
}
