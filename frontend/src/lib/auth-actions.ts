import {startAuthentication,startRegistration} from "@simplewebauthn/browser"
import {rpc} from "./rpc"
import {meFn} from "./actions"
import {normalizeMapboxStyle} from "./mapbox-styles"
import {normalizeGoogleMapStyle} from "./google-map-styles"
import {normalizeSparkline} from "./sparkline-styles"
import QRCode from "qrcode"
export async function rootPreferencesFn(){const [user,prefs]=await Promise.all([meFn(),rpc<{mapboxStyle:string;googleMapStyle?:string;sparklineStyle?:string;sparklineRange?:string}>("/api/preferences")]);return {locale:user?.locale||((localStorage.getItem("plost:locale")==="en")?"en":"zh-CN"),theme:user?.theme||"system",mapboxStyle:normalizeMapboxStyle(prefs.mapboxStyle),googleMapStyle:normalizeGoogleMapStyle(prefs.googleMapStyle),sparkline:normalizeSparkline(prefs.sparklineStyle,prefs.sparklineRange)}}
/** Browsers refuse with these names when the person cancels, times out or repeats a credential. */
function passkeyBrowserError(error:unknown):Error{
  const name=error instanceof Error?error.name:""
  if(name==="NotAllowedError"||name==="AbortError")return new Error("已取消，或操作超时。")
  if(name==="InvalidStateError")return new Error("这台设备上的这把 Passkey 已经添加。")
  return error instanceof Error?error:new Error("Passkey 操作失败")
}
export type PasskeyItem={id:number;name:string;createdAt:number;lastUsedAt:number}
export const passkeyListFn=()=>rpc<PasskeyItem[]>("/api/passkeys")
export async function passkeyLoginFn(){
  const options=await rpc<Parameters<typeof startAuthentication>[0]["optionsJSON"]>("/api/login/passkey/begin","POST",{})
  let response:Awaited<ReturnType<typeof startAuthentication>>
  try{response=await startAuthentication({optionsJSON:options})}catch(error){throw passkeyBrowserError(error)}
  return rpc("/api/login/passkey/finish","POST",response)
}
export async function passkeyAddFn({data}:{data:{password:string;name:string}}){
  const options=await rpc<Parameters<typeof startRegistration>[0]["optionsJSON"]>("/api/passkeys/register/begin","POST",{password:data.password})
  let response:Awaited<ReturnType<typeof startRegistration>>
  try{response=await startRegistration({optionsJSON:options})}catch(error){throw passkeyBrowserError(error)}
  return rpc("/api/passkeys/register/finish"+(data.name?"?name="+encodeURIComponent(data.name):""),"POST",response)
}
export const passkeyRemoveFn=({data}:{data:{id:number;password:string}})=>rpc("/api/passkeys/"+data.id,"DELETE",{password:data.password})
export const verifyLogin2faFn=({data}:{data:{code:string}})=>rpc("/api/login/totp","POST",data)
export async function initiate2faFn({data}:{data:{password:string}}){const r=await rpc<{secret:string;uri:string}>("/api/account/totp","POST",{action:"begin",...data});return {secret:r.secret,qr:await QRCode.toDataURL(r.uri)}}
export async function confirm2faFn({data}:{data:{code:string;password:string}}){const r=await rpc<{recoveryCodes:string[]}>("/api/account/totp","POST",{action:"enable",...data});return {codes:r.recoveryCodes}}
export const disable2faFn=({data}:{data:{code:string;password:string}})=>rpc("/api/account/totp","POST",{action:"disable",...data})
export async function regenRecoveryFn({data}:{data:{code:string;password:string}}){const r=await rpc<{recoveryCodes:string[]}>("/api/account/totp","POST",{action:"recovery",...data});return {codes:r.recoveryCodes}}
