import { createFileRoute } from "@tanstack/react-router"
import { SettingsTips } from "~/components/SettingsTips"
import { SparklineStylePicker } from "~/components/SparklineStylePicker"
import { Tile } from "~/components/ui"

export const Route = createFileRoute("/settings/sparkline")({
  component: () => <><Tile icon="activity" title="站点卡片走势图"><SparklineStylePicker /></Tile><SettingsTips tips={[
    "站点列表每张卡片下方的小走势图。样式和时间范围选择后自动保存到当前账号。",
    "走势图的 24 小时按小时、30 天按天显示访客。",
  ]} /></>,
})
