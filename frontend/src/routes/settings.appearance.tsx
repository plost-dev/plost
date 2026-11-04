import { createFileRoute } from "@tanstack/react-router"
import { AccentPicker } from "~/components/AccentPicker"
import { Tile } from "~/components/ui"

export const Route = createFileRoute("/settings/appearance")({
  component: () => <><Tile icon="sliders" title="界面外观"><AccentPicker /></Tile></>,
})
