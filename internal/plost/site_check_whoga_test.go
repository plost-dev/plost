package plost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// Shapes copied from real who.ga answers (2026-11-04), with the contact fields kept to show they are dropped.
const (
	whogaNotFound   = `{"domain":"gone.com","queryType":"domain","result":{"partial":false,"whoisRaw":"No match"},"error":"Domain not found","errorCode":"DOMAIN_NOT_FOUND"}`
	whogaBriefST    = `{"domain":"plo.st","registeredAt":"2026-10-29T00:00:00.000Z","expiresAt":"2027-10-29T00:00:00.000Z","status":["serverTransferProhibited"],"registrar":"Nets To Limited","nameservers":["a.ns.test"]}`
	whogaBriefCnNil = `{"domain":"example.com.cn","registeredAt":null,"expiresAt":null,"status":["clientDeleteProhibited"],"registrar":"Example CN Registrar","nameservers":["a.ns.test"]}`
	whogaFullCn     = `{"domain":"example.com.cn","queryType":"domain","rdapServer":null,"match":{"resolver":"whois-port43"},"result":{"partial":false,"whoisRaw":"Domain Name: example.com.cn\nRegistrant: Private Person\n","whoisHost":"whois.cnnic.cn",` +
		`"parsed":{"domain name":"example.com.cn","registrant":"Private Person","registrant contact email":"someone@example.test","sponsoring registrar":"Example CN Registrar","registration time":"2000-02-15 00:00:00","expiration time":"2029-02-15 00:00:00"}}}`
	whogaFullRDAP = `{"domain":"example.dev","queryType":"domain","result":{"events":[{"eventAction":"registration","eventDate":"2020-01-02T03:04:05Z"},{"eventAction":"expiration","eventDate":"2028-01-02T03:04:05Z"}],` +
		`"entities":[{"roles":["registrant"],"vcardArray":["vcard",[["fn",{},"text","Someone Private"]]]},{"roles":["registrar"],"vcardArray":["vcard",[["fn",{},"text","Dev Registrar"]]]}]}}`
)

func TestDomainExpiryFromWhoga(t *testing.T) {
	cnnicDown := func(string) whoisResult {
		return whoisFailure("无法连接 WHOIS 服务，当前网络可能限制 TCP 43 端口，请稍后重试或到注册商查看", true)
	}
	lookup, asked, _ := newFakeLookup(map[string]fakeReply{
		bootstrapURL: {status: 200, body: bootstrapBody},
		"https://rdap.registry.test/com/domain/example.com": {status: 200, body: rdapBody},
		"https://rdap.registry.test/com/domain/gone.com":    {status: 404},
		"https://api.who.ga/plo.st":                         {status: 200, body: whogaBriefST},
		"https://api.who.ga/example.com.cn":                 {status: 200, body: whogaBriefCnNil},
		"https://api.who.ga/full/example.com.cn":            {status: 200, body: whogaFullCn},
		"https://api.who.ga/example.dev":                    {status: 200, body: `{"domain":"example.dev","expiresAt":null}`},
		"https://api.who.ga/full/example.dev":               {status: 200, body: whogaFullRDAP},
		"https://api.who.ga/gone.com":                       {status: 404, body: whogaNotFound},
		"https://api.who.ga/other.st":                       {status: 200, body: whogaBriefST},
		"https://api.who.ga/busy.st":                        {status: 429, body: `{"error":"rate limited"}`},
		"https://api.who.ga/junk.st":                        {status: 200, body: `<html>`},
		"https://api.who.ga/baddate.st":                     {status: 200, body: `{"domain":"baddate.st","expiresAt":"soon"}`},
		"https://api.who.ga/empty.st":                       {status: 200, body: `{"domain":"empty.st","expiresAt":null}`},
		"https://api.who.ga/full/empty.st":                  {status: 200, body: `{"domain":"empty.st","result":{"parsed":{"domain name":"empty.st","registrant":"x"}}}`},
	}, cnnicDown)
	ctx := context.Background()

	// A registry with RDAP answers by itself; who.ga is not asked.
	if d := lookup.inspect(ctx, "www.example.com"); d.Source != "rdap" || d.ExpiresAt == "" {
		t.Fatalf("RDAP first: %+v", d)
	}
	for _, u := range *asked {
		if strings.Contains(u, "who.ga") {
			t.Fatalf("who.ga asked although the registry answered: %v", *asked)
		}
	}

	// .st has no RDAP service: who.ga's brief answer fills in.
	d := lookup.inspect(ctx, "www.plo.st")
	if d.Domain != "plo.st" || d.ExpiresAt != "2027-10-29T00:00:00.000Z" || d.RegisteredAt != "2026-10-29T00:00:00.000Z" ||
		d.Registrar != "Nets To Limited" || d.Source != "who.ga" || d.Error != "" || d.Retryable == nil || *d.Retryable {
		t.Fatalf("a suffix without RDAP uses who.ga: %+v", d)
	}

	// .cn with CNNIC's port 43 unreachable: the brief answer has no date, so the full record is read,
	// keeping only the date and registrar; the zone-less CNNIC time keeps only the calendar date.
	d = lookup.inspect(ctx, "shop.example.com.cn")
	if d.ExpiresAt != "2029-02-15" || d.RegisteredAt != "2000-02-15" || d.Registrar != "Example CN Registrar" || d.Source != "who.ga" {
		t.Fatalf("a .cn name through who.ga's full record: %+v", d)
	}
	if strings.Contains(strings.ToLower(d.Registrar+d.Error), "private") {
		t.Fatalf("contact data kept: %+v", d)
	}

	// A full record in RDAP form: the earliest expiration and the registrar, not the registrant.
	if d := (&domainLookup{fetch: lookup.fetch, now: lookup.now}).whoga(ctx, "example.dev"); d.ExpiresAt != "2028-01-02T03:04:05.000Z" ||
		d.RegisteredAt != "2020-01-02T03:04:05.000Z" || d.Registrar != "Dev Registrar" {
		t.Fatalf("an RDAP-shaped full record: %+v", d)
	}

	notRetryable := func(d DomainExpiry) bool { return d.Retryable != nil && !*d.Retryable }
	// The registry's transient error stays the message, and the check is retried.
	if d := lookup.inspect(ctx, "gone.com"); d.Error != "域名到期信息暂时不可用" || d.Retryable != nil {
		t.Errorf("a registry error explains the failure and stays retryable: %+v", d)
	}
	for host, want := range map[string]string{
		"other.st":   "who.ga 返回的域名不匹配，未采用到期信息，请到注册商查看",
		"busy.st":    "who.ga 查询受限，请稍后重试或到注册商查看",
		"junk.st":    "who.ga 返回的信息格式无效，请稍后重试或到注册商查看",
		"baddate.st": "who.ga 返回的信息格式无效，请稍后重试或到注册商查看",
		"missing.st": "who.ga 查询暂时不可用，请稍后重试或到注册商查看",
	} {
		if d := lookup.inspect(ctx, host); d.Error != want || d.ExpiresAt != "" || d.Retryable != nil {
			t.Errorf("%s: %+v", host, d)
		}
	}
	if d := lookup.inspect(ctx, "empty.st"); d.Error != "who.ga 未返回该域名的到期时间，请到注册商查看" || !notRetryable(d) {
		t.Errorf("no date published anywhere: %+v", d)
	}

	// A stale result keeps where its date came from.
	prev := ExpirySnapshot{CheckedAt: "2026-10-30T00:00:00.000Z", Domain: DomainExpiry{Domain: "plo.st", ExpiresAt: "2027-10-29T00:00:00.000Z", Source: "who.ga"}}
	merged := mergeExpiry(&prev, ExpirySnapshot{CheckedAt: "2026-10-31T00:00:00.000Z", Domain: DomainExpiry{Domain: "plo.st", Error: "who.ga 检查超时"}})
	if !merged.Domain.Stale || merged.Domain.Source != "who.ga" {
		t.Fatalf("stale merge: %+v", merged.Domain)
	}
}

