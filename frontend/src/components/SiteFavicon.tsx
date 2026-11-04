import { ImageWithFallback } from "./ImageWithFallback"

/** Site identity keeps the same dimensions when a remote icon is unavailable. */
/** `version` is when the site's own uploaded icon last changed (0 for none), so a new icon shows at once. */
export function SiteFavicon({domain, version = 0, className = "size-4"}: {domain: string; version?: number; className?: string}) {
  const src = `/favicon/sources/${encodeURIComponent(domain)}${version ? `?v=${version}` : ""}`
  const classes = `site-favicon block shrink-0 object-contain ${className}`
  return <ImageWithFallback src={src} className={classes} fallback={
    <svg aria-hidden="true" viewBox="0 0 20 20" fill="none" stroke="#71717a" strokeWidth="1.5" className={classes}>
      <circle cx="10" cy="10" r="7.5"/><ellipse cx="10" cy="10" rx="3.25" ry="7.5"/><path d="M2.5 10h15"/>
    </svg>
  }/>
}
