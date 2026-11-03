import {rpc} from "./rpc"
import type {GoogleMapStyleId} from "./google-map-styles"

/** previewKey is the saved key, returned only while Google Maps is on. */
export type GoogleMapsSettings={enabled:boolean;configured:boolean;previewKey?:string}
export const googleMapsSettingsFn=()=>rpc<GoogleMapsSettings>("/api/google-maps")
export const saveGoogleMapsSettingsFn=({data}:{data:{enabled:boolean;key?:string}})=>rpc<GoogleMapsSettings>("/api/google-maps","PUT",data)
export const saveGoogleMapStyleFn=({data}:{data:{styleId:GoogleMapStyleId}})=>rpc<{styleId:GoogleMapStyleId}>("/api/google-map-style","PUT",data)
