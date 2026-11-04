package plost

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// Referrers as production stores them: subdomains, country domains and app ids fall in their channel, and AI
// assistants on a search engine's domain are AI, not search.
func TestChannelOfReferrer(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for referrer, want := range map[string]string{
		"": "direct", "Direct": "direct",
		"cn.bing.com": "organic_search", "www4.bing.com": "organic_search", "bing.com": "organic_search",
		"google.com": "organic_search", "google.com.hk": "organic_search", "com.google.android.googlequicksearchbox": "organic_search",
		"noai.duckduckgo.com": "organic_search", "yandex.ru": "organic_search", "ya.ru": "organic_search", "r.search.yahoo.com": "organic_search",
		"search.yahoo.co.jp": "organic_search", "m.baidu.com": "organic_search", "kagi.com": "organic_search",
		"chatgpt.com": "ai", "chat.openai.com": "ai", "gemini.google.com": "ai", "l.meta.ai": "ai", "doubao.com": "ai",
		"yiyan.baidu.com": "ai", "perplexity.ai": "ai", "claude.ai": "ai",
		"t.co": "organic_social", "com.twitter.android": "organic_social", "l.threads.com": "organic_social",
		"web.telegram.org": "organic_social", "org.telegram.messenger.web": "organic_social",
		"accounts.google.com": "referral", "github.com": "referral", "ntp.msn.cn": "referral", "notgoogle.com": "referral",
		"bing.com.evil.example": "referral",
	} {
		var got string
		if err := db.QueryRow("SELECT "+channelSQL+" FROM (SELECT ? AS referrer_source)", referrer).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%q: %s, want %s", referrer, got, want)
		}
	}
}

func TestSourceNamePrefersReferringSite(t *testing.T) {
	for _, c := range []struct{ site, utm, want string }{
		{"bimos.ylink.uk", "voguesly", "bimos.ylink.uk"},
		{"Direct", "voguesly", "utm:voguesly"},
		{"", " voguesly ", "utm:voguesly"},
		{"Direct", "", "Direct"},
		{"google.com", "", "google.com"},
		{"Direct", "chatgpt.com", "chatgpt.com"},
		{"Direct", "Perplexity", "perplexity.ai"},
		{"Direct", "www.Example.org", "example.org"},
		{"Direct", "spotlight_house", "utm:spotlight_house"},
	} {
		if got := sourceName(c.site, c.utm); got != c.want {
			t.Errorf("sourceName(%q, %q) = %q, want %q", c.site, c.utm, got, c.want)
		}
	}
}
