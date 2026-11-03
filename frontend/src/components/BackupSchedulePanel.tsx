import { useCallback, useEffect, useState } from "react"
import { Tile } from "./ui"
import { ActionButton } from "./ActionButton"
import { HelpHint } from "./HelpHint"
import { RestoreForm } from "./RestoreForm"
import { Field, SettingsForm } from "./SettingsForm"
import { Toggle } from "./Toggle"
import { UIIcon } from "./UIIcon"
import { formatTimestamp } from "~/lib/format"
import { useT } from "~/lib/i18n"
import { rpc } from "~/lib/rpc"
import { showToast } from "~/lib/toast"
import { PasswordInput } from "~/components/PasswordInput"
import { EnvKeyStatus } from "./EnvKeyStatus"

type Settings = {
  provider: "r2" | "s3", endpoint: string, region: string, bucket: string, prefix: string
  hasAccessKey: boolean, hasSecretKey: boolean, hasBackupPassword: boolean
  cloudEnabled: boolean, cloudConfigured: boolean
  schedule: "off" | "hourly" | "daily" | "weekly", hour: number, minute: number, weekday: number, keep: number
  lastRunAt: number, lastAttemptAt: number, lastStatus: string, lastBackup: string, lastError: string
  effectivePrefix: string, scheduleTimezone: string, running: boolean
}
// The credentials and the backup password come from the env file; only the account password is typed here.
type Secrets = { currentPassword: string }
type BackupFile = { name: string, size: number, modified: number }

const NO_SECRETS: Secrets = { currentPassword: "" }
const EDITABLE = ["provider", "endpoint", "region", "bucket", "prefix", "cloudEnabled", "schedule", "hour", "minute", "weekday", "keep"] as const
const STORAGE = new Set<keyof Settings>(["provider", "endpoint", "region", "bucket", "prefix"])
const sizeLabel = (bytes: number) => bytes >= 1 << 20 ? (bytes / (1 << 20)).toFixed(1) + " MiB" : Math.max(1, Math.round(bytes / 1024)) + " KiB"

