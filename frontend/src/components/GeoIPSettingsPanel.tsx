import { useEffect, useState } from "react"
import "~/styles/backup-settings.css"
import { readSettings, useCachedState } from "~/lib/settings-cache"
import { geoipSettingsFn, saveGeoIPSettingsFn, updateGeoIPFn, type GeoIPSettings } from "~/lib/geoip-actions"
import { formatTimestamp } from "~/lib/format"
import { useT } from "~/lib/i18n"
import { showToast } from "~/lib/toast"
import { ActionButton } from "./ActionButton"
import { Tile } from "./ui"

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

/** Visitors in mainland China are always placed by api.cnip.io (ip2region, sponsored by cnip.io), which names their
 *  province and city far better than GeoLite2. The card says so, shows the release and how much it has been used. */
function ChinaCityTile({ data }: { data: GeoIPSettings }) {
  const { t, locale } = useT()
  const release = data.cnVersion
  const stats = data.cnStats
  const number = (n: number) => n.toLocaleString(locale === "en" ? "en-US" : "zh-CN")
  return <Tile icon="map" title="中国访客的省份和城市" titleAddon={release
    ? <GeoIPBadge text={t("ip2region {0} · 更新于 {1}", { 0: release.version, 1: formatTimestamp(release.updatedAt * 1000, locale) })} />
    : null}>
    <p className="geoip-cn-note">{t("中国大陆访客的省份和城市使用 ip2region 付费版数据，每月更新，由 {0} 赞助。", { 0: "cnip.io" })}{" "}
      <a className="settings-heading-link" href="https://cnip.io" target="_blank" rel="noreferrer">{t("打开 cnip.io")}</a></p>
    {stats ? <dl className="squircle geoip-stats">
      <div><dt>{t("查询 IP")}</dt><dd>{number(stats.calls)}</dd></div>
      <div><dt>{t("定位到城市")}</dt><dd>{number(stats.located)}</dd></div>
      <div><dt>{t("覆盖城市")}</dt><dd>{number(stats.cities)}</dd></div>
    </dl> : null}
    {stats?.since ? <p className="geoip-stats-since">{t("自 {0} 起统计；同一个 IP 一天内只查询一次。", { 0: formatTimestamp(stats.since * 1000, locale) })}</p> : null}
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
    <ChinaCityTile data={data} />
  </>
}

function GeoIPSettings({ data, onChange }: { data: GeoIPSettings; onChange: (next: GeoIPSettings) => void }) {
  const { t, locale } = useT()
  const [checking, setChecking] = useState(false)
  const state = data.state

  async function check() {
    setChecking(true)
    try {
      // The database always comes from the Plost mirror; an instance left on another source is moved back to it.
      if (data.source !== "plost") {
        onChange(await saveGeoIPSettingsFn({ source: "plost" }))
        showToast("正在后台下载 IP 数据库。")
        return
      }
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
  const version = data.loaded ? state.version ? versionLabel(state.version) : versionOfTime(state.builtAt) : ""
  const newer = !!state.latest && state.latest !== state.version
  // Only worth pressing when the mirror has a newer version than the one in use, or there is none yet or the last try failed.
  const canUpdate = data.source !== "plost" || !data.loaded || !!state.error || newer
  const updated = state.updatedAt ? formatTimestamp(state.updatedAt * 1000, locale) : ""
  const badge = busy ? <GeoIPBadge text={t("正在更新…")} />
    : state.error ? <GeoIPBadge text={t(state.error)} tone="error" />
    : !data.loaded ? <GeoIPBadge text={t("尚未下载")} />
    : newer ? <GeoIPBadge text={t("有新版本 {0}", { 0: versionLabel(state.latest) })} />
    : <GeoIPBadge text={updated ? t("已是最新 · 更新于 {0}", { 0: updated }) : t("已是最新")} />
  return <>
    <Tile icon="globe" title="IP 数据库" titleAddon={badge} actions={<MaxMindWatermark />}>
      <div className="geoip-edition-row">
        <span className="geoip-edition-name">GeoLite2-City{version ? <span className="geoip-edition-version">{version}</span> : null}</span>
        <ActionButton className="btn btn-secondary" loading={checking} disabled={busy || !canUpdate} title={canUpdate ? undefined : t("已经是最新版本。")} onClick={() => void check()}>{t("更新")}</ActionButton>
      </div>
    </Tile>
  </>
}
