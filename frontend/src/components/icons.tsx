import { UIIcon } from "./UIIcon"
import { ImageWithFallback } from "./ImageWithFallback"
import { campaignTag, isDirectSource, sourceIconSrc } from "~/lib/source-icons"
import { browserDisplayName } from "~/lib/report-values"

// Original-color SVGs are pinned to The SVG; unsupported products retain their
// existing local artwork instead of borrowing a different product's brand.
const platform = (file: string) => `/brands/platforms/${file}.svg`

const BROWSER_ICONS: Record<string, string> = {
  Chrome: platform("chrome"),
  "Google Chrome": platform("chrome"),
  "Chrome Mobile": platform("chrome"),
  "Chrome Mobile iOS": platform("chrome"),
  "Chrome Headless": platform("chrome"),
  Chromium: platform("chromium"),
  "Chromium WebView": platform("chrome"),
  "Android Browser": platform("android"),
  Instagram: "/brands/sources/instagram.svg",
  Facebook: "/brands/sources/facebook.svg",
  Safari: platform("safari"),
  "Mobile Safari": platform("safari"),
  "Safari Mobile": platform("safari"),
  Firefox: platform("firefox"),
  "Firefox Mobile": platform("firefox"),
  "Firefox iOS": platform("firefox"),
  "Microsoft Edge": platform("edge"),
  Edge: platform("edge"),
  "Edge Mobile": platform("edge"),
  "Edge iOS": platform("edge"),
  Opera: platform("opera"),
  "Opera Mobile": platform("opera"),
  "Opera Mini": platform("opera"),
  Brave: platform("brave-browser"),
  "Brave Browser": platform("brave-browser"),
  "Internet Explorer": platform("internetexplorer"),
  IE: platform("internetexplorer"),
  Vivaldi: platform("vivaldi"),
  "Samsung Browser": platform("samsung-browser"),
  "Samsung Internet": platform("samsung-browser"),
  "DuckDuckGo Privacy Browser": platform("duckduckgo"),
  "DuckDuckGo Browser": platform("duckduckgo"),
  DuckDuckGo: platform("duckduckgo"),
  curl: platform("curl"),
  Ecosia: platform("ecosia"),
  "Yandex Browser": "/images/icon/browser/yandex.png",
  "QQ Browser": "/images/icon/browser/qq.png",
  "UC Browser": "/images/icon/browser/uc.svg",
  // The vendors' own marks: their browsers' icons are near-identical blue globes.
  "MIUI Browser": platform("xiaomi"),
  "Huawei Browser Mobile": platform("huawei"),
  "Huawei Browser": platform("huawei"),
  "vivo Browser": platform("vivo"),
  "iOS WebView": platform("safari"),
  "macOS WebView": platform("safari"),
  WeChat: "/brands/sources/wechat.svg",
  QQ: "/brands/sources/qq.svg",
  Weibo: "/brands/sources/sina-weibo.svg",
  Douyin: "/brands/sources/tiktok.svg",
  Baidu: "/brands/sources/baidu.svg",
  "Sogou Browser": "/brands/sources/sogou.svg",
  Feishu: platform("lark-badge"),
  Alipay: platform("alipay"),
  LINE: platform("line"),
}

const OS_ICONS: Record<string, string> = {
  Windows: platform("windows"),
  "Windows 11": platform("windows11"),
  Windows11: platform("windows11"),
  Windows10: platform("windows"),
  "Windows Phone": platform("windows"),
  Android: platform("android"),
  "GNU/Linux": platform("linux"),
  Linux: platform("linux"),
  Ubuntu: platform("ubuntu"),
  Debian: platform("debian"),
  "Arch Linux": platform("arch-linux"),
  ArchLinux: platform("arch-linux"),
  Arch: platform("arch-linux"),
  Fedora: platform("fedora"),
  FreeBSD: platform("freebsd"),
  PlayStation: platform("playstation"),
  KaiOS: platform("kaios"),
  iOS: "/images/icon/os/ios.png",
  iPadOS: "/images/icon/os/ipad_os.png",
  "iPad OS": "/images/icon/os/ipad_os.png",
  macOS: "/images/icon/os/mac.png",
  Mac: "/images/icon/os/mac.png",
  "Mac OS X": "/images/icon/os/mac.png",
  "Mac OS": "/images/icon/os/mac.png",
  "Chrome OS": "/images/icon/os/chrome_os.png",
  ChromeOS: "/images/icon/os/chrome_os.png",
  "Fire OS": "/images/icon/os/fire_os.png",
  FireOS: "/images/icon/os/fire_os.png",
  HarmonyOS: "/images/icon/os/harmony_os.png",
  Tizen: "/images/icon/os/tizen.png",
}

function platformIconSrc(icons: Record<string, string>, name: string) {
  const normalized = name.trim().toLowerCase().replace(/\s+/g, " ")
  // Longest alias first keeps e.g. Windows 11 and Chrome Mobile distinct from
  // the generic family. Require a version boundary, not an arbitrary prefix.
  const alias = Object.keys(icons).sort((a, b) => b.length - a.length).find((key) => {
    const lower = key.toLowerCase()
    if (normalized === lower) return true
    const suffix = normalized.slice(lower.length)
    return normalized.startsWith(lower) && /^(?:[ /]+v?\d|[ /]+(?:xp|vista|nt)(?:[ /]|$))/.test(suffix)
  })
  return alias ? icons[alias] : undefined
}

