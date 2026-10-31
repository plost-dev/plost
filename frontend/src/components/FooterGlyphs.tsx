import type { ReactNode } from "react"

/**
 * The four footer icons are drawn into the same 20×20 box and each one is scaled so its artwork touches the
 * box on its longer side. Their height and width then match, whatever the shape underneath.
 */
function Glyph({ viewBox, className = "", children }: { viewBox: string, className?: string, children: ReactNode }) {
  return <svg className={`site-footer-glyph ${className}`.trim()} viewBox={viewBox} preserveAspectRatio="xMidYMid meet" fill="currentColor" aria-hidden="true">{children}</svg>
}

export function LanguageGlyph() {
  return <Glyph viewBox="1 2 22 20"><path d="M12.87 15.07l-2.54-2.51.03-.03c1.74-1.94 2.98-4.17 3.71-6.53H17V4h-7V2H8v2H1v1.99h11.17C11.5 7.92 10.44 9.75 9 11.35 8.07 10.32 7.3 9.19 6.69 8h-2c.73 1.63 1.73 3.17 2.98 4.56l-5.09 5.02L4 19l5-5 3.11 3.11.76-2.04zM18.5 10h-2L12 22h2l1.12-3h4.75L21 22h2l-4.5-12zm-2.62 7l1.62-4.33L19.12 17h-3.24z" /></Glyph>
}

const RAYS = Array.from({ length: 8 }, (_, i) => {
  const angle = i * Math.PI / 4
  return { x1: 12 + 8.4 * Math.cos(angle), y1: 12 + 8.4 * Math.sin(angle), x2: 12 + 10.7 * Math.cos(angle), y2: 12 + 10.7 * Math.sin(angle) }
})

export function SunGlyph({ className }: { className?: string }) {
  return <Glyph viewBox="0 0 24 24" className={className}>
    <circle cx="12" cy="12" r="5.6" />
    <g stroke="currentColor" strokeWidth="2.6" strokeLinecap="round">{RAYS.map((ray, i) => <line key={i} {...ray} />)}</g>
  </Glyph>
}

export function MoonGlyph({ className }: { className?: string }) {
  return <Glyph viewBox="3 3 18 18" className={className}><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" /></Glyph>
}

export function GithubGlyph() {
  return <Glyph viewBox="2.12 3.29 11 12.31"><path fillRule="evenodd" d="M5.47 15.59C5.87 15.66 6.02 15.42 6.02 15.21C6.02 15.02 6.01 14.39 6.01 13.72C4 14.09 3.48 13.23 3.32 12.78C3.23 12.55 2.84 11.84 2.5 11.65C2.22 11.5 1.82 11.13 2.49 11.12C3.12 11.11 3.57 11.7 3.72 11.94C4.44 13.15 5.59 12.81 6.05 12.6C6.12 12.08 6.33 11.73 6.56 11.53C4.78 11.33 2.92 10.64 2.92 7.58C2.92 6.71 3.23 5.99 3.74 5.43C3.66 5.23 3.38 4.41 3.82 3.31C3.82 3.31 4.49 3.1 6.02 4.13C6.66 3.95 7.34 3.86 8.02 3.86C8.7 3.86 9.38 3.95 10.02 4.13C11.55 3.09 12.22 3.31 12.22 3.31C12.66 4.41 12.38 5.23 12.3 5.43C12.81 5.99 13.12 6.7 13.12 7.58C13.12 10.65 11.25 11.33 9.47 11.53C9.76 11.78 10.01 12.26 10.01 13.01C10.01 14.08 10 14.94 10 15.21C10 15.42 10.15 15.67 10.55 15.59Z" /></Glyph>
}
