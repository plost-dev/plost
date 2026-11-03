import {rpc} from "./rpc"

export type ClientIPWarning={code:"untrusted_proxy"|"proxy_addresses"|"cdn_addresses"|"shared_network";share:number;suggest?:string}
export type ClientIPHop={address:string;kind:string;trusted:boolean;visitor:boolean}
export type ClientIPCheck={
  config:{trusted:string[];header:string;geoip:boolean}
  request:{peer:string;peerTrusted:boolean;headers:Record<string,string>;address:string;kind:string;via:string;known:boolean;country:string;hops:ClientIPHop[]}
  recent:{total:number;via:Record<string,number>;kinds:Record<string,number>;ignored:number;networks:number;topShare:number;warnings:ClientIPWarning[]}
  suggested:string
}
export const clientIPCheckFn=()=>rpc<ClientIPCheck>("/api/client-ip")