function countryFlagSrc(code: string, shape: "square" | "rect" = "square") {
  const raw = (code || "").trim().toLowerCase()
  const slug = !raw || raw === "zz" || raw === "a1" || raw === "(none)" || raw === "(NONE)"
    ? "xx"
    : /^[a-z]{2}(?:-[a-z0-9]+)?$/.test(raw)
      ? raw
      : "xx"
  // Rectangular flags share the map assets; retain the existing dedicated Taiwan path.
  if (shape === "rect" && slug !== "tw") return `/flags/${slug}.svg`
  return `/images/flags/${shape === "rect" ? "4x3" : "1x1"}/${slug}.svg`
}

export function CountryFlag({ code, className = "", shape = "square" }: { code: string, className?: string, shape?: "square" | "rect" }) {
  const src = countryFlagSrc(code, shape)
  return (
    <img
      alt=""
      src={src}
      className={`country-flag ${className.includes("country-flag--") ? "" : shape === "rect" ? "country-flag--rect" : "country-flag--square"} ${className}`.trim()}
      onError={(e) => {
        const img = e.currentTarget
        if (shape === "rect" && !img.src.includes("/1x1/") && !img.src.endsWith("/xx.svg")) {
          img.src = countryFlagSrc(code, "square")
          return
        }
        if (!img.src.endsWith("/xx.svg")) img.src = countryFlagSrc("xx", shape)
        else img.style.visibility = "hidden"
      }}
    />
  )
}

const rowIconClassName = "mr-2 h-4 w-4 shrink-0"

// These original marks are too dark against a dark report row. A neutral
// surface preserves their official colors and their 16px footprint.
const neutralPlatformArtwork = new Set([
  "/images/icon/os/mac.png",
  platform("curl"),
  platform("kaios"),
  platform("debian"),
  platform("freebsd"),
])
const platformIconClass = (src: string) => neutralPlatformArtwork.has(src) ? "platform-icon-neutral" : ""

function AssetIcon({ src, fallback, className = "" }: { src: string, fallback: string, className?: string }) {
  return <ImageWithFallback src={src} className={`${rowIconClassName} object-contain ${className}`.trim()} fallback={<UIIcon name={fallback} className={rowIconClassName}/>}/>
}

function BrowserIcon({ name }: { name: string }) {
  const src = platformIconSrc(BROWSER_ICONS, browserDisplayName(name))
  if (src) return <AssetIcon src={src} fallback="browser" className={platformIconClass(src)} />
  return <UIIcon name="browser" className={rowIconClassName} />
}

function OsIcon({ name }: { name: string }) {
  const src = platformIconSrc(OS_ICONS, name)
  if (src) return <AssetIcon src={src} fallback="unknown" className={platformIconClass(src)} />
  return <UIIcon name="unknown" className={rowIconClassName} />
}

const DEVICE_ICONS: Record<string, string> = {
  Mobile: "device-mobile",
  Tablet: "device-tablet",
  Laptop: "device-laptop",
  Desktop: "device-desktop",
  "Ultra-wide": "device-ultrawide",
}

function DeviceIcon({ name }: { name: string }) {
  return <UIIcon name={DEVICE_ICONS[name] || "unknown"} className="-mt-px h-5 w-5" />
}

function SourceIcon({ name }: { name: string }) {
  const key = (name || "").trim()
  if (isDirectSource(key)) {
    return <UIIcon name="direct" className={rowIconClassName} />
  }
  if (campaignTag(key) !== null) return <UIIcon name="link" className={rowIconClassName} />
  const src = sourceIconSrc(key)
  if (src) return <AssetIcon src={src} fallback="link" className="source-brand-icon" />
  return <AssetIcon src={`/favicon/sources/${encodeURIComponent(key)}`} fallback="link" />
}

/** The traffic channels (report-values CHANNEL_NAMES): a symbol for what kind of visit each one is. */
const CHANNEL_ICONS: Record<string, string> = {
  direct: "direct", organic_search: "search", organic_social: "share", paid_search: "bullhorn", paid_social: "bullhorn",
  email: "mail", display: "ad", affiliate: "handshake", ai: "sparkles", referral: "link", other: "unknown",
}

function ChannelIcon({ name }: { name: string }) {
  return <UIIcon name={CHANNEL_ICONS[name] || "link"} className={rowIconClassName} />
}

export type RowIconKind = "source" | "ai" | "channel" | "country" | "browser" | "os" | "device"

export function RowIcon({ kind, name }: { kind?: RowIconKind, name: string }) {
  if (!kind) return null
  if (kind === "source" || kind === "ai") return <SourceIcon name={name} />
  if (kind === "channel") return <ChannelIcon name={name} />
  if (kind === "country") return <CountryFlag code={name} />
  if (kind === "browser") return <BrowserIcon name={name} />
  if (kind === "os") return <OsIcon name={name} />
  if (kind === "device") return <span className="mr-1.5"><DeviceIcon name={name} /></span>
  return null
}
