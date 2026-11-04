// Brand names cover UTM/legacy labels; domains cover referrers stored by the tracker.
// Keep the original source string for filtering and use this registry only for artwork.
type SourceBrand = [slug: string, names: string[], domains: string[]]

const brands: SourceBrand[] = [
  ["google", ["Google", "Google Images", "Google News", "Google Video", "Google Blogsearch", "Google Product Search", "谷歌"], ["google.com", "google.co.uk", "google.co.jp", "google.com.hk", "google.com.tw", "google.cn", "google.de", "google.fr", "google.ca", "google.com.au", "google.co.in", "google.ac", "google.es", "google.it", "google.co.kr", "google.com.br", "google.ru", "google.nl", "google.com.sg"]],
  ["google-ads", ["Google Ads", "Google AdWords", "AdWords"], ["googleadservices.com"]],
  ["bing", ["Bing", "Bing Images", "Bing Ads", "Microsoft Advertising", "必应"], ["bing.com"]],
  ["baidu", ["Baidu", "百度"], ["baidu.com"]],
  ["yahoo-badge", ["Yahoo", "Yahoo!", "Yahoo! Images", "Yahoo! Mail"], ["yahoo.com", "yahoo.co.jp", "yahoo.co.uk", "yahoo.cn", "yahoo.net"]],
  ["yandex", ["Yandex", "Yandex Images", "Yandex Maps", "Yandex.Market", "Yandex.Direct"], ["yandex.com", "yandex.ru", "yandex.kz", "ya.ru"]],
  ["duckduckgo", ["DuckDuckGo", "Duck Duck Go"], ["duckduckgo.com", "duck.com"]],
  ["brave-browser", ["Brave", "Brave Search"], ["search.brave.com"]],
  ["ecosia", ["Ecosia"], ["ecosia.org"]],
  ["naver", ["Naver", "Naver Images", "Naver Mail"], ["naver.com"]],
  ["sogou", ["Sogou", "搜狗"], ["sogou.com", "sougou.com"]],
  ["qwant", ["Qwant"], ["qwant.com"]],
  ["kagi", ["Kagi"], ["kagi.com"]],
  ["startpage", ["Startpage", "Start Page"], ["startpage.com"]],
  ["x-formerly-twitter", ["X", "Twitter", "X (Twitter)", "X / Twitter"], ["x.com", "twitter.com", "t.co"]],
  ["twitter", ["Twitter (legacy)"], []],
  ["sina-weibo", ["Weibo", "Sina Weibo", "微博", "新浪微博"], ["weibo.com", "weibo.cn", "t.cn"]],
  ["wechat", ["WeChat", "Weixin", "微信"], ["weixin.qq.com", "weixin.com"]],
  ["facebook", ["Facebook", "FB"], ["facebook.com", "fb.com", "fb.me"]],
  ["instagram", ["Instagram"], ["instagram.com"]],
  ["threads", ["Threads"], ["threads.com", "threads.net"]],
  ["linkedin", ["LinkedIn"], ["linkedin.com", "lnkd.in"]],
  ["reddit", ["Reddit"], ["reddit.com", "redd.it"]],
  ["youtube", ["YouTube"], ["youtube.com", "youtu.be"]],
  ["tiktok", ["TikTok"], ["tiktok.com"]],
  ["pinterest", ["Pinterest"], ["pinterest.com", "pinterest.ca", "pinterest.co.uk", "pinterest.fr", "pinterest.de", "pinterest.jp", "pin.it"]],
  ["telegram", ["Telegram"], ["telegram.org", "telegram.me", "t.me"]],
  ["discord", ["Discord"], ["discord.com", "discord.gg", "discordapp.com"]],
  ["github", ["GitHub"], ["github.com"]],
  ["gitlab", ["GitLab"], ["gitlab.com"]],
  ["y-combinator", ["Hacker News", "HackerNews", "HN"], ["news.ycombinator.com"]],
  ["stack-overflow", ["StackOverflow", "Stack Overflow"], ["stackoverflow.com"]],
  ["v2ex", ["V2EX"], ["v2ex.com"]],
  ["zhihu", ["Zhihu", "知乎"], ["zhihu.com"]],
  ["bilibili", ["Bilibili", "哔哩哔哩", "B站"], ["bilibili.com", "b23.tv"]],
  ["douban", ["Douban", "豆瓣"], ["douban.com"]],
  ["xiaohongshu", ["Xiaohongshu", "小红书", "RedNote"], ["xiaohongshu.com", "xhslink.com"]],
  ["juejin", ["Juejin", "掘金", "稀土掘金"], ["juejin.cn", "juejin.im"]],
  ["csdn", ["CSDN"], ["csdn.net"]],
  ["qq", ["QQ", "Qzone", "QQ空间"], ["qq.com"]],
  ["wikipedia", ["Wikipedia", "维基百科"], ["wikipedia.org"]],
  ["medium", ["Medium"], ["medium.com"]],
  ["devdotto", ["DEV", "DEV.to", "Dev Community"], ["dev.to"]],
  ["product-hunt", ["Product Hunt", "ProductHunt"], ["producthunt.com"]],
  ["quora", ["Quora"], ["quora.com"]],
  ["mastodon", ["Mastodon"], ["mastodon.social", "mastodon.online"]],
  ["bluesky", ["Bluesky", "Blue Sky"], ["bsky.app", "bsky.social"]],
  ["twitch", ["Twitch"], ["twitch.tv"]],
  ["snapchat", ["Snapchat"], ["snapchat.com"]],
  ["vimeo", ["Vimeo"], ["vimeo.com"]],
  ["whatsapp", ["WhatsApp"], ["whatsapp.com", "wa.me"]],
  ["slack", ["Slack"], ["slack.com"]],
  ["tumblr", ["Tumblr"], ["tumblr.com"]],
  ["substack", ["Substack"], ["substack.com"]],
  ["openai-chatgpt", ["ChatGPT", "Chat GPT"], ["chatgpt.com", "chat.openai.com"]],
  ["openai", ["OpenAI", "Open AI"], ["openai.com"]],
  ["claude-ai", ["Claude", "Claude AI", "Anthropic"], ["claude.ai", "claude.com", "anthropic.com"]],
  ["perplexity-ai", ["Perplexity", "Perplexity AI"], ["perplexity.ai", "pplx.ai"]],
  ["gemini", ["Gemini", "Google Gemini", "Bard"], ["gemini.google.com", "bard.google.com"]],
  ["deepseek", ["DeepSeek", "Deep Seek", "深度求索"], ["deepseek.com"]],
  ["microsoft-copilot", ["Copilot", "Microsoft Copilot", "Bing Chat"], ["copilot.microsoft.com"]],
  ["github-copilot", ["GitHub Copilot"], ["copilot.github.com"]],
  ["kimi", ["Kimi", "Kimi AI"], ["kimi.com", "kimi.ai", "kimi.moonshot.cn"]],
  ["qwen", ["Qwen", "通义千问", "通义"], ["qwen.ai", "tongyi.aliyun.com", "qianwen.aliyun.com", "tongyi.com", "qianwen.com"]],
  ["doubao", ["Doubao", "豆包"], ["doubao.com"]],
  ["mistral-ai", ["Mistral", "Mistral AI", "Le Chat"], ["mistral.ai"]],
  ["grok", ["Grok", "xAI"], ["grok.com", "x.ai"]],
  ["metaai", ["Meta AI"], ["meta.ai"]],
  ["poe", ["Poe"], ["poe.com"]],
  ["phind", ["Phind"], ["phind.com"]],
  ["yuanbao", ["Yuanbao", "腾讯元宝", "元宝"], ["yuanbao.tencent.com"]],
  ["wenxin", ["Wenxin", "ERNIE Bot", "文心一言", "文心"], ["yiyan.baidu.com"]],
  ["zhipu", ["ChatGLM", "Zhipu", "智谱清言", "智谱"], ["chatglm.cn", "zhipuai.cn", "bigmodel.cn"]],
  ["notebooklm", ["NotebookLM"], ["notebooklm.google", "notebooklm.google.com"]],
]