func TestWhoisDate(t *testing.T) {
	for in, want := range map[string]string{
		"2027-10-29T19:49:28Z":      "2027-10-29T19:49:28.000Z",
		"2027-10-29T19:49:28+08:00": "2027-10-29T11:49:28.000Z",
		"2027-10-29":                "2027-10-29",
		"2027-10-29 19:49:28":       "2027-10-29",
		"2027-02-30":                "",
		"tomorrow":                  "",
		"":                          "",
	} {
		if got := whoisDate(in); got != want {
			t.Errorf("whoisDate(%q) = %q, want %q", in, got, want)
		}
	}
}

// who.ga requests go through the same client as every other site check: a local fake server answers when the
// test trusts it directly, and the real public-only client refuses to reach it at all.
func TestWhogaOverLocalServer(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("User-Agent") != "Plost-SiteCheck/1.0" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			http.Error(w, "unexpected headers", 400)
			return
		}
		switch r.URL.Path {
		case "/plo.st":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(whogaBriefST))
		default:
			w.WriteHeader(404)
			w.Write([]byte(whogaNotFound))
		}
	}))
	defer srv.Close()

	client := *srv.Client()
	trusting := &domainLookup{fetch: publicFetch(&client, func(*url.URL) error { return nil }), whogaBase: srv.URL}
	if d := trusting.whoga(context.Background(), "plo.st"); d.ExpiresAt != "2027-10-29T00:00:00.000Z" || d.Source != "who.ga" {
		t.Fatalf("a local who.ga answer: %+v", d)
	}
	if d := trusting.whoga(context.Background(), "gone.com"); d.Error != "who.ga 未找到该域名的注册记录，请到注册商查看" {
		t.Fatalf("a not-found answer: %+v", d)
	}
	seen := hits.Load()

	strict := &domainLookup{fetch: publicFetch(newSiteHTTPClient(nil), validatePublicURL), whogaBase: srv.URL}
	if d := strict.whoga(context.Background(), "plo.st"); d.ExpiresAt != "" || d.Error != "who.ga 仅支持不含凭据的标准 HTTP/HTTPS 地址" {
		t.Fatalf("the public-only client must refuse a loopback lookup service: %+v", d)
	}
	strictName := &domainLookup{fetch: publicFetch(newSiteHTTPClient(nil), func(*url.URL) error { return nil }), whogaBase: srv.URL}
	if d := strictName.whoga(context.Background(), "plo.st"); d.ExpiresAt != "" || d.Error != "who.ga 域名没有可安全访问的公网地址" {
		t.Fatalf("the connect-time guard also refuses loopback: %+v", d)
	}
	if hits.Load() != seen {
		t.Fatalf("the guarded client reached the local server")
	}
}
