// Shapes returned by GET /api/sites/{domain}/checks. Absent values arrive as null, an empty string or not at all.
export type HttpCheck = {checkedAt: string; ok: boolean; status: number | null; durationMs: number | null; error: string | null}

export type DomainExpiry = {
  domain?: string | null
  expiresAt?: string | null
  error?: string | null
  registeredAt?: string | null
  registrar?: string | null
  retryable?: boolean
  stale?: boolean
  lastSuccessAt?: string
}

export type CertificateResult = {
  expiresAt?: string | null
  valid: boolean
  error?: string | null
  validation?: "DV" | "OV" | "EV" | null
  stale?: boolean
  lastSuccessAt?: string
}

export type ExpirySnapshot = {checkedAt: string; domain: DomainExpiry; certificate: CertificateResult}

export type SiteCheckReport = {
  enabled: boolean
  path: string
  url: string
  expiry: ExpirySnapshot | null
  latest: HttpCheck | null
  checks: HttpCheck[]
  now: string
}
