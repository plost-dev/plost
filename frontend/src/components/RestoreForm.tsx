import { useState } from "react"
import { ActionButton } from "./ActionButton"
import { Field, SettingsForm } from "./SettingsForm"
import { useT } from "~/lib/i18n"
import { rpc } from "~/lib/rpc"
import { PasswordInput } from "~/components/PasswordInput"

/** Verify, then restore. The backup is either an uploaded file or a listed one named by `source` ("local:<name>" / "cloud:<name>"). */
export function RestoreForm({ source, label, onCancel }: { source?: string, label?: string, onCancel: () => void }) {
  const { t } = useT()
  const [current, setCurrent] = useState("")
  const [password, setPassword] = useState("")
  const [file, setFile] = useState<File | null>(null)
  const [verified, setVerified] = useState(false)
  const [confirmation, setConfirmation] = useState("")
  const [notice, setNotice] = useState("")
  const stale = () => { setVerified(false); setNotice("") }

  return <SettingsForm className="squircle full-backup-confirm" onSubmit={async () => {
    if (!source && !file) throw Error(t("请选择备份文件"))
    if (verified && confirmation !== "RESTORE") throw Error(t("输入 RESTORE 确认恢复"))
    const data = new FormData()
    if (source) data.set("source", source)
    else data.set("file", file!)
    data.set("currentPassword", current); data.set("password", password); data.set("confirmation", confirmation)
    await rpc("/api/backup/" + (verified ? "restore" : "validate"), "POST", data)
    if (verified) location.assign("/login")
    else { setVerified(true); setNotice("校验通过，可以恢复此备份。") }
  }}>
    <h3>{t("校验与恢复")}</h3>
    {notice ? <p role="status" className="squircle settings-feedback">{t(notice)}</p> : null}
    {source
      ? <>
        <p className="settings-hint">{t("将使用列表中的这份备份，仍需输入创建它时的备份密码。")}</p>
        <p className="full-backup-filename">{label}</p>
      </>
      : <>
        <Field label={t("上传备份文件")}><input className="squircle input" type="file" accept=".plostbk" required onChange={e => { setFile(e.target.files?.[0] || null); stale() }} /></Field>
      </>}
    <Field label={t("当前管理员密码")}><PasswordInput className="squircle input" required autoComplete="current-password" value={current} onChange={e => { setCurrent(e.target.value); stale() }} /></Field>
    <Field label={t("备份密码")}><PasswordInput className="squircle input" required minLength={12} autoComplete="off" value={password} onChange={e => { setPassword(e.target.value); stale() }} /></Field>
    {verified ? <Field label={t("输入 RESTORE 确认恢复")}><input className="squircle input" required pattern="RESTORE" autoComplete="off" spellCheck={false} value={confirmation} onChange={e => setConfirmation(e.target.value)} /></Field> : null}
    <div className="backup-run-actions">
      <ActionButton className={verified ? "btn btn-danger" : ""} type="submit">{t(verified ? "恢复此完整备份" : "校验备份文件")}</ActionButton>
      <ActionButton onClick={onCancel}>{t("取消")}</ActionButton>
    </div>
  </SettingsForm>
}
