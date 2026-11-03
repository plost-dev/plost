import { createFileRoute, redirect } from "@tanstack/react-router"

// The general page used to have this address; it is now /settings itself.
export const Route = createFileRoute("/sites/$domain/settings/general")({
  beforeLoad: ({ params }) => { throw redirect({ to: "/sites/$domain/settings", params, replace: true }) },
})
