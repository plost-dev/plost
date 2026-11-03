import { createFileRoute } from "@tanstack/react-router"
import { BackupPanel } from "~/components/BackupPanel"

export const Route = createFileRoute("/settings/backup")({ component: BackupPanel })
