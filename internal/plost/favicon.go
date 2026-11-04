package plost

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/net/idna"
)

// The dashboard asks this server for every site and source icon. Public names are sent on to favicon.la by the
// browser; everything that is not a public domain is answered here with a local picture, so private, synthetic
// and reserved hostnames never leave the instance.

//go:embed referer-domains.json
var refererDomainsJSON []byte

// Labels the tracker and UTM links use that the bundled table does not know or maps to a less useful site.
var customSourceDomains = map[string]string{
	"Brave": "search.brave.com", "Kagi": "kagi.com", "Wikipedia": "en.wikipedia.org", "Discord": "discord.com",
	"Perplexity": "perplexity.ai", "LinkedIn": "linkedin.com", "Bluesky": "bsky.app", "ChatGPT": "chatgpt.com",
	"Claude": "claude.ai", "X (Twitter)": "x.com", "Twitter": "twitter.com", "Google": "google.com",
	"Facebook": "facebook.com", "DuckDuckGo": "duckduckgo.com",
}

var sourceDomains = sync.OnceValue(func() map[string]string {
	table := map[string]string{}
	_ = json.Unmarshal(refererDomainsJSON, &table)
	for name, domain := range customSourceDomains {
		table[name] = domain
	}
	return table
})

const (
	linkPlaceholder = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="#71717a"><path d="M12.232 4.232a2.5 2.5 0 0 1 3.536 3.536l-1.225 1.224a.75.75 0 0 0 1.061 1.06l1.224-1.224a4 4 0 0 0-5.656-5.656l-3 3a4 4 0 0 0 .225 5.865.75.75 0 0 0 .977-1.138 2.5 2.5 0 0 1-.142-3.667l3-3Z"/><path d="M11.603 7.963a.75.75 0 0 0-.977 1.138 2.5 2.5 0 0 1 .142 3.667l-3 3a2.5 2.5 0 0 1-3.536-3.536l1.225-1.224a.75.75 0 0 0-1.061-1.06l-1.224 1.224a4 4 0 1 0 5.656 5.656l3-3a4 4 0 0 0-.225-5.865Z"/></svg>`
	sitePlaceholder = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="none" stroke="#71717a" stroke-width="1.5"><circle cx="10" cy="10" r="7.5"/><ellipse cx="10" cy="10" rx="3.25" ry="7.5"/><path d="M2.5 10h15"/></svg>`
)

var (
	directSourceRE   = regexp.MustCompile(`(?i)^(direct(\s*/\s*none)?|\(none\))$`)
	explicitSchemeRE = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://`)
	faviconLabelRE   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	numericLastRE    = regexp.MustCompile(`^(\d+|0x[0-9a-f]*)$`)
	reservedSuffixRE = regexp.MustCompile(`\.(localhost|local|internal|invalid|test|example|lan|onion)$`)
	reservedExactRE  = regexp.MustCompile(`(^|\.)(example\.(com|net|org)|home\.arpa)$`)
)

// publicFaviconHost accepts a bare domain or a full http(s) URL and returns the lower-case ASCII host, or false
// when it is not a plain public domain name.
func publicFaviconHost(value string) (string, bool) {
	if len(value) > 2048 || strings.ContainsRune(value, '\\') || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", false
	}
	if !explicitSchemeRE.MatchString(value) {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return "", false
	}
	host := strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), "www.")
	for _, r := range host {
		if r > unicode.MaxASCII {
			ascii, err := idna.Lookup.ToASCII(host)
			if err != nil {
				return "", false
			}
			host = ascii
			break
		}
	}
	labels := strings.Split(host, ".")
	if len(host) > 253 || len(labels) < 2 {
		return "", false
	}
	for _, label := range labels {
		if !faviconLabelRE.MatchString(label) {
			return "", false
		}
	}
	// An address written as 2130706433, 0x7f000001 or 127.1 is still an address, not a name.
	if numericLastRE.MatchString(labels[len(labels)-1]) || reservedSuffixRE.MatchString(host) || reservedExactRE.MatchString(host) {
		return "", false
	}
	return host, true
}

func faviconPlaceholder(w http.ResponseWriter, svg string) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write([]byte(svg))
}

// faviconSource serves /favicon/sources/{name}, where name is a site domain, a referrer host or a source label.
// It is public on purpose: the shared report pages show the same icons to visitors who are not signed in.
func (a *App) faviconSource(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" || name == "placeholder" || directSourceRE.MatchString(name) {
		faviconPlaceholder(w, linkPlaceholder)
		return
	}
	if name == "site-placeholder" {
		faviconPlaceholder(w, sitePlaceholder)
		return
	}
	if a.serveSiteIcon(w, r, name) {
		return
	}
	source := name
	if domain, ok := sourceDomains()[name]; ok {
		source = domain
	}
	host, ok := publicFaviconHost(source)
	if !ok {
		faviconPlaceholder(w, sitePlaceholder)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Location", "https://favicon.la/"+url.PathEscape(host))
	w.WriteHeader(http.StatusFound)
}
