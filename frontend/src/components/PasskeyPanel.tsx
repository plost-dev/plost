import { useCallback, useEffect, useState } from "react"
import { ActionButton } from "./ActionButton"
import { ConfirmModal, Field } from "./SettingsForm"
import { Tile } from "./ui"
import { UIIcon } from "./UIIcon"
import { formatTimestamp } from "~/lib/format"
import { passkeyAddFn, passkeyListFn, passkeyRemoveFn, type PasskeyItem } from "~/lib/auth-actions"
import { useT } from "~/lib/i18n"

/** Passkeys are listed here; adding or removing one needs the account password. */
export function PasskeyPanel() {
  const { t, locale } = useT()
  const [items, setItems] = useState<PasskeyItem[] | null>(null)
  const [error, setError] = useState("")
  const [adding, setAdding] = useState(false)
  const [removing, setRemoving] = useState<PasskeyItem | null>(null)
  const [password, setPassword] = useState("")
  const [name, setName] = useState("")
  const [pending, setPending] = useState(false)

  const load = useCallback(() => passkeyListFn().then(setItems, err => setError(err instanceof Error ? err.message : "加载失败，请刷新页面重试")), [])
  useEffect(() => { void load() }, [load])
  const close = () => { setAdding(false); setRemoving(null); setPassword(""); setName("") }

  return <>
    <Tile icon="key" title={t("security.passkey")} subtitle={t("security.passkey_sub")}>
      {items && items.length === 0 ? <div className="settings-empty"><UIIcon name="key" /><span>{t("security.no_passkey")}</span></div> : null}
      {items && items.length ? <ul className="divide-y divide-gray-100 text-sm">
        {items.map(item => <li key={item.id} className="settings-list-row flex min-w-0 flex-wrap items-center justify-between gap-3 py-2">
          <span className="min-w-0 flex-1">
            <span className="block [overflow-wrap:anywhere]">{item.name}</span>
            <span className="block text-xs text-gray-500">{t("创建于 {0}", { 0: formatTimestamp(item.createdAt * 1000, locale) })} · {item.lastUsedAt ? t("最近使用 {0}", { 0: formatTimestamp(item.lastUsedAt * 1000, locale) }) : t("从未使用")}</span>
          </span>
          <ActionButton className="shrink-0 text-red-600 hover:text-red-700" onClick={() => { setError(""); setRemoving(item) }}>{t("security.remove")}</ActionButton>
        </li>)}
      </ul> : null}
      <div className="mt-4">
        <ActionButton className="btn btn-primary" disabled={!items} onClick={() => { setError(""); setAdding(true) }}>{t("security.add_passkey")}</ActionButton>
      </div>
      {error ? <p role="alert" className="mt-3 text-sm text-red-600">{t(error)}</p> : null}
    </Tile>
    <ConfirmModal
      open={adding}
      title={t("security.add_passkey")}
      confirmLabel={t("继续")}
      pending={pending}
      onClose={close}
      onSubmit={async () => {
        setPending(true)
        try { await passkeyAddFn({ data: { password, name: name.trim() } }); close(); await load() }
        finally { setPending(false) }
      }}
    >
      <p>{t("确认密码后，浏览器会请你用指纹、面容或安全密钥创建 Passkey。")}</p>
      <Field label={t("Passkey 名称（可选）")}><input className="squircle input" maxLength={60} autoComplete="off" placeholder={t("例如：工作电脑")} value={name} onChange={e => setName(e.target.value)} /></Field>
      <Field label={t("security.confirm_password")}><input className="squircle input" required type="password" autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} /></Field>
    </ConfirmModal>
    <ConfirmModal
      open={!!removing}
      title={t("移除这把 Passkey")}
      confirmLabel={t("security.remove")}
      danger
      pending={pending}
      onClose={close}
      onSubmit={async () => {
        if (!removing) return
        setPending(true)
        try { await passkeyRemoveFn({ data: { id: removing.id, password } }); close(); await load() }
        finally { setPending(false) }
      }}
    >
      <p>{t("移除后，{0} 将不能再用于登录。", { 0: removing?.name || "" })}</p>
      <Field label={t("security.confirm_password")}><input className="squircle input" required type="password" autoComplete="current-password" value={password} onChange={e => setPassword(e.target.value)} /></Field>
    </ConfirmModal>
  </>
}
