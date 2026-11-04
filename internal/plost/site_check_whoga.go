package plost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// who.ga (sponsor of Plost's domain lookups) answers for suffixes whose registry offers no RDAP service, such
// as .st, and reaches CNNIC's WHOIS when TCP 43 is blocked here. It is asked only after the registry's own
// sources have no date, so the registry stays the authority and most domains never leave for a third party.
// Only the domain name is sent, and only the expiration, registration date and registrar are kept.
const whogaBase = "https://api.who.ga"

var whoisDateOnly = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// whogaBrief is GET /<domain>: dates already read from RDAP or WHOIS, and no contact data.
type whogaBrief struct {
	Domain       string  `json:"domain"`
	ExpiresAt    *string `json:"expiresAt"`
	RegisteredAt *string `json:"registeredAt"`
	Registrar    *string `json:"registrar"`
	ErrorCode    string  `json:"errorCode"`
}

// whogaFull is GET /full/<domain>: the raw record. Its parsed WHOIS fields include contacts, which are never kept.
type whogaFull struct {
	Domain    string `json:"domain"`
	ErrorCode string `json:"errorCode"`
	Result    *struct {
		rdapDomain
		Parsed map[string]any `json:"parsed"`
	} `json:"result"`
}

// whoisDate reads a WHOIS timestamp: RFC 3339 keeps its instant; a plain date or a time without a zone keeps only the date.
func whoisDate(value string) string {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return isoTime(t)
	}
	if whoisDateOnly.MatchString(value) {
		if t, err := time.Parse("2006-01-02", value); err == nil && t.Format("2006-01-02") == value {
			return value
		}
		return ""
	}
	return cnExpirationDate(value)
}

// parsedField returns the first listed WHOIS field that is present exactly once as text.
func parsedField(parsed map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := parsed[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func whogaStatusFailure(status int, code string) DomainExpiry {
	switch {
	case code == "DOMAIN_NOT_FOUND":
		return DomainExpiry{Error: "who.ga 未找到该域名的注册记录，请到注册商查看", Retryable: boolPtr(false)}
	case status == http.StatusTooManyRequests:
		return DomainExpiry{Error: "who.ga 查询受限，请稍后重试或到注册商查看"}
	}
	return DomainExpiry{Error: "who.ga 查询暂时不可用，请稍后重试或到注册商查看"}
}

// whoga asks the brief endpoint first and the full record only when the brief one has no expiration date.
func (l *domainLookup) whoga(ctx context.Context, domain string) DomainExpiry {
	base, err := url.Parse(firstNonEmpty(l.whogaBase, whogaBase))
	if err != nil {
		return DomainExpiry{Domain: domain, Error: "who.ga 查询暂时不可用，请稍后重试或到注册商查看"}
	}
	ask := func(path string) (int, []byte, *DomainExpiry) {
		status, body, err := l.fetch(ctx, base.ResolveReference(&url.URL{Path: path}))
		if err != nil {
			fail := DomainExpiry{Domain: domain, Error: "who.ga " + checkErrorMessage(err)}
			return 0, nil, &fail
		}
		return status, body, nil
	}
	matches := func(name string) bool {
		return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(name), "."), domain)
	}
	invalid := DomainExpiry{Domain: domain, Error: "who.ga 返回的信息格式无效，请稍后重试或到注册商查看"}
	mismatch := DomainExpiry{Domain: domain, Error: "who.ga 返回的域名不匹配，未采用到期信息，请到注册商查看"}

	out := DomainExpiry{Domain: domain}
	status, body, fail := ask("/" + domain)
	if fail != nil {
		return *fail
	}
	var brief whogaBrief
	decoded := json.Unmarshal(body, &brief) == nil
	if status != http.StatusOK {
		f := whogaStatusFailure(status, brief.ErrorCode)
		f.Domain = domain
		return f
	}
	if !decoded {
		return invalid
	}
	if !matches(brief.Domain) {
		return mismatch
	}
	text := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	if v := text(brief.RegisteredAt); v != "" {
		out.RegisteredAt = whoisDate(v)
	}
	out.Registrar = short(strings.TrimSpace(text(brief.Registrar)), 200)
	if v := text(brief.ExpiresAt); v != "" {
		if out.ExpiresAt = whoisDate(v); out.ExpiresAt == "" {
			return invalid
		}
		out.Retryable, out.Source = boolPtr(false), "who.ga"
		return out
	}

	status, body, fail = ask("/full/" + domain)
	if fail != nil {
		return *fail
	}
	var full whogaFull
	decoded = json.Unmarshal(body, &full) == nil
	if status != http.StatusOK {
		f := whogaStatusFailure(status, full.ErrorCode)
		f.Domain = domain
		return f
	}
	if !decoded || full.Result == nil {
		return invalid
	}
	if !matches(full.Domain) {
		return mismatch
	}
	r := full.Result
	if expires := rdapEarliest(r.Events, "expiration"); expires != "" {
		out.ExpiresAt = expires
		out.RegisteredAt = firstNonEmpty(rdapEarliest(r.Events, "registration"), out.RegisteredAt)
		out.Registrar = firstNonEmpty(r.registrar(), out.Registrar)
	} else if r.Parsed != nil {
		if name := parsedField(r.Parsed, "domain name", "domain"); name != "" && !matches(name) {
			return mismatch
		}
		raw := parsedField(r.Parsed, "registry expiry date", "expiration date", "expiration time", "expiry date",
			"registrar registration expiration date", "expiration-date", "expire", "expires", "paid-till")
		if raw == "" {
			return DomainExpiry{Domain: domain, Error: "who.ga 未返回该域名的到期时间，请到注册商查看", Retryable: boolPtr(false)}
		}
		if out.ExpiresAt = whoisDate(raw); out.ExpiresAt == "" {
			return DomainExpiry{Domain: domain, Error: "who.ga 返回的到期时间格式无效，请到注册商查看"}
		}
		if reg := whoisDate(parsedField(r.Parsed, "creation date", "registration time", "created-date", "created", "registered")); reg != "" {
			out.RegisteredAt = reg
		}
		out.Registrar = firstNonEmpty(out.Registrar, short(parsedField(r.Parsed, "registrar", "sponsoring registrar", "registration-service-provider"), 200))
	}
	if out.ExpiresAt == "" {
		return DomainExpiry{Domain: domain, Error: "who.ga 未返回该域名的到期时间，请到注册商查看", Retryable: boolPtr(false)}
	}
	out.Retryable, out.Source = boolPtr(false), "who.ga"
	return out
}
