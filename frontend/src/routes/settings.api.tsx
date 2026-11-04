import { createFileRoute } from "@tanstack/react-router"
import { ApiKeyPanel } from "~/components/ApiKeyPanel"

export const Route = createFileRoute("/settings/api")({
  component: () => <><ApiKeyPanel /></>,
})
