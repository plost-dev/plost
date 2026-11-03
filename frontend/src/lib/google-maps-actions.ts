import {rpc} from "./rpc"
import type {GoogleMapStyleId} from "./google-map-styles"

export type GoogleMapsSettings={enabled:boolean;configured:boolean}
export const googleMapsSettingsFn=()=>rpc<GoogleMapsSettings>("/api/google-maps")
export const saveGoogleMapsSettingsFn=({data}:{data:{enabled:boolean}})=>rpc<GoogleMapsSettings>("/api/google-maps","PUT",data)
export const saveGoogleMapStyleFn=({data}:{data:{styleId:GoogleMapStyleId}})=>rpc<{styleId:GoogleMapStyleId}>("/api/google-map-style","PUT",data)
