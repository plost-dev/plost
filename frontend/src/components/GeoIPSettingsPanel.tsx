import { useEffect, useState } from "react"
import "~/styles/backup-settings.css"
import { readSettings, useCachedState } from "~/lib/settings-cache"
import { geoipSettingsFn, saveGeoIPSettingsFn, updateGeoIPFn, type GeoIPEdition, type GeoIPSettings } from "~/lib/geoip-actions"
import { formatTimestamp } from "~/lib/format"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { ActionButton } from "./ActionButton"
import { Field } from "./SettingsForm"
import { Tile } from "./ui"
import { Toggle } from "./Toggle"

const EDITIONS: GeoIPEdition[] = ["GeoLite2-City", "GeoLite2-Country"]

/** MaxMind's build date, 20261103, written as 2026-11-03. */
function versionLabel(version?: string) {
  return version && /^\d{8}$/.test(version) ? `${version.slice(0, 4)}-${version.slice(4, 6)}-${version.slice(6)}` : ""
}

/** The build date of a database known only by its timestamp, in the same form as a version. */
function versionOfTime(seconds?: number) {
  return seconds ? new Date(seconds * 1000).toISOString().slice(0, 10) : ""
}

/** MaxMind's logo, in its own colours, in the corner of the card: the data is theirs (GeoLite2). */
function MaxMindWatermark() {
  return <span className="geoip-watermark" aria-hidden="true">
    <img className="settings-brand-logo-color" src="/brands/maxmind.svg?v=20261104" width={104} alt="" />
    <img className="settings-brand-logo-color is-dark" src="/brands/maxmind-dark.svg?v=20261104" width={104} alt="" />
  </span>
}

/** The version and last update, as one pill beside the card title. */
function GeoIPBadge({ text, tone }: { text: string, tone?: "error" }) {
  return text ? <span className={`squircle settings-badge ${tone === "error" ? "is-error" : ""}`}>{text}</span> : null
}

/** Visitors in mainland China placed by api.cnip.io, which names their province and city far better than GeoLite2. */
function ChinaCityTile({ data, onChange }: { data: GeoIPSettings, onChange: (next: GeoIPSettings) => void }) {
  const { t, locale } = useT()
  const release = data.cnVersion
  const [pending, setPending] = useState(false)
  async function change(cnCity: boolean) {
    setPending(true)
    try { onChange(await saveGeoIPSettingsFn({ cnCity })) }
    catch (error) { showToast(error instanceof Error ? error.message : "保存失败", "error") }
    finally { setPending(false) }
  }
  return <Tile icon="map" title="中国访客的省份和城市"
    titleAddon={<a className="settings-heading-link" href="https://cnip.io" target="_blank" rel="noreferrer">{t("打开 cnip.io")}</a>}>
    <Toggle checked={data.cnCity} disabled={pending} onChange={next => void change(next)}>{t("用 cnip.io 查询中国大陆访客的省份和城市")}</Toggle>
    <p className="geoip-cn-note">{release
      ? t("采用 ip2region 付费版，每月更新 · 版本 {0} · 更新于 {1}", { 0: release.version, 1: formatTimestamp(release.updatedAt * 1000, locale) })
      : t("采用 ip2region 付费版，每月更新")}</p>
  </Tile>
}

/** Settings → IP 数据库: where the GeoLite2 database comes from, which one, and how its updates are going. */
export function GeoIPSettingsPage() {
  const { t, locale } = useT()
  const [data, setData] = useCachedState<GeoIPSettings | null>("geoip", null)
  useEffect(() => {
    let live = true
    readSettings<GeoIPSettings>("geoip").then(next => { if (live) setData(next) }, error => showToast(error instanceof Error ? error.message : "读取失败", "error"))
    return () => { live = false }
  }, [setData])
  // While a download runs in the background, look again every two seconds.
  useEffect(() => {
    if (!data?.updating) return
    const timer = setTimeout(() => { void geoipSettingsFn().then(setData, () => {}) }, 2000)
    return () => clearTimeout(timer)
  }, [data, setData])
  if (!data) return null
  return <>
    {data.managed ? <GeoIPSettings data={data} onChange={setData} /> : <Tile icon="globe" title="IP 数据库" actions={<MaxMindWatermark />} titleAddon={data.loaded
      ? <GeoIPBadge text={[versionOfTime(data.fileBuiltAt), data.fileChangedAt ? t("更新于 {0}", { 0: formatTimestamp(data.fileChangedAt * 1000, locale) }) : ""].filter(Boolean).join(" · ")} />
      : <GeoIPBadge text={t("未能加载")} tone="error" />}>
      <p className="text-sm leading-6 text-gray-600">{t("IP 数据库由启动参数 -geoip-db 指定：{0}。Plost 不会下载或更新它，请在服务器上自行更新该文件。", { 0: data.flagPath })}</p>
    </Tile>}
    <ChinaCityTile data={data} onChange={setData} />
  </>
}

function GeoIPSettings({ data, onChange }: { data: GeoIPSettings; onChange: (next: GeoIPSettings) => void }) {
  const { t, locale } = useT()
  const [pending, setPending] = useState(false)
  const [checking, setChecking] = useState(false)
  const state = data.state

  // The source is not a choice on the page: the database always comes from the Plost mirror.
  async function chooseEdition(edition: GeoIPEdition) {
    setPending(true)
    try {
      onChange(await saveGeoIPSettingsFn(data.source === "plost" ? { edition } : { edition, source: "plost" }))
      showToast("已保存，正在后台下载 IP 数据库。")
    } catch (error) {
      showToast(error instanceof Error ? error.message : "保存失败", "error")
    } finally { setPending(false) }
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

  const busy = data.updating || checking
  // Only worth pressing when the mirror has a newer version than the one in use, or there is none yet or the last try failed.
  const canUpdate = !data.loaded || !!state.error || (!!state.latest && state.latest !== state.version)
  const badge = busy ? <GeoIPBadge text={t("正在更新…")} />
    : state.error ? <GeoIPBadge text={t(state.error)} tone="error" />
    : data.loaded ? <GeoIPBadge text={[state.version ? versionLabel(state.version) : versionOfTime(state.builtAt), state.updatedAt ? t("更新于 {0}", { 0: formatTimestamp(state.updatedAt * 1000, locale) }) : ""].filter(Boolean).join(" · ")} />
    : <GeoIPBadge text={t("尚未下载")} />
  return <>
    <Tile icon="globe" title="IP 数据库" titleAddon={badge} actions={<MaxMindWatermark />}>
      <fieldset disabled={pending} className="backup-fields">
        <Field label="数据库">
          <div className="geoip-edition-row">
            <select className="squircle input" value={data.edition} onChange={event => void chooseEdition(event.target.value as GeoIPEdition)}>
              {EDITIONS.map(id => <option key={id} value={id}>{id}</option>)}
            </select>
            <ActionButton className="btn btn-secondary" loading={checking} disabled={busy || !canUpdate} title={canUpdate ? undefined : t("已经是最新版本。")} onClick={() => void check()}>{t("更新")}</ActionButton>
          </div>
        </Field>
      </fieldset>
    </Tile>
  </>
}
