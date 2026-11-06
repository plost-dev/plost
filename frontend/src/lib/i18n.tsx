import UI_EN from "./ui-en.json"
import { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from "react"

/** Interface languages, in the order the language menu shows them (two columns, row by row). */
export const LOCALES = [
  { code: "zh-CN", name: "简体中文", flag: "cn" },
  { code: "zh-TW", name: "繁體中文", flag: "tw" },
  { code: "en", name: "English", flag: "gb" },
  { code: "ru", name: "Русский", flag: "ru" },
  { code: "vi", name: "Tiếng Việt", flag: "vn" },
  { code: "id", name: "Indonesia", flag: "id" },
  { code: "pt-BR", name: "Português", flag: "br" },
  { code: "fa", name: "فارسی", flag: "ir" },
  { code: "ar", name: "العربية", flag: "sa" },
  { code: "ja", name: "日本語", flag: "jp" },
  { code: "ko", name: "한국어", flag: "kr" },
  { code: "th", name: "ไทย", flag: "th" },
  { code: "es", name: "Español", flag: "es" },
  { code: "fr", name: "Français", flag: "fr" },
  { code: "de", name: "Deutsch", flag: "de" },
  { code: "tr", name: "Türkçe", flag: "tr" },
  { code: "ur", name: "اردو", flag: "pk" },
  { code: "it", name: "Italiano", flag: "it" },
  { code: "pl", name: "Polski", flag: "pl" },
  { code: "nl", name: "Nederlands", flag: "nl" },
] as const

export type Locale = typeof LOCALES[number]["code"]
const LOCALE_CODES: ReadonlySet<string> = new Set(LOCALES.map(item => item.code))
const RTL_LOCALES: ReadonlySet<string> = new Set(["ar", "fa", "ur"])

export function isRtl(locale: string) {
  return RTL_LOCALES.has(locale)
}

/** Chinese locales keep the Chinese date style (11月4日) and full-width punctuation. */
export function isChinese(locale: string) {
  return locale === "zh-CN" || locale === "zh-TW"
}

/** The BCP 47 tag handed to Intl formatters; English keeps day-month order. Digits stay Western (0–9) in every
 *  language, including Arabic, Persian and Urdu, so dates match the counters and charts next to them. */
export function intlLocale(locale: string) {
  return locale === "en" ? "en-GB" : `${locale}-u-nu-latn`
}

const ZH: Record<string, string> = {
  "nav.sites": "站点",
  "nav.add_site": "添加网站",
  "nav.account": "账户设置",
  "nav.backup": "备份与恢复",
  "nav.team_create": "创建团队",
  "nav.team_settings": "团队设置",
  "nav.login": "登录",
  "nav.register": "注册",
  "nav.logout": "退出登录",
  "nav.signed_in": "当前登录",
  "settings.back_sites": "返回站点",
  "settings.back_stats": "返回统计",
  "settings.title": "设置",
  "settings.site_for": "站点设置",
  "settings.account": "账户",
  "settings.team": "团队",
  "settings.preferences": "偏好设置",
  "settings.api_keys": "API 密钥",
  "settings.backup": "备份",
  "settings.danger": "危险操作",
  "settings.general": "常规",
  "profile.avatar": "头像",
  "profile.avatar_sub": "显示在页头和成员列表里。",
  "profile.change_avatar": "更换头像",
  "profile.remove_avatar": "移除",
  "profile.nickname": "昵称",
  "profile.nickname_sub": "用于页头和团队成员显示。",
  "profile.nickname_label": "昵称",
  "profile.save_nickname": "保存昵称",
  "profile.save_avatar": "保存头像",
  "profile.language": "语言",
  "profile.language_sub": "切换界面语言。",
  "profile.language_label": "界面语言",
  "profile.save_language": "保存语言",
  "profile.appearance": "外观",
  "profile.theme": "主题",
  "profile.theme_system": "跟随系统",
  "profile.theme_light": "浅色",
  "profile.theme_dark": "深色",
  "profile.save_theme": "更改主题",
  "security.email": "邮箱地址",
  "security.email_sub": "更改与账号关联的邮箱。",
  "security.current_email": "当前邮箱",
  "security.new_email": "新邮箱",
  "security.confirm_password": "输入密码确认",
  "security.change_email": "更改邮箱",
  "security.password": "密码",
  "security.password_sub": "更改你的密码。",
  "security.old_password": "旧密码",
  "security.new_password": "新密码",
  "security.confirm_new_password": "确认新密码",
  "security.verify_2fa": "两步验证码",
  "security.change_password": "更改密码",
  "security.2fa": "两步验证 (2FA)",
  "security.2fa_sub": "登录时增加一步验证器验证，保护你的账号。",
  "security.enable_2fa": "启用两步验证",
  "security.disable_2fa": "停用两步验证",
  "security.2fa_scan": "用验证器扫描二维码，或手动输入密钥。",
  "security.2fa_secret": "密钥",
  "security.2fa_code": "验证码",
  "security.2fa_confirm": "验证并启用",
  "security.2fa_codes": "请立刻保存这些恢复码",
  "security.2fa_codes_sub": "丢失验证器时可用。每条只能用一次。",
  "security.2fa_done": "我已保存",
  "security.lost_recovery": "丢失了恢复码？",
  "security.generate_new": "生成新的恢复码",
  "security.disable_confirm": "输入密码和验证码以停用",
  "security.passkey": "Passkey",
  "security.passkey_sub": "用本机指纹、面容或安全密钥登录，无需输入密码。",
  "security.add_passkey": "添加 Passkey",
  "security.remove": "移除",
  "security.no_passkey": "还没有 Passkey。添加后可在登录页一键验证。",
  "backup.title": "备份与恢复",
  "backup.storage": "对象存储",
  "backup.storage_sub": "只支持 Cloudflare R2 和 Amazon S3。新装系统填同一组地址后可以扫描并一键恢复。",
  "backup.provider": "服务",
  "backup.prefix": "前缀",
  "backup.schedule": "备份时间",
  "backup.schedule_sub": "会备份 PostgreSQL 站点/账号数据和 ClickHouse 访问事件。不是逐条实时复制，最短可按小时跑。",
  "backup.enabled": "启用定时备份",
  "backup.frequency": "频率",
  "backup.off": "仅手动",
  "backup.hourly": "每小时",
  "backup.daily": "每天",
  "backup.weekly": "每周",
  "backup.weekday": "星期",
  "backup.time": "时间",
  "backup.save": "保存设置",
  "backup.run": "立即备份",
  "backup.scan_title": "扫描与恢复",
  "backup.scan_sub": "新机器填好同一组 R2/S3 地址后点扫描，选一份备份即可恢复。恢复会覆盖当前数据库。",
  "backup.scan": "扫描备份",
  "backup.restore": "一键恢复",
  "site.new_title": "添加网站",
  "site.new_sub": "开始统计一个新站点的访问。",
  "site.domain": "域名",
  "site.domain_hint": "只要裸域名或子域名，不要带 www、https。",
  "site.timezone": "时区",
  "site.next": "下一步",
  "site.prev": "上一步",
  "site.snippet": "把这段代码放到网站 </head> 前。",
  "site.check": "我已安装，检查流量",
  "site.skip": "稍后安装",
  "site.waiting": "还没收到访问，确认脚本已保存并刷新一下站点。",
  "site.detected": "已经收到访问。",
  "site.done_go": "打开统计",
  "site.step_site": "站点信息",
  "site.step_install": "安装代码",
  "site.step_done": "完成",
  "login.title": "登录 Plost",
  "login.welcome": "欢迎回来",
  "login.subtitle": "登录你的 Plost 统计后台",
  "login.or": "或",
  "login.email": "邮箱",
  "login.password": "密码",
  "login.submit": "登录",
  "login.pending": "登录中",
  "login.verifying": "验证中",
  "login.passkey": "使用 Passkey 登录",
  "login.2fa_title": "两步验证",
  "login.2fa_sub": "输入验证器中的 6 位数字，或一条恢复码。",
  "login.2fa_submit": "验证",
  "login.back": "返回登录",
  "login.no_account": "还没有账号？",
  "login.create": "创建账号",
  "footer.tagline": "自托管网站统计。流量与搜索词——数据始终属于你。",
  "footer.own": "看清每一次访问",
  "footer.sites": "{n} 个站点",
  "footer.today_visitors": "今日 {n} 访客",
  "footer.today_visits": "{n} 访问",
  "footer.today_views": "{n} 浏览",
}

const EN: Record<string, string> = {
  "nav.sites": "Sites",
  "nav.add_site": "Add website",
  "nav.account": "Account settings",
  "nav.backup": "Backup & restore",
  "nav.team_create": "Create a team",
  "nav.team_settings": "Team settings",
  "nav.login": "Log in",
  "nav.register": "Sign up",
  "nav.logout": "Log out",
  "nav.signed_in": "Signed in as",
  "settings.back_sites": "Back to sites",
  "settings.back_stats": "Back to stats",
  "settings.title": "Settings",
  "settings.site_for": "Site settings",
  "settings.account": "Account",
  "settings.team": "Team",
  "settings.preferences": "Preferences",
  "settings.api_keys": "API keys",
  "settings.backup": "Backup",
  "settings.danger": "Danger zone",
  "settings.general": "General",
  "profile.avatar": "Avatar",
  "profile.avatar_sub": "Shown in the header and team member list.",
  "profile.change_avatar": "Change avatar",
  "profile.remove_avatar": "Remove",
  "profile.nickname": "Display name",
  "profile.nickname_sub": "Used in the header and team member list.",
  "profile.nickname_label": "Display name",
  "profile.save_nickname": "Save name",
  "profile.save_avatar": "Save avatar",
  "profile.language": "Language",
  "profile.language_sub": "Switch the interface language.",
  "profile.language_label": "Interface language",
  "profile.save_language": "Save language",
  "profile.appearance": "Appearance",
  "profile.theme": "Theme",
  "profile.theme_system": "System",
  "profile.theme_light": "Light",
  "profile.theme_dark": "Dark",
  "profile.save_theme": "Change theme",
  "security.email": "Email address",
  "security.email_sub": "Change the email associated with this account.",
  "security.current_email": "Current email",
  "security.new_email": "New email",
  "security.confirm_password": "Enter password to confirm",
  "security.change_email": "Change email",
  "security.password": "Password",
  "security.password_sub": "Change your password.",
  "security.old_password": "Old password",
  "security.new_password": "New password",
  "security.confirm_new_password": "Confirm new password",
  "security.verify_2fa": "Two-factor code",
  "security.change_password": "Change password",
  "security.2fa": "Two-factor authentication (2FA)",
  "security.2fa_sub": "Add an authenticator step when you sign in.",
  "security.enable_2fa": "Enable 2FA",
  "security.disable_2fa": "Disable 2FA",
  "security.2fa_scan": "Scan the QR code with your authenticator, or enter the secret manually.",
  "security.2fa_secret": "Secret",
  "security.2fa_code": "Code",
  "security.2fa_confirm": "Verify and enable",
  "security.2fa_codes": "Save these recovery codes now",
  "security.2fa_codes_sub": "Use them if you lose your authenticator. Each code works once.",
  "security.2fa_done": "I saved them",
  "security.lost_recovery": "Lost your recovery codes?",
  "security.generate_new": "Generate new codes",
  "security.disable_confirm": "Enter your password and a code to disable",
  "security.passkey": "Passkeys",
  "security.passkey_sub": "Sign in with your device fingerprint, face, or security key.",
  "security.add_passkey": "Add a passkey",
  "security.remove": "Remove",
  "security.no_passkey": "No passkeys yet. Add one to sign in without a password.",
  "backup.title": "Backup & restore",
  "backup.storage": "Object storage",
  "backup.storage_sub": "Cloudflare R2 and Amazon S3 only. On a new install, enter the same details to scan and restore.",
  "backup.provider": "Provider",
  "backup.prefix": "Prefix",
  "backup.schedule": "Schedule",
  "backup.schedule_sub": "Backs up PostgreSQL account/site data and ClickHouse events. Hourly is the shortest interval.",
  "backup.enabled": "Enable scheduled backups",
  "backup.frequency": "Frequency",
  "backup.off": "Manual only",
  "backup.hourly": "Hourly",
  "backup.daily": "Daily",
  "backup.weekly": "Weekly",
  "backup.weekday": "Weekday",
  "backup.time": "Time",
  "backup.save": "Save settings",
  "backup.run": "Back up now",
  "backup.scan_title": "Scan & restore",
  "backup.scan_sub": "On a new machine, enter the same R2/S3 details, scan, then restore. Restore overwrites the current database.",
  "backup.scan": "Scan backups",
  "backup.restore": "Restore",
  "site.new_title": "Add a website",
  "site.new_sub": "Start measuring a new site.",
  "site.domain": "Domain",
  "site.domain_hint": "Bare domain or subdomain only. No www or https.",
  "site.timezone": "Timezone",
  "site.next": "Next",
  "site.prev": "Back",
  "site.snippet": "Put this snippet just before </head>.",
  "site.check": "I installed it, check traffic",
  "site.skip": "Install later",
  "site.waiting": "No visits yet. Confirm the script is saved and reload the site.",
  "site.detected": "Traffic received.",
  "site.done_go": "Open stats",
  "site.step_site": "Site details",
  "site.step_install": "Install snippet",
  "site.step_done": "Done",
  "login.title": "Log in to Plost",
  "login.welcome": "Welcome back",
  "login.subtitle": "Sign in to your Plost analytics",
  "login.or": "or",
  "login.email": "Email",
  "login.password": "Password",
  "login.submit": "Log in",
  "login.pending": "Logging in",
  "login.verifying": "Verifying",
  "login.passkey": "Sign in with a passkey",
  "login.2fa_title": "Two-factor authentication",
  "login.2fa_sub": "Enter the 6-digit authenticator code or a recovery code.",
  "login.2fa_submit": "Verify",
  "login.back": "Back to sign in",
  "login.no_account": "Don't have an account?",
  "login.create": "Create an account",
  "footer.tagline": "Self-hosted analytics. Traffic and search terms — the data stays yours.",
  "footer.own": "Understand every visit",
  "footer.sites": "{n} sites",
  "footer.today_visitors": "{n} visitors today",
  "footer.today_visits": "{n} visits",
  "footer.today_views": "{n} views",
}

type Params = Record<string, string | number>
export type Translator = (key: string, params?: Params) => string
const ENGLISH: Record<string, string> = { ...Object.fromEntries(Object.entries(ZH).map(([key, value]) => [value, EN[key] || value])), ...UI_EN }

// Every other language is a JSON catalog keyed by the Simplified Chinese source text,
// split into its own chunk and loaded before the first render in that language.
const catalogLoaders = import.meta.glob<{ default: Record<string, string> }>("../locales/*.json")
const catalogs: Partial<Record<Locale, Record<string, string>>> = {}

// Place and browser names arrive in simplified Chinese; zh-TW converts them once its converter has loaded.
let traditional: ((text: string) => string) | undefined

export function toTraditional(text: string) {
  return traditional ? traditional(text) : text
}

export function toTraditionalFor(locale: string, text: string) {
  return locale === "zh-TW" ? toTraditional(text) : text
}

export async function loadLocale(locale: Locale): Promise<void> {
  if (locale === "zh-TW" && !traditional) {
    const { Converter } = await import("opencc-js/cn2t")
    traditional = Converter({ from: "cn", to: "twp" })
  }
  if (locale === "zh-CN" || locale === "en" || catalogs[locale]) return
  const load = catalogLoaders[`../locales/${locale}.json`]
  if (load) catalogs[locale] = (await load()).default
}

const own = (map: Record<string, string> | undefined, key: string) => map && Object.hasOwn(map, key) ? map[key] : undefined

export function translate(locale: Locale, key: string, params?: Params) {
  const source = own(ZH, key) ?? key
  let value: string
  if (locale === "zh-CN") value = source
  else if (locale === "en") value = own(EN, key) ?? own(ENGLISH, source) ?? source
  // A missing string falls back to English, except Traditional Chinese which reads the Chinese source better.
  // A catalog may also hold a keyed entry (e.g. "nav.sites", plural) where one Chinese word needs two translations.
  else value = own(catalogs[locale], key) ?? own(catalogs[locale], source) ?? (locale === "zh-TW" ? source : own(EN, key) ?? own(ENGLISH, source) ?? source)
  if (params) value = value.replace(/\{(\w+)\}/g, (match, name) => Object.hasOwn(params, name) ? String(params[name]) : match)
  return value
}

const I18nContext = createContext<{ locale: Locale, setLocale: (locale: Locale) => Promise<void>, t: Translator }>({
  locale: "zh-CN", setLocale: async () => {}, t: (key, params) => translate("zh-CN", key, params),
})

export function I18nProvider({ locale: initialLocale, children }: { locale: Locale, children: ReactNode }) {
  const [locale, setShown] = useState(initialLocale)
  const setLocale = useCallback(async (next: Locale) => { await loadLocale(next); setShown(next) }, [])
  useEffect(() => { void setLocale(initialLocale) }, [initialLocale, setLocale])
  useEffect(() => {
    document.documentElement.lang = locale
    document.documentElement.dir = isRtl(locale) ? "rtl" : "ltr"
  }, [locale])
  const t = useCallback<Translator>((key, params) => translate(locale, key, params), [locale])
  return <I18nContext.Provider value={{ locale, setLocale, t }}>{children}</I18nContext.Provider>
}

export function useT() {
  return useContext(I18nContext)
}

/** The first browser language Plost speaks, for visitors who have not chosen one. */
export function browserLocale(languages: readonly string[] = typeof navigator === "undefined" ? [] : navigator.languages): Locale {
  for (const tag of languages) {
    const lower = tag.toLowerCase()
    if (lower.startsWith("zh")) return /^zh-(tw|hk|mo|hant)/.test(lower) ? "zh-TW" : "zh-CN"
    if (lower.startsWith("pt")) return "pt-BR"
    const base = lower.split("-")[0]
    const match = LOCALES.find(item => item.code === base)
    if (match) return match.code
  }
  return "zh-CN"
}

export function isLocale(value: unknown): value is Locale {
  return typeof value === "string" && LOCALE_CODES.has(value)
}
