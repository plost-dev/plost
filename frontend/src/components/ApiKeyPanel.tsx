import { useCallback, useEffect, useState } from "react"
import { ActionButton } from "./ActionButton"
import { ConfirmModal, Field, FormActions, SettingsForm } from "./SettingsForm"
import { Tile } from "./ui"
import { UIIcon } from "./UIIcon"
import { siteDomainsFn, type Site } from "~/lib/actions"
import { apiKeysFn, createApiKeyFn, removeApiKeyFn, type ApiKey } from "~/lib/api-key-actions"
import { formatTimestamp } from "~/lib/format"
import { useT } from "~/lib/i18n"

// The full API guide lives on the website, so this page only hands out and revokes keys.
export const API_DOCS_URL = "https://plo.st/self-hosted-docs.html#api"

export function ApiKeyPanel() {
  const { t, locale } = useT()
  const [keys, setKeys] = useState<ApiKey[] | null>(null)
  const [sites, setSites] = useState<Site[]>([])
  const [domain, setDomain] = useState("")
  const [name, setName] = useState("")
  const [created, setCreated] = useState("")
  const [copied, setCopied] = useState(false)
  const [removing, setRemoving] = useState<ApiKey | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")

  const load = useCallback(() => apiKeysFn().then(r => setKeys(r.keys), err => setError(err instanceof Error ? err.message : "加载失败，请刷新页面重试")), [])
  useEffect(() => {
    void load()
    void siteDomainsFn().then(list => { setSites(list); setDomain(d => d || list[0]?.domain || "") }, () => {})
  }, [load])

  return <>
    <Tile icon="key" title="API Key" subtitle="只读，每个 Key 只能读取一个站点的在线人数、访客、访问次数和浏览量。"
      titleAddon={<a className="settings-heading-link" href={API_DOCS_URL} target="_blank" rel="noreferrer">{t("查看 API 文档")}</a>}>
      {created ? <div className="space-y-4">
        <p role="status" className="text-sm font-medium text-gray-900">{t("API Key 已创建，请现在复制保存，关闭后无法再次查看。")}</p>
        <input className="squircle input font-mono" type="text" value={created} readOnly autoComplete="off" spellCheck={false} aria-label="API Key" onFocus={e => e.currentTarget.select()} />
        <FormActions>
          <ActionButton className="btn btn-primary" icon="copy" onClick={async () => {
            try { await navigator.clipboard.writeText(created); setCopied(true) } catch { setError("无法复制，请选中上面的 Key 手动复制。") }
          }}>{t(copied ? "已复制" : "复制 API Key")}</ActionButton>
          <ActionButton className="btn btn-secondary" onClick={() => { setCreated(""); setCopied(false); setError("") }}>{t("完成")}</ActionButton>
        </FormActions>
      </div> : sites.length ? <SettingsForm onSubmit={async () => {
        const result = await createApiKeyFn({ domain, name: name.trim() })
        setCreated(result.key); setCopied(false); setName("")
        await load()
      }}>
        <Field label="站点"><select className="squircle input" value={domain} onChange={e => setDomain(e.target.value)}>
          {sites.map(site => <option key={site.domain} value={site.domain}>{site.name && site.name !== site.domain ? `${site.name}（${site.domain}）` : site.domain}</option>)}
        </select></Field>
        <Field label="名称" hint="写清用途，例如「博客侧栏」，以后便于识别和删除。">
          <input className="squircle input" value={name} maxLength={100} required autoComplete="off" onChange={e => setName(e.target.value)} />
        </Field>
        <FormActions><ActionButton type="submit" className="btn btn-primary" disabled={!name.trim()}>{t("申请 API Key")}</ActionButton></FormActions>
      </SettingsForm> : keys ? <div className="settings-empty"><UIIcon name="key" /><span>{t("还没有站点，请先在站点列表中添加网站。")}</span></div> : null}
      {error ? <p role="alert" className="mt-3 text-sm text-red-600">{t(error)}</p> : null}
    </Tile>
    <Tile icon="lock" title="已申请的 API Key">
      {keys && keys.length === 0 ? <div className="settings-empty"><UIIcon name="key" /><span>{t("还没有申请 API Key。")}</span></div> : null}
      {keys && keys.length ? <ul className="divide-y divide-gray-100 text-sm">
        {keys.map(key => <li key={key.id} className="settings-list-row flex min-w-0 flex-wrap items-center justify-between gap-3 py-2">
          <span className="min-w-0 flex-1">
            <span className="block [overflow-wrap:anywhere]">{key.name} <span className="text-gray-500">· {key.domain}</span></span>
            <span className="block text-xs text-gray-500"><code>plst_…{key.hint}</code> · {t("创建于 {0}", { 0: formatTimestamp(key.createdAt * 1000, locale) })} · {key.lastUsedAt ? t("最近使用 {0}", { 0: formatTimestamp(key.lastUsedAt * 1000, locale) }) : t("从未使用")}</span>
          </span>
          <ActionButton className="shrink-0 text-red-600 hover:text-red-700" onClick={() => { setError(""); setRemoving(key) }}>{t("删除")}</ActionButton>
        </li>)}
      </ul> : null}
    </Tile>
    <ConfirmModal open={!!removing} title={t("删除这个 API Key")} confirmLabel={t("删除")} danger pending={pending} onClose={() => setRemoving(null)} onSubmit={async () => {
      if (!removing) return
      setPending(true)
      try { await removeApiKeyFn(removing.id); setRemoving(null); await load() } finally { setPending(false) }
    }}>
      <p>{t("删除后，使用 {0} 的程序会立即无法读取统计。", { 0: removing?.name || "" })}</p>
    </ConfirmModal>
  </>
}
