import { createFileRoute } from "@tanstack/react-router"
import { AccentPicker } from "~/components/AccentPicker"
import { SettingsTips } from "~/components/SettingsTips"
import { Tile } from "~/components/ui"

export const Route = createFileRoute("/settings/appearance")({
  component: () => <><Tile icon="sliders" title="界面外观"><AccentPicker /></Tile><SettingsTips tips={[
    "界面配色立即生效，保存在当前浏览器，换一台电脑或浏览器需要重新选择。",
    "深浅色在页脚的主题按钮切换。",
  ]} /></>,
})
