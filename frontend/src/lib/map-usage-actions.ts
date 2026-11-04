import {rpc} from "./rpc"

/** Where the chosen map service is used: each site's statistics page, the site overview's globe, or both. */
export type MapUsage={detail:boolean;overview:boolean}
export const mapUsageFn=()=>rpc<MapUsage>("/api/map-usage")
export const saveMapUsageFn=(data:Partial<MapUsage>)=>rpc<MapUsage>("/api/map-usage","PUT",data)

/** The map the site overview draws: a service with its browser key, or "" for the built-in globe. */
export type OverviewMapConfig={service:""|"mapbox"|"google";key?:string}
export const overviewMapFn=()=>rpc<OverviewMapConfig>("/api/overview/map")
