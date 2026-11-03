import { useEffect, useState } from "react"
import "~/styles/backup-settings.css"
import { geoipSettingsFn, saveGeoIPSettingsFn, updateGeoIPFn, type GeoIPEdition, type GeoIPSettings, type GeoIPSource } from "~/lib/geoip-actions"
import { formatTimestamp } from "~/lib/format"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { ActionButton } from "./ActionButton"
import { Field, FormActions, SettingsForm } from "./SettingsForm"
import { SettingsTips } from "./SettingsTips"
import { Tile } from "./ui"

const SOURCES: { id: GeoIPSource; label: string }[] = [
  { id: "plost", label: "Plost 镜像（ipdata.plost.io，无需账号）" },
  { id: "maxmind", label: "自己的 MaxMind 账号" },
  { id: "off", label: "不使用" },
]
const EDITIONS: { id: GeoIPEdition; label: string }[] = [
  { id: "GeoLite2-City", label: "GeoLite2-City（约 50 MB，国家 + 地图点位）" },
  { id: "GeoLite2-Country", label: "GeoLite2-Country（约 3 MB，只有国家）" },
]

function megabytes(size: number) {
  return `${(size / 1048576).toFixed(1)} MB`
}

/** Settings → IP 数据库: where the GeoLite2 database comes from, which one, and how its updates are going. */
export function GeoIPSettingsPage() {
  const { t } = useT()
  const [data, setData] = useState<GeoIPSettings | null>(null)
  useEffect(() => {
    let live = true
    geoipSettingsFn().then(next => { if (live) setData(next) }, error => showToast(error instanceof Error ? error.message : "读取失败", "error"))
    return () => { live = false }
  }, [])
  // While a download runs in the background, look again every two seconds.
  useEffect(() => {
    if (!data?.updating) return
    const timer = setTimeout(() => { void geoipSettingsFn().then(setData, () => {}) }, 2000)
    return () => clearTimeout(timer)
  }, [data])
  if (!data) return null
  return <>
    {data.managed ? <GeoIPSettings data={data} onChange={setData} /> : <Tile icon="globe" title="IP 数据库">
      <p className="text-sm leading-6 text-gray-600">{t("IP 数据库由启动参数 -geoip-db 指定：{0}。Plost 不会下载或更新它，请在服务器上自行更新该文件。", { 0: data.flagPath })}</p>
    </Tile>}
    <SettingsTips tips={[
      "IP 数据库用来判断访客所在的国家；GeoLite2-City 还能让实时访客地图显示到城市。经过 Cloudflare 时优先使用它给出的国家。",
      "Plost 每天检查一次新版本，下载后先核对 SHA-256、再校验整个数据库，通过后才替换正在使用的库，并删除旧版本和未完成的下载。",
      "想直接从 MaxMind 下载：在 MaxMind 官网注册免费账号，在「Manage License Keys」里生成 Key，填进上面的 MaxMind 卡片。保存后会自动改用 MaxMind，并立即检测、下载、校验和替换，之后每天自动检测新版本。账号 ID 和 License Key 只写入 env 文件，不存数据库。",
      "查询全部在本机完成，访客 IP 不会发给任何第三方。",
    ]} />
  </>
}

