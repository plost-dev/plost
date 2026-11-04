import { BackupSchedulePanel } from "./BackupSchedulePanel"
import { FullBackupPanel } from "./FullBackupPanel"
import { SettingsTips } from "./SettingsTips"
import "~/styles/backup-settings.css"

export function BackupPanel() {
  return <div className="backup-panel">
    <FullBackupPanel />
    <BackupSchedulePanel />
    <SettingsTips tips={[
      "完整备份统一保存账号、站点配置与全部访问数据，加密成一个 .plostbk 文件。备份密码至少 12 个字符，用来恢复，不能找回。",
      "恢复前会自动保存当前数据，再还原到新数据库并校验，成功后才切换；旧数据库保留。",
      "备份和恢复期间会短暂暂停采集与后台访问，完成后自动恢复。",
      "计划备份保存在数据目录的 backups/scheduled 中，按「本地保留份数」只留最近几份；填 0 则不在本地保留，需要打开云存储上传。",
      "计划备份与手动完整备份使用相同的加密格式。只能恢复此 Go 版生成的备份，旧版 PostgreSQL / ClickHouse 的备份不能直接恢复。",
      "云端旧备份不会被自动删除，请在存储桶中设置生命周期规则。「测试连接」只检查当前填写的信息，不会保存设置或创建备份。",
    ]} />
  </div>
}
