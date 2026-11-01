import {rpc} from "./rpc"
import type {VisitorMode} from "./visitor-mode-preference"

export const visitorModeFn=()=>rpc<{mode:VisitorMode}>("/api/visitor-mode")
export const saveVisitorModeFn=({data}:{data:{mode:VisitorMode}})=>rpc<{mode:VisitorMode}>("/api/visitor-mode","PUT",data)