function GeoIPSettings({ data, onChange }: { data: GeoIPSettings; onChange: (next: GeoIPSettings) => void }) {
  const { t, locale } = useT()
  // Choosing MaxMind waits for the keys when none are saved yet.
  const [source, setSource] = useState<GeoIPSource>(data.source)
  const [pending, setPending] = useState(false)
  const [checking, setChecking] = useState(false)
  const [account, setAccount] = useState("")
  const [key, setKey] = useState("")
  const hasKeys = !!data.account && !!data.key
  const state = data.state

  async function save(change: { source?: GeoIPSource; edition?: GeoIPEdition }) {
    setPending(true)
    try {
      const next = await saveGeoIPSettingsFn(change)
      onChange(next)
      setSource(next.source)
      showToast(next.source === "off" ? "已停用 IP 数据库，本机的库文件已删除。" : "已保存，正在后台下载 IP 数据库。")
    } catch (error) {
      setSource(data.source)
      showToast(error instanceof Error ? error.message : "保存失败", "error")
    } finally { setPending(false) }
  }
  function chooseSource(next: GeoIPSource) {
    if (next === "maxmind" && !hasKeys) {
      showToast("请先在下方 MaxMind 卡片里填写并保存账号 ID 和 License Key", "error")
      document.getElementById("maxmind-account")?.focus()
      return
    }
    setSource(next)
    void save({ source: next })
  }
  // Waits for the background download that a save started, so the page can say how it went.
  async function settled() {
    for (let i = 0; i < 300; i++) {
      await new Promise(resolve => setTimeout(resolve, 2000))
      const next = await geoipSettingsFn()
      onChange(next)
      if (!next.updating) return next
    }
    return null
  }
  async function check() {
    setChecking(true)
    try {
      const next = await updateGeoIPFn()
      const fresh = next.state.updatedAt !== state.updatedAt
      onChange(next)
      showToast(fresh ? "IP 数据库已更新到最新版本。" : "已经是最新版本。")
    } catch (error) {
      showToast(error instanceof Error ? error.message : "更新失败", "error")
      void geoipSettingsFn().then(onChange, () => {})
    } finally { setChecking(false) }
  }

  const status = data.updating || checking ? t("正在更新…") : state.error ? t(state.error) : data.loaded ? t("正常") : data.source === "off" ? t("未使用") : t("尚未下载")
  return <>
    <Tile icon="globe" title="IP 数据库" subtitle="判断访客所在国家和城市用的 MaxMind GeoLite2 数据库">
      <fieldset disabled={pending} className="backup-fields">
        <Field label="数据来源">
          <select className="squircle input" value={source} onChange={event => chooseSource(event.target.value as GeoIPSource)}>
            {SOURCES.map(item => <option key={item.id} value={item.id}>{t(item.label)}</option>)}
          </select>
        </Field>
        <Field label="数据库">
          <select className="squircle input" value={data.edition} disabled={source === "off"} onChange={event => void save({ edition: event.target.value as GeoIPEdition })}>
            {EDITIONS.map(item => <option key={item.id} value={item.id}>{t(item.label)}</option>)}
          </select>
        </Field>
      </fieldset>
      {data.source !== "off" ? <>
        <dl className="squircle backup-status-grid mt-6">
          <div><dt>{t("状态")}</dt><dd className={state.error && !data.updating && !checking ? "text-red-600" : undefined}>{status}</dd></div>
          <div><dt>{t("数据版本")}</dt><dd>{data.loaded && state.builtAt ? formatTimestamp(state.builtAt * 1000, locale) : "—"}</dd></div>
          <div><dt>{t("上次更新")}</dt><dd>{data.loaded && state.updatedAt ? formatTimestamp(state.updatedAt * 1000, locale) : "—"}</dd></div>
          <div><dt>{t("上次检查")}</dt><dd>{state.checkedAt ? formatTimestamp(state.checkedAt * 1000, locale) : "—"}</dd></div>
          {data.loaded && state.size ? <div><dt>{t("文件大小")}</dt><dd>{megabytes(state.size)}</dd></div> : null}
        </dl>
        <FormActions>
          <ActionButton className="btn btn-secondary" loading={checking} disabled={data.updating || pending} onClick={() => void check()}>{t("立即检查更新")}</ActionButton>
        </FormActions>
      </> : null}
    </Tile>
    <Tile icon="key" title="MaxMind" logo={{ src: "/brands/maxmind.svg", width: 128 }}
      titleAddon={<a className="settings-heading-link" href="https://www.maxmind.com/en/accounts/current/license-key" target="_blank" rel="noreferrer">{t("打开 MaxMind 获取 License Key")}</a>}>
      <SettingsForm onSubmit={async () => {
        const next = await saveGeoIPSettingsFn({ source: "maxmind", account: account.trim(), key: key.trim() })
        onChange(next); setSource(next.source); setAccount(""); setKey("")
        showToast("已写入 env 文件，正在从 MaxMind 检测并下载最新版本…")
        const done = await settled()
        if (!done) return
        if (done.state.error) throw new Error(done.state.error)
        showToast("已从 MaxMind 下载最新版本并换用，之后每天自动检测更新。")
      }}>
        <div className="backup-fields">
          <Field label="账号 ID（Account ID）" hint={data.editable ? undefined : t("启动 Plost 时没有指定 env 文件（-env-file），请直接在 env 文件里修改 {0}。", { 0: "PLOST_MAXMIND_ACCOUNT_ID" })}>
            <input id="maxmind-account" className="squircle input" inputMode="numeric" autoComplete="off" spellCheck={false} required disabled={!data.editable}
              value={account} placeholder={data.account || "1234567"} onChange={event => setAccount(event.target.value)} />
          </Field>
          <Field label="License Key">
            <input className="squircle input" autoComplete="off" spellCheck={false} required disabled={!data.editable}
              value={key} placeholder={data.key || "xxxxxx_…_mmk"} onChange={event => setKey(event.target.value)} />
          </Field>
        </div>
        {data.editable ? <FormActions><ActionButton type="submit" className="btn btn-primary">{t(hasKeys ? "保存并更新" : "保存并下载")}</ActionButton></FormActions> : null}
      </SettingsForm>
    </Tile>
  </>
}
