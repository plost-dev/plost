import { BackupSchedulePanel } from "./BackupSchedulePanel"
import { FullBackupPanel } from "./FullBackupPanel"
import "~/styles/backup-settings.css"

export function BackupPanel() {
  return <div className="backup-panel">
    <FullBackupPanel />
    <BackupSchedulePanel />
  </div>
}
