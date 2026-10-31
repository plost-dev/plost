import { ActionButton } from "~/components/ActionButton"
import { useState } from "react"
import { ConfirmModal, Field, FormActions, SettingsForm } from "~/components/SettingsForm"
import { Tile } from "~/components/ui"
import { updateProfileFn } from "~/lib/actions"
import {
  confirm2faFn,
  disable2faFn,
  initiate2faFn,
  regenRecoveryFn,
} from "~/lib/auth-actions"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { PasswordInput } from "~/components/PasswordInput"

export function AccountSecurity({
  user,
  onSaved,
}: {
  user: { email: string, totp_enabled?: boolean }
  onSaved: () => void
}) {
  const { t } = useT()
  const [email, setEmail] = useState("")
  const [emailPassword, setEmailPassword] = useState("")
  const [oldPassword, setOldPassword] = useState("")
  const [password, setPassword] = useState("")
  const [confirm, setConfirm] = useState("")
  const [error, setError] = useState("")
  const [emailError, setEmailError] = useState("")
  const [passwordError, setPasswordError] = useState("")
  const [setup, setSetup] = useState<{ qr: string, secret: string } | null>(null)
  const [code, setCode] = useState("")
  const [codes, setCodes] = useState<string[] | null>(null)
  const [enableOpen, setEnableOpen] = useState(false)
  const [disableOpen, setDisableOpen] = useState(false)
  const [regenOpen, setRegenOpen] = useState(false)
  const [securePassword, setSecurePassword] = useState("")
  const [secureCode, setSecureCode] = useState("")
  const [pending, setPending] = useState(false)

  return (
    <>
      <Tile icon="mail" title={t("security.email")}>
        <SettingsForm
          onSubmit={async (e) => {
            e.preventDefault()
            setEmailError("")
            try {
              await updateProfileFn({ data: { email, oldPassword: emailPassword } })
              setEmail("")
              setEmailPassword("")
              showToast("邮箱已更新")
              onSaved()
            } catch (err) {
              setEmailError(err instanceof Error ? err.message : t("无法更改邮箱"))
            }
          }}
        >
          <Field label={t("security.current_email")}>
            <input className="squircle input bg-gray-50" value={user.email} disabled />
          </Field>
          <Field label={t("security.new_email")}>
            <input className="squircle input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </Field>
          <Field label={t("security.confirm_password")}>
            <PasswordInput className="squircle input" value={emailPassword} onChange={(e) => setEmailPassword(e.target.value)} required />
          </Field>
          {emailError ? <p role="alert" className="mt-3 text-sm text-red-600">{t(emailError)}</p> : null}
          <FormActions>
            <ActionButton className="btn btn-primary" type="submit">{t("security.change_email")}</ActionButton>
          </FormActions>
        </SettingsForm>
      </Tile>
      <Tile icon="lock" title={t("security.password")}>
        <SettingsForm
          onSubmit={async (e) => {
            e.preventDefault()
            setPasswordError("")
            if (password !== confirm) {
              setPasswordError(t("两次输入的新密码不一致"))
              return
            }
            try {
              await updateProfileFn({ data: { oldPassword, password } })
              setOldPassword("")
              setPassword("")
              setConfirm("")
              showToast("密码已更新")
            } catch (err) {
              setPasswordError(err instanceof Error ? err.message : t("无法更改密码"))
            }
          }}
        >
          <Field label={t("security.old_password")}>
            <PasswordInput className="squircle input" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} required />
          </Field>
          <Field label={t("security.new_password")}>
            <PasswordInput className="squircle input" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </Field>
          <Field label={t("security.confirm_new_password")}>
            <PasswordInput className="squircle input" value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
          </Field>
          {passwordError ? <p role="alert" className="mt-3 text-sm text-red-600">{t(passwordError)}</p> : null}
          <FormActions>
            <ActionButton className="btn btn-primary" type="submit">{t("security.change_password")}</ActionButton>
          </FormActions>
        </SettingsForm>
      </Tile>
      <Tile icon="shield" title={t("security.2fa")}>
        {codes ? (
          <div>
            <p className="text-sm font-medium text-gray-900">{t("security.2fa_codes")}</p>
            <p className="mt-1 text-sm text-gray-500">{t("security.2fa_codes_sub")}</p>
            <div className="squircle recovery-codes">
              {codes.map((item) => <div key={item} className="min-w-0 [overflow-wrap:anywhere]">{item}</div>)}
            </div>
            <ActionButton  type="button" className="btn btn-primary mt-4" onClick={() => { setCodes(null); onSaved() }}>{t("security.2fa_done")}</ActionButton>
          </div>
        ) : setup ? (
          <SettingsForm
            onSubmit={async (e) => {
              e.preventDefault()
              setPending(true)
              try {
                const result = await confirm2faFn({ data: { code, password: securePassword } })
                setSetup(null)
                setCode("")
                setCodes(result.codes)
              } catch (err) {
                setError(err instanceof Error ? err.message : t("无法启用"))
              } finally {
                setPending(false)
              }
            }}
          >
            <p className="text-sm text-gray-600">{t("security.2fa_scan")}</p>
            <img src={setup.qr} alt="" className="squircle auth-qr mt-3 size-44 rounded-md border border-gray-200 p-2" />
            <p className="mt-2 min-w-0 text-xs text-gray-500">{t("security.2fa_secret")}：<code className="[overflow-wrap:anywhere]">{setup.secret}</code></p>
            <Field label={t("security.2fa_code")}>
              <input className="squircle input" inputMode="numeric" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} required />
            </Field>
            <FormActions>
              <ActionButton className="btn btn-primary" type="submit" disabled={pending}>{t("security.2fa_confirm")}</ActionButton>
              <ActionButton type="button" className="btn btn-secondary" onClick={() => { setSetup(null); setSecurePassword(""); setError("") }}>{t("取消")}</ActionButton>
            </FormActions>
          </SettingsForm>
        ) : user.totp_enabled ? (
          <div className="flex flex-wrap gap-2">
            <ActionButton type="button" className="btn btn-danger" onClick={() => setDisableOpen(true)}>{t("security.disable_2fa")}</ActionButton>
            <ActionButton  type="button" className="btn btn-secondary" onClick={() => setRegenOpen(true)}>{t("security.generate_new")}</ActionButton>
          </div>
        ) : (
          <ActionButton
            
            type="button"
            className="btn btn-primary"
            onClick={() => {
              setError("")
              setSecurePassword("")
              setEnableOpen(true)
            }}
          >
            {t("security.enable_2fa")}
          </ActionButton>
        )}
        {error ? <p role="alert" className="mt-3 text-sm text-red-600">{t(error || "")}</p> : null}
      </Tile>

      <ConfirmModal
        open={enableOpen}
        title={t("security.enable_2fa")}
        confirmLabel={t("继续")}
        pending={pending}
        onClose={() => { setEnableOpen(false); setSecurePassword("") }}
        onSubmit={async () => {
          setPending(true)
          try {
            setSetup(await initiate2faFn({ data: { password: securePassword } }))
            setEnableOpen(false)
          } finally {
            setPending(false)
          }
        }}
      >
        <Field label={t("security.confirm_password")}>
          <PasswordInput className="squircle input" required autoComplete="current-password" value={securePassword} onChange={event => setSecurePassword(event.target.value)} />
        </Field>
      </ConfirmModal>
      <ConfirmModal
        open={disableOpen}
        title={t("security.disable_2fa")}
        confirmLabel={t("security.disable_2fa")}
        danger
        pending={pending}
        onClose={() => setDisableOpen(false)}
        onSubmit={async () => {
          setPending(true)
          try {
            await disable2faFn({ data: { password: securePassword, code: secureCode } })
            setDisableOpen(false)
            setSecurePassword("")
            setSecureCode("")
            onSaved()
          } finally {
            setPending(false)
          }
        }}
      >
        <p>{t("security.disable_confirm")}</p>
        <Field label={t("security.confirm_password")}><PasswordInput className="squircle input" autoComplete="current-password" value={securePassword} onChange={(e) => setSecurePassword(e.target.value)} required /></Field>
        <Field label={t("security.verify_2fa")}><input className="squircle input" autoComplete="one-time-code" value={secureCode} onChange={(e) => setSecureCode(e.target.value)} required /></Field>
      </ConfirmModal>
      <ConfirmModal
        open={regenOpen}
        title={t("security.generate_new")}
        confirmLabel={t("security.generate_new")}
        pending={pending}
        onClose={() => setRegenOpen(false)}
        onSubmit={async () => {
          setPending(true)
          try {
            const result = await regenRecoveryFn({ data: { password: securePassword, code: secureCode } })
            setRegenOpen(false)
            setSecurePassword("")
            setSecureCode("")
            setCodes(result.codes)
          } finally {
            setPending(false)
          }
        }}
      >
        <p>{t("security.disable_confirm")}</p>
        <Field label={t("security.confirm_password")}><PasswordInput className="squircle input" autoComplete="current-password" value={securePassword} onChange={(e) => setSecurePassword(e.target.value)} required /></Field>
        <Field label={t("security.verify_2fa")}><input className="squircle input" autoComplete="one-time-code" value={secureCode} onChange={(e) => setSecureCode(e.target.value)} required /></Field>
      </ConfirmModal>
    </>
  )
}
