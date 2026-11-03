import { rpc } from "./rpc"

/** A read-only key for /api/v1/stats; the full key is only returned once, when it is created. */
export type ApiKey = { id: number; name: string; domain: string; hint: string; createdAt: number; lastUsedAt: number }
export const apiKeysFn = () => rpc<{ keys: ApiKey[] }>("/api/api-keys")
export const createApiKeyFn = (data: { domain: string; name: string }) => rpc<{ key: string }>("/api/api-keys", "POST", data)
export const removeApiKeyFn = (id: number) => rpc("/api/api-keys/" + id, "DELETE")
