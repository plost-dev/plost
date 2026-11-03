import {rpc} from "./rpc"

export type ClientIPWarning={code:"untrusted_proxy"|"proxy_addresses"|"cdn_addresses"|"shared_network";share:number;suggest?:string}
export type ClientIPCheck={
  config:{trusted:string[];header:string;geoip:boolean}
  request:{peer:string;peerTrusted:boolean;headers:Record<string,string>;address:string;kind:string;via:string;known:boolean;country:string}
  recent:{total:number;via:Record<string,number>;kinds:Record<string,number>;ignored:number;networks:number;topShare:number;warnings:ClientIPWarning[]}
}
export const clientIPCheckFn=()=>rpc<ClientIPCheck>("/api/client-ip")
