import {rpc} from "./rpc"

export type GeoIPSource="plost"|"maxmind"|"off"
export type GeoIPEdition="GeoLite2-City"|"GeoLite2-Country"
export type GeoIPState={source?:GeoIPSource;edition?:GeoIPEdition;sha256?:string;builtAt?:number;size?:number;updatedAt?:number;checkedAt?:number;error?:string}
export type GeoIPSettings={managed:boolean;flagPath:string;loaded:boolean;source:GeoIPSource;edition:GeoIPEdition;account:string;key:string;editable:boolean;updating:boolean;state:GeoIPState}

export const geoipSettingsFn=()=>rpc<GeoIPSettings>("/api/geoip")
export const saveGeoIPSettingsFn=(data:{source?:GeoIPSource;edition?:GeoIPEdition;account?:string;key?:string})=>rpc<GeoIPSettings>("/api/geoip","PUT",data)
export const updateGeoIPFn=()=>rpc<GeoIPSettings>("/api/geoip/update","POST")
