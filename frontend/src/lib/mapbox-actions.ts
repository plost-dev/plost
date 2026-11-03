import {rpc} from "./rpc"
import type {MapboxStyleId} from "./mapbox-styles"
export const saveMapStyleFn=({data}:{data:{styleId:MapboxStyleId}})=>rpc<{styleId:MapboxStyleId}>("/api/map-style","PUT",data)

export type MapboxSettings={enabled:boolean;configured:boolean}
export const mapboxSettingsFn=()=>rpc<MapboxSettings>("/api/mapbox")
export const saveMapboxSettingsFn=({data}:{data:{enabled:boolean}})=>rpc<MapboxSettings>("/api/mapbox","PUT",data)
