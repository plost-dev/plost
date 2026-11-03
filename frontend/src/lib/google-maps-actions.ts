import {rpc} from "./rpc"

export type GoogleMapsSettings={enabled:boolean;configured:boolean}
export const googleMapsSettingsFn=()=>rpc<GoogleMapsSettings>("/api/google-maps")
export const saveGoogleMapsSettingsFn=({data}:{data:{enabled:boolean;key?:string}})=>rpc<GoogleMapsSettings>("/api/google-maps","PUT",data)
