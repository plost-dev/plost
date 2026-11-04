import {rpc} from "./rpc"

export type GeoIPSource="plost"|"online"|"off"
export type GeoIPEdition="GeoLite2-City"
export type GeoIPState={source?:GeoIPSource;edition?:GeoIPEdition;sha256?:string;version?:string;latest?:string;latestAt?:number;builtAt?:number;size?:number;updatedAt?:number;checkedAt?:number;error?:string}
export type GeoIPSettings={managed:boolean;flagPath:string;loaded:boolean;source:GeoIPSource;edition:GeoIPEdition;updating:boolean;state:GeoIPState;fileBuiltAt?:number;fileChangedAt?:number;cnCity:boolean;cnVersion?:{version:string;updatedAt:number}}

export const geoipSettingsFn=()=>rpc<GeoIPSettings>("/api/geoip")
export const saveGeoIPSettingsFn=(data:{source?:GeoIPSource;edition?:GeoIPEdition;cnCity?:boolean})=>rpc<GeoIPSettings>("/api/geoip","PUT",data)
export const updateGeoIPFn=()=>rpc<GeoIPSettings>("/api/geoip/update","POST")
