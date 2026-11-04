import { createFileRoute } from "@tanstack/react-router"
import { AccentPicker } from "~/components/AccentPicker"
import { SparklineStylePicker } from "~/components/SparklineStylePicker"
import { Tile } from "~/components/ui"

export const Route = createFileRoute("/settings/appearance")({
  component: () => <>
    <Tile icon="sliders" title="界面外观"><AccentPicker /></Tile>
    <Tile icon="activity" title="站点卡片走势图"><SparklineStylePicker /></Tile>
  </>,
})