/** Stored empty/legacy labels mean direct traffic; domains and campaign names do not. */
/** A source known only from a link's utm_source tag; the server stores it as "utm:<tag>". */
export function campaignTag(name: string): string | null {
  return name.startsWith("utm:") ? name.slice(4) : null
}

export function isDirectSource(name: string): boolean {
  const key = name.trim()
  return !key || /^(direct(\s*\/\s*none)?|\(none\))$/i.test(key)
}

const normalize = (value: string) => value.trim().toLowerCase().replace(/\s+/g, " ")
const names = new Map(brands.flatMap(([slug, aliases]) => aliases.map(name => [normalize(name), slug] as const)))
// Specific product hosts (Gemini, ChatGPT, Copilot) must win over their parent brand.
const domains = brands.flatMap(([slug, , hosts]) => hosts.map(host => [host, slug] as const)).sort((a, b) => b[0].length - a[0].length)
// These upstream defaults are white; retain the original black light-theme artwork.
const lightVariants = new Set(["x-formerly-twitter", "openai", "threads", "grok", "github-copilot", "qwen"])
const assetPath = (slug: string) => `/brands/sources/${slug}${lightVariants.has(slug) ? "-light" : ""}.svg`

export function sourceIconSrc(name: string): string | undefined {
  const key = name.trim()
  const named = names.get(normalize(key))
  if (named) return assetPath(named)
  try {
    if (!key || key.length > 2048 || /[\s\\]/.test(key)) return undefined
    const url = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(key) ? key : `https://${key}`)
    if (!["http:", "https:"].includes(url.protocol) || url.username || url.password) return undefined
    const host = url.hostname.toLowerCase().replace(/\.$/, "")
    const match = domains.find(([domain]) => host === domain || host.endsWith(`.${domain}`))
    return match ? assetPath(match[1]) : undefined
  } catch {
    return undefined
  }
}
