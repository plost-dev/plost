import { useEffect, useRef, useState } from "react"

function AvatarImage({source,onError}:{source:string;onError:()=>void}) {
  const image=useRef<HTMLImageElement>(null)
  const [loaded,setLoaded]=useState(false)
  useEffect(()=>{
    if(image.current?.complete) {
      if(image.current.naturalWidth>0)setLoaded(true)
      else onError()
    }
  },[source,onError])
  return <img ref={image} src={source} alt="" referrerPolicy="no-referrer" onLoad={()=>setLoaded(true)} onError={onError} style={{position:"absolute",inset:0,width:"100%",height:"100%",objectFit:"cover",opacity:loaded ? 1 : 0}}/>
}

/** Uploaded avatar takes precedence; missing Gravatars fall back to an initial. The frame clips the picture, so it keeps whatever corner shape the class gives it. */
export function UserAvatar({avatar,gravatarUrl,name,className,alt=""}:{avatar?:string|null;gravatarUrl?:string;name:string;className:string;alt?:string}) {
  const [failed,setFailed]=useState<string[]>([])
  const source=[avatar,gravatarUrl].find((url):url is string=>!!url && !failed.includes(url))
  return <span className={className} style={{position:"relative",overflow:"hidden"}} role={alt ? "img" : undefined} aria-label={alt || undefined}>
    {(name || "?").slice(0,1).toUpperCase()}
    {source ? <AvatarImage key={source} source={source} onError={()=>setFailed(previous=>previous.includes(source) ? previous : [...previous,source])}/> : null}
  </span>
}