export function BackupSchedulePanel() {
  const { t, locale } = useT()
  const [saved, setSaved] = useState<Settings | null>(null)
  const [form, setForm] = useState<Settings | null>(null)
  const [secrets, setSecrets] = useState<Secrets>(NO_SECRETS)
  const [loadError, setLoadError] = useState("")
  const [local, setLocal] = useState<BackupFile[]>([])
  const [cloud, setCloud] = useState<BackupFile[] | null>(null)
  const [connection, setConnection] = useState<{ bucket: string } | null>(null)
  const [connectionError, setConnectionError] = useState("")
  const [restoring, setRestoring] = useState<{ source: string, label: string } | null>(null)
  const [msg, setMsg] = useState("")
  const [err, setErr] = useState("")
  const [busy, setBusy] = useState("")

  // Stored errors can join two messages with "：", so each part is translated on its own.
  const message = (text: string) => text.split("：").map(part => t(part)).join(locale === "en" ? ": " : "：")

  const loadLocal = useCallback(() => rpc<BackupFile[]>("/api/backup/local").then(setLocal, () => undefined), [])
  useEffect(() => {
    rpc<Settings>("/api/backup/settings").then(next => { setSaved(next); setForm(next) }, error => setLoadError(error instanceof Error ? error.message : "加载失败，请刷新页面重试"))
  }, [])

  // Backups also start on their own schedule, so keep following the server: closely while one runs, slowly otherwise.
  const loaded = !!saved
  const running = saved?.running
  const attempt = saved?.lastAttemptAt
  useEffect(() => {
    if (!loaded) return
    const timer = setInterval(() => {
      rpc<Settings>("/api/backup/settings").then(setSaved, () => undefined)
    }, running ? 1500 : 30000)
    return () => clearInterval(timer)
  }, [loaded, running])
  // Local files change whenever a backup starts, finishes or is pruned.
  useEffect(() => { if (loaded) void loadLocal() }, [loaded, running, attempt, loadLocal])

  if (!saved || !form) {
    return <Tile icon="cloud" id="storage" title={t("对象存储")}>
      {loadError ? <p role="alert" className="squircle settings-feedback is-error">{t(loadError)}</p> : <p className="settings-hint">{t("加载中…")}</p>}
    </Tile>
  }

  const dirty = EDITABLE.some(key => form[key] !== saved[key])
  const frequency = form.schedule
  const hasKeys = saved.hasAccessKey && saved.hasSecretKey
  const ready = !!form.bucket.trim() && hasKeys && (form.provider !== "r2" || !!form.endpoint.trim()) && !!form.region.trim()
  const canRun = saved.hasBackupPassword && (saved.keep > 0 || saved.cloudEnabled) && (!saved.cloudEnabled || saved.cloudConfigured)
  const status = saved.running ? "正在备份…" : saved.lastStatus === "error" ? "最近一次备份失败" : saved.lastRunAt ? "备份成功" : "尚未执行备份"

  function set<K extends keyof Settings>(key: K, value: Settings[K]) {
    setMsg(""); setErr("")
    if (STORAGE.has(key)) { setConnection(null); setConnectionError(""); setCloud(null) }
    setForm(cur => cur && {
      ...cur, [key]: value,
      ...(key === "provider" ? { region: value === "r2" ? "auto" : cur.region === "auto" ? "" : cur.region } : {}),
    } as Settings)
  }
  function secret(key: keyof Secrets, value: string) { setMsg(""); setErr(""); setSecrets(cur => ({ ...cur, [key]: value })) }

  const storageData = () => ({ provider: form.provider, endpoint: form.endpoint, region: form.region, bucket: form.bucket, prefix: form.prefix })

  async function perform(kind: string, action: () => Promise<void>) {
    if (busy) return
    setBusy(kind); setMsg(""); setErr("")
    try { await action() }
    catch (error) { setErr(error instanceof Error ? t(error.message) : t("操作失败，请重试")) }
    finally { setBusy("") }
  }

  async function save() {
    const next = await rpc<Settings>("/api/backup/settings", "PUT", {
      ...storageData(), currentPassword: secrets.currentPassword,
      cloudEnabled: form!.cloudEnabled, schedule: form!.schedule, hour: form!.hour, minute: form!.minute, weekday: form!.weekday, keep: form!.keep,
    })
    setSaved(next); setForm(next); setSecrets(NO_SECRETS); setConnection(null)
    showToast("备份设置已保存")
  }

  return <>
    <SettingsForm onSubmit={save}>
      <fieldset disabled={!!busy} className="backup-form-fields">
        <Tile icon="cloud" brand={form.provider === "r2" ? "cloudflare" : undefined} id="storage" title={t("对象存储")}>
          <div className="mb-6"><Toggle checked={form.cloudEnabled} onChange={next => set("cloudEnabled", next)}>{t("每次备份后上传到云存储")}</Toggle></div>
          <div className="backup-fields">
            <Field label={t("存储服务")}>
              <select className="squircle input" value={form.provider} onChange={e => set("provider", e.target.value as Settings["provider"])}>
                <option value="r2">Cloudflare R2</option><option value="s3">Amazon S3</option>
              </select>
            </Field>
            <Field label={t("存储桶（Bucket）")} hint={t("填写已创建的桶名称，不含路径。")}>
              <input className="squircle input" maxLength={63} value={form.bucket} placeholder="plost-backups" autoCapitalize="none" spellCheck={false} onChange={e => set("bucket", e.target.value)} />
            </Field>
            <Field wide label={t("服务地址（Endpoint）")} hint={form.provider === "r2" ? t("在 R2 存储桶概览中复制 S3 API 地址；不要填写公开访问域名。") : t("标准 Amazon S3 可留空；使用自定义地址时请填写完整的 HTTPS 地址。")}>
              <input className="squircle input" type="url" required={form.provider === "r2" && form.cloudEnabled} maxLength={2048} value={form.endpoint} placeholder={form.provider === "r2" ? "https://<ACCOUNT_ID>.r2.cloudflarestorage.com" : "https://s3.us-east-1.amazonaws.com"} autoCapitalize="none" spellCheck={false} onChange={e => set("endpoint", e.target.value)} />
            </Field>
            <Field label={t("区域（Region）")}>
              <input className="squircle input" title={form.provider === "r2" ? t("R2 使用 auto，系统自动选择区域。") : t("填写存储桶所在的 AWS 区域，例如 us-east-1。")} maxLength={64} readOnly={form.provider === "r2"} value={form.region} placeholder="us-east-1" spellCheck={false} onChange={e => set("region", e.target.value)} />
            </Field>
            <Field label={t("备份目录前缀")} hint={t("备份存放在 {0}/backups/ 下，留空使用默认目录。", { 0: form.prefix.trim() || saved.effectivePrefix })}>
              <input className="squircle input" maxLength={512} value={form.prefix} placeholder={t("默认备份目录")} spellCheck={false} onChange={e => set("prefix", e.target.value)} />
            </Field>
          </div>
          <div className="backup-credentials">
            <div className="flex items-center gap-2"><h3>{t("访问凭据")}</h3><HelpHint>{t("密钥写在服务的 env 文件里，不存数据库，恢复备份也不会丢；改完重启 Plost 生效。")}</HelpHint></div>
            <div className="backup-fields">
              <EnvKeyStatus label="Access Key ID" name="PLOST_BACKUP_ACCESS_KEY" configured={saved.hasAccessKey}/>
              <EnvKeyStatus label="Secret Access Key" name="PLOST_BACKUP_SECRET_KEY" configured={saved.hasSecretKey}/>
            </div>
          </div>
          <div className="backup-test-actions">
            <ActionButton disabled={!!busy || !ready} loading={busy === "test"} onClick={async () => {
              if (busy) return
              setBusy("test"); setConnection(null); setConnectionError(""); setMsg(""); setErr("")
              try { setConnection(await rpc<{ bucket: string }>("/api/backup/cloud/test", "POST", storageData())) }
              catch (error) { setConnectionError(error instanceof Error ? t(error.message) : t("连接测试失败，请重试")) }
              finally { setBusy("") }
            }}>{t(busy === "test" ? "测试中…" : "测试连接")}</ActionButton>
          </div>
          {connection ? <div role="status" className="squircle backup-connection-result"><UIIcon name="check" /><div className="min-w-0"><strong>{t("连接成功，可以读取备份目录")}</strong><p>{t("已验证 {0} 的读取权限，未上传或删除文件。", { 0: connection.bucket })}</p><p>{t("上传权限将在执行备份时验证。")}</p></div></div> : null}
          {connectionError ? <p role="alert" className="squircle settings-feedback is-error">{connectionError}</p> : null}
        </Tile>
        <Tile icon="clock" id="schedule" title={t("自动备份")}>
          <div className="backup-fields">
            <Field label={t("备份频率")} hint={frequency === "hourly" ? t("距离上次成功备份满一小时后执行，服务每 30 秒检查一次。") : undefined}>
              <select className="squircle input" title={t("选择“仅手动”可关闭自动备份。")} value={frequency} onChange={e => set("schedule", e.target.value as Settings["schedule"])}>
                <option value="off">{t("仅手动")}</option><option value="hourly">{t("每小时")}</option><option value="daily">{t("每天")}</option><option value="weekly">{t("每周")}</option>
              </select>
            </Field>
            {frequency === "weekly" ? <Field label={t("执行日期")}>
              <select className="squircle input" value={form.weekday} onChange={e => set("weekday", Number(e.target.value))}>
                {["星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"].map((day, i) => <option key={day} value={i}>{t(day)}</option>)}
              </select>
            </Field> : null}
            {frequency === "daily" || frequency === "weekly" ? <Field label={t("执行时间")} hint={t("按服务器时区 {0} 执行。", { 0: saved.scheduleTimezone })}>
              <input className="squircle input" type="time" required value={String(form.hour).padStart(2, "0") + ":" + String(form.minute).padStart(2, "0")} onChange={e => {
                if (!/^\d{2}:\d{2}$/.test(e.target.value)) return
                const [hour, minute] = e.target.value.split(":").map(Number)
                setMsg(""); setErr(""); setForm(cur => cur && { ...cur, hour, minute })
              }} />
            </Field> : null}
            <Field label={t("本地保留份数")} hint={t("最近的备份保存在数据目录的 backups/scheduled 中。0 表示不在本地保留，需要启用云存储上传。")}>
              <input className="squircle input" type="number" min={0} max={90} step={1} required value={form.keep} onChange={e => set("keep", Math.max(0, Math.min(90, Math.trunc(Number(e.target.value) || 0))))} />
            </Field>
            <EnvKeyStatus wide label={t("计划备份密码")} name="PLOST_BACKUP_PASSWORD" configured={saved.hasBackupPassword}/>
            <Field wide label={t("当前管理员密码")} hint={t("修改备份去向时需要确认身份。")}>
              <PasswordInput className="squircle input" required={dirty} autoComplete="current-password" value={secrets.currentPassword} onChange={e => secret("currentPassword", e.target.value)} />
            </Field>
          </div>
          <div className="settings-actions backup-save-actions">
            <ActionButton type="submit" className="btn btn-primary" disabled={!!busy || !dirty}>{t("保存设置")}</ActionButton>
            <span>{t(dirty ? "有未保存的修改" : "设置已保存")}</span>
          </div>
        </Tile>
      </fieldset>
    </SettingsForm>
    {msg ? <p role="status" className="squircle settings-feedback">{msg}</p> : null}
    {err ? <p role="alert" className="squircle settings-feedback is-error">{err}</p> : null}
    <Tile icon="cloud" id="restore" title={t("备份记录与恢复")}>
      <dl className="squircle backup-status-grid">
        <div><dt>{t("上次成功备份")}</dt><dd>{saved.lastRunAt ? formatTimestamp(saved.lastRunAt * 1000, locale) : t("尚无成功备份")}</dd></div>
        <div><dt>{t("备份状态")}</dt><dd>{t(status)}</dd></div>
        {saved.lastBackup ? <div className="backup-status-id"><dt>{t("备份编号")}</dt><dd>{saved.lastBackup}</dd></div> : null}
      </dl>
      {saved.lastStatus === "error" && saved.lastError ? <p className="squircle settings-feedback is-error">{t("最近一次错误：{0}", { 0: message(saved.lastError) })}</p> : null}
      {dirty ? <p className="backup-unsaved-hint">{t("请先保存修改，再执行备份、扫描或恢复。连接测试可以直接使用当前填写的信息。")}</p> : null}
      <div className="backup-run-actions">
        <ActionButton loading={busy === "run" || saved.running} disabled={!!busy || saved.running || dirty || !canRun} onClick={() => perform("run", async () => {
          await rpc("/api/backup/run", "POST", {})
          setSaved(cur => cur && { ...cur, running: true })
          setMsg(t("已开始备份，完成后会显示在这里。"))
        })}>{t(saved.running ? "备份中" : "立即备份")}</ActionButton>
        <ActionButton loading={busy === "scan"} disabled={!!busy || dirty || !saved.cloudConfigured} onClick={() => perform("scan", async () => {
          const rows = await rpc<BackupFile[]>("/api/backup/cloud/scan", "POST", {})
          setCloud(rows)
          setMsg(rows.length ? t("找到 {0} 份备份", { 0: rows.length }) : t("这个桶里还没有 Plost 备份"))
        })}>{t(busy === "scan" ? "扫描中" : "扫描云端备份")}</ActionButton>
      </div>
      <BackupList title="本地备份" empty="还没有本地备份。" items={local} source="local" onDownload={name => location.assign("/api/backup/local/" + encodeURIComponent(name))} onRestore={setRestoring} disabled={!!busy || dirty} />
      {cloud ? <BackupList title="云端备份" empty="这个目录下还没有 Plost 备份" items={cloud} source="cloud" onRestore={setRestoring} disabled={!!busy || dirty} /> : null}
      {restoring ? <RestoreForm key={restoring.source} source={restoring.source} label={restoring.label} onCancel={() => setRestoring(null)} /> : null}
      <p className="backup-restore-note"><UIIcon name="warn" />{t("恢复会覆盖当前数据库。请确认备份后再操作。")}</p>
    </Tile>
  </>
}

