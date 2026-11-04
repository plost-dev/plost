package plost

import (
	"regexp"
	"strings"
)

// Channels group referrers by kind of visit. A referrer is the host (or, from apps, the app id) the tracker stored,
// so a domain matches itself and its subdomains: cn.bing.com and www4.bing.com are Bing, noai.duckduckgo.com is
// DuckDuckGo. AI assistants are checked before search engines, so gemini.google.com and yiyan.baidu.com are not
// counted as Google or Baidu searches.

var aiReferrers = []string{
	"chatgpt.com", "openai.com", "perplexity.ai", "pplx.ai", "claude.ai", "gemini.google.com", "bard.google.com",
	"notebooklm.google", "notebooklm.google.com", "copilot.microsoft.com", "copilot.com", "copilot.cloud.microsoft", "grok.com",
	"deepseek.com", "meta.ai", "kimi.com", "kimi.ai", "kimi.moonshot.cn", "doubao.com", "qwen.ai", "tongyi.aliyun.com",
	"qianwen.aliyun.com", "tongyi.com", "qianwen.com", "chat.mistral.ai", "poe.com", "phind.com", "you.com",
	"yuanbao.tencent.com", "yiyan.baidu.com", "chatglm.cn", "metaso.cn",
}

// Google, Yahoo and Yandex search under many country domains (google.com.hk, search.yahoo.co.jp, yandex.ru); these
// are GLOB patterns. Other Google hosts (accounts, mail, docs) are not searches and stay referrals.
var searchPatterns = []string{"google.*", "www.google.*", "search.yahoo.*", "*.search.yahoo.*", "yandex.*", "*.yandex.*"}

var searchReferrers = []string{
	"com.google.android.googlequicksearchbox", "bing.com", "baidu.com", "duckduckgo.com", "ya.ru", "search.brave.com",
	"ecosia.org", "sogou.com", "so.com", "sm.cn", "search.naver.com", "qwant.com", "kagi.com", "startpage.com",
	"seznam.cz", "search.daum.net",
}

var socialReferrers = []string{
	"twitter.com", "x.com", "t.co", "com.twitter.android", "facebook.com", "fb.com", "instagram.com", "reddit.com",
	"weibo.com", "weibo.cn", "t.cn", "youtube.com", "youtu.be", "bsky.app", "threads.net", "threads.com", "tiktok.com",
	"linkedin.com", "lnkd.in", "pinterest.com", "telegram.org", "t.me", "org.telegram.messenger",
	"org.telegram.messenger.web", "discord.com", "zhihu.com", "xiaohongshu.com", "bilibili.com", "douban.com",
	"vk.com", "quora.com", "mastodon.social", "tumblr.com", "weixin.qq.com", "news.ycombinator.com", "v2ex.com",
	"whatsapp.com", "line.me", "zalo",
}

// hostIn matches the referrer against domains and their subdomains.
func hostIn(domains []string) string {
	parts := make([]string, 0, len(domains)*2)
	for _, d := range domains {
		parts = append(parts, "referrer_source='"+d+"'", "referrer_source LIKE '%."+d+"'")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func hostGlob(patterns []string) string {
	parts := make([]string, 0, len(patterns))
	for _, p := range patterns {
		parts = append(parts, "referrer_source GLOB '"+p+"'")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// channelSQL names the channel of an event's referrer: direct, ai, organic_search, organic_social or referral.
var channelSQL = "CASE WHEN referrer_source IN ('','Direct') THEN 'direct' WHEN " + hostIn(aiReferrers) + " THEN 'ai' WHEN " +
	hostGlob(searchPatterns) + " OR " + hostIn(searchReferrers) + " THEN 'organic_search' WHEN " + hostIn(socialReferrers) +
	" THEN 'organic_social' ELSE 'referral' END"

// campaignPrefix marks a source that is only known from a link's utm_source tag.
const campaignPrefix = "utm:"

// taggedSites maps utm_source tags that name a well-known site without its domain.
var taggedSites = map[string]string{
	"chatgpt": "chatgpt.com", "openai": "openai.com", "perplexity": "perplexity.ai", "gemini": "gemini.google.com",
	"copilot": "copilot.microsoft.com", "claude": "claude.ai", "deepseek": "deepseek.com", "grok": "grok.com", "zalo": "zalo",
}

var hostTag = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)+$`)

// sourceName picks what the source report shows. A referring website always wins and
// is stored as its domain. Without one, a utm_source tag that is itself a domain (ChatGPT
// adds utm_source=chatgpt.com) or names a well-known site counts as that site; any other
// tag is stored as "utm:<tag>" so the interface can say it is a campaign link, not a site.
func sourceName(site, utmSource string) string {
	if site != "" && site != "Direct" {
		return site
	}
	tag := strings.TrimSpace(utmSource)
	if tag == "" {
		return "Direct"
	}
	lower := strings.ToLower(tag)
	if known, ok := taggedSites[lower]; ok {
		return known
	}
	if len(lower) <= 253 && hostTag.MatchString(lower) {
		return canonicalHost(lower)
	}
	return campaignPrefix + short(tag, 200)
}
