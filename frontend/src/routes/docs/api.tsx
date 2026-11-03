import {createFileRoute,redirect} from "@tanstack/react-router"
import {API_DOCS_URL} from "~/components/ApiKeyPanel"
import {meFn} from "~/lib/actions"

// The API guide moved to the website; signed in, this opens the page for API keys instead.
export const Route=createFileRoute("/docs/api")({loader:async()=>{if(await meFn().catch(()=>null))throw redirect({to:"/account",search:{tab:"api"}});throw redirect({href:API_DOCS_URL})}})