function BackupList({ title, empty, items, source, disabled, onDownload, onRestore }: {
  title: string, empty: string, items: BackupFile[], source: "local" | "cloud", disabled: boolean
  onDownload?: (name: string) => void, onRestore: (target: { source: string, label: string }) => void
}) {
  const { t, locale } = useT()
  return <div className="mt-6">
    <h3 className="text-sm font-medium text-gray-800">{t(title)}</h3>
    {items.length ? <ul className="backup-result-list full-backup-list">
      {items.map(item => <li key={item.name} className="backup-result-row">
        <div className="min-w-0"><strong className="full-backup-filename">{item.name}</strong><p>{formatTimestamp(item.modified * 1000, locale)} · {sizeLabel(item.size)}</p></div>
        <div className="backup-run-actions">
          {onDownload ? <ActionButton disabled={disabled} onClick={() => onDownload(item.name)}>{t("下载")}</ActionButton> : null}
          <ActionButton className="btn btn-danger" disabled={disabled} onClick={() => onRestore({ source: source + ":" + item.name, label: t(source === "local" ? "本地备份 {0}" : "云端备份 {0}", { 0: item.name }) })}>{t("恢复此备份")}</ActionButton>
        </div>
      </li>)}
    </ul> : <div className="backup-empty"><UIIcon name="cloud" /><p>{t(empty)}</p></div>}
  </div>
}
