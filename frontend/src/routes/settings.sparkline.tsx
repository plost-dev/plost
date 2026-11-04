import { createFileRoute } from "@tanstack/react-router"
import { SparklineStylePicker } from "~/components/SparklineStylePicker"
import { Tile } from "~/components/ui"

export const Route = createFileRoute("/settings/sparkline")({
  component: () => <><Tile icon="activity" title="站点卡片走势图"><SparklineStylePicker /></Tile></>,
})
