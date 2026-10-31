package plost

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestSiteCheckURLRules(t *testing.T) {
	for path, want := range map[string]string{"": "https://example.com/", "/": "https://example.com/", "/health": "https://example.com/health",
		"/a/../b": "https://example.com/b", "/ping?x=1": "https://example.com/ping?x=1"} {
		if u, err := siteCheckURL("example.com", path); err != nil || u.String() != want {
			t.Errorf("%q: got %v %v, want %s", path, u, err, want)
		}
	}
	for _, path := range []string{"health", "//evil.example/x", "https://evil.example/", "/a#frag", "/" + strings.Repeat("a", 2048), "/\x00", "\\evil.example"} {
		if _, err := siteCheckURL("example.com", path); err == nil {
			t.Errorf("%q should stay on the site and was accepted", path)
		}
	}
}

func TestValidatePublicURL(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://example.com/": true, "https://example.com:443/x": true, "http://example.com:80/": true, "https://Sub.Example.com/a?b=1": true,
		"https://example.com:8443/": false, "https://user:pw@example.com/": false, "ftp://example.com/": false, "https://example.com/#frag": false,
		"https://127.0.0.1/": false, "https://10.1.2.3/": false, "https://[::1]/": false, "https://169.254.169.254/": false, "https://100.64.0.1/": false,
		"https://localhost/": false, "https://printer.local/": false, "https://db.internal/": false, "https://8.8.8.8/": true,
	} {
		if err := validatePublicURL(mustURL(t, raw)); (err == nil) != ok {
			t.Errorf("%s: accepted=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
}

func TestProbeHTTPOutcomes(t *testing.T) {
	var plain *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "Plost-SiteCheck/1.0" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			http.Error(w, "unexpected headers", 400)
			return
		}
		io.WriteString(w, "fine")
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) })
	mux.HandleFunc("/missing", http.NotFound)
	mux.HandleFunc("/chain/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		if n == 0 {
			io.WriteString(w, "end")
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/chain/%d", n-1), http.StatusFound)
	})
	mux.HandleFunc("/downgrade", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, plain.URL+"/ok", http.StatusFound) })
	mux.HandleFunc("/to-internal", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://127.0.0.1/secret", http.StatusFound)
	})
	mux.HandleFunc("/to-name", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://localhost/secret", http.StatusFound)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	plain = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer plain.Close()

	// The test server lives on loopback, so only the redirect hop and downgrade rules are exercised here.
	// httptest hands out one shared client, so each policy gets its own copy.
	lenientCopy, strictCopy := *srv.Client(), *srv.Client()
	lenient, strict := &lenientCopy, &strictCopy
	lenient.CheckRedirect = redirectPolicy(func(*url.URL) error { return nil })
	strict.CheckRedirect = followPublicRedirect
	probe := func(client *http.Client, path string) HTTPCheck {
		return probeHTTP(context.Background(), client, mustURL(t, srv.URL+path), time.Now)
	}

	if got := probe(lenient, "/ok"); !got.OK || got.Status == nil || *got.Status != 200 || got.DurationMs == nil || got.Error != "" {
		t.Fatalf("a 200 answer is healthy: %+v", got)
	}
	for path, want := range map[string]int{"/error": 500, "/missing": 404} {
		got := probe(lenient, path)
		if got.OK || got.Status == nil || *got.Status != want || got.Error != fmt.Sprintf("HTTP %d", want) {
			t.Errorf("%s: %+v", path, got)
		}
	}
	if got := probe(lenient, "/chain/3"); !got.OK {
		t.Fatalf("three redirects are followed: %+v", got)
	}
	if got := probe(lenient, "/chain/4"); got.OK || got.Error != "跳转次数过多" || got.Status != nil {
		t.Fatalf("a fourth redirect is refused: %+v", got)
	}
	if got := probe(lenient, "/downgrade"); got.OK || got.Error != "HTTPS 跳转到了不安全的 HTTP 地址" {
		t.Fatalf("an HTTPS to HTTP redirect is refused: %+v", got)
	}
	for _, path := range []string{"/to-internal", "/to-name"} {
		if got := probe(strict, path); got.OK || got.Error != "不允许检查内网或保留地址" {
			t.Fatalf("%s: a redirect into the local network is refused: %+v", path, got)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if got := probeHTTP(ctx, lenient, mustURL(t, srv.URL+"/slow"), time.Now); got.OK || got.Error != "检查超时" {
		t.Fatalf("a slow site is a timeout: %+v", got)
	}
	// A client that does not trust the server's certificate sees it as a certificate problem.
	if got := probe(&http.Client{}, "/ok"); got.OK || got.Error != "HTTPS 证书验证失败" {
		t.Fatalf("an untrusted certificate: %+v", got)
	}
	dead := httptest.NewTLSServer(mux)
	deadURL := dead.URL
	dead.Close()
	if got := probeHTTP(context.Background(), &http.Client{}, mustURL(t, deadURL+"/ok"), time.Now); got.OK || got.Error != "无法连接网站" {
		t.Fatalf("a refused connection: %+v", got)
	}
}

func TestProbeRefusesLocalAddressesAtConnectTime(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	got := probeHTTP(context.Background(), newSiteHTTPClient(nil), mustURL(t, srv.URL), time.Now)
	if got.OK || got.Error != "域名没有可安全访问的公网地址" || hits.Load() != 0 {
		t.Fatalf("the real client must not reach loopback: %+v (server saw %d requests)", got, hits.Load())
	}
}

func TestProbeReportsAnUnknownName(t *testing.T) {
	var dns *net.DNSError
	if _, err := net.LookupHost("nonexistent.invalid"); !errors.As(err, &dns) {
		t.Skip("this machine's resolver does not answer with a DNS error")
	}
	if got := probeHTTP(context.Background(), newSiteHTTPClient(nil), mustURL(t, "https://nonexistent.invalid/"), time.Now); got.Error != "域名解析失败" {
		t.Fatalf("an unknown name: %+v", got)
	}
}

// Every failure is reduced to a short message that carries no address, stack or response body.
func TestCheckErrorMessages(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"already worded":  {siteCheckError("跳转次数过多"), "跳转次数过多"},
		"wrapped wording": {&url.Error{Op: "Get", URL: "https://secret.example/?token=abc", Err: siteCheckError("跳转次数过多")}, "跳转次数过多"},
		"deadline":        {context.DeadlineExceeded, "检查超时"},
		"network timeout": {&net.OpError{Op: "dial", Err: &timeoutError{}}, "检查超时"},
		"local address":   {&net.OpError{Op: "dial", Err: errNotPublic}, "域名没有可安全访问的公网地址"},
		"unknown name":    {&url.Error{Op: "Get", URL: "https://secret.example/", Err: &net.DNSError{Name: "secret.example", Err: "no such host"}}, "域名解析失败"},
		"certificate":     {&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}, "HTTPS 证书验证失败"},
		"refused":         {&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}, "无法连接网站"},
		"hung up":         {&url.Error{Op: "Get", URL: "https://secret.example/", Err: io.EOF}, "无法连接网站"},
		"cut short":       {&url.Error{Op: "Get", URL: "https://secret.example/", Err: io.ErrUnexpectedEOF}, "无法连接网站"},
		"anything else":   {errors.New("Get \"https://secret.example/?token=abc\": strange failure at 10.0.0.1"), "检查失败，请稍后重试"},
	} {
		got := checkErrorMessage(tc.err)
		if got != tc.want || strings.Contains(got, "secret") || strings.Contains(got, "10.0.0.1") || strings.Contains(got, "token") {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}

type timeoutError struct{}

func (*timeoutError) Error() string   { return "i/o timeout" }
func (*timeoutError) Timeout() bool   { return true }
func (*timeoutError) Temporary() bool { return true }

func certWithPolicies(t *testing.T, oids ...string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "example.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	for _, text := range oids {
		oid, err := x509.ParseOID(text)
		if err != nil {
			t.Fatal(err)
		}
		template.Policies = append(template.Policies, oid)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestCertificateValidationLevels(t *testing.T) {
	for want, oids := range map[string][]string{
		"EV": {"2.23.140.1.1"}, "OV": {"2.23.140.1.2.2"}, "DV": {"2.23.140.1.2.1"},
		"": {"1.2.3.4"},
	} {
		if got := certificateValidation(certWithPolicies(t, oids...)); got != want {
			t.Errorf("%v: %q, want %q", oids, got, want)
		}
	}
	if got := certificateValidation(certWithPolicies(t)); got != "" {
		t.Errorf("no policy: %q", got)
	}
	if got := certificateValidation(certWithPolicies(t, "2.23.140.1.2.1", "2.23.140.1.1", "2.23.140.1.2.2")); got != "EV" {
		t.Errorf("EV outranks OV and DV whatever the order: %q", got)
	}
}

func TestInspectCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	roots := srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	leaf := srv.Certificate()
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		if !strings.HasSuffix(address, ":443") {
			t.Errorf("certificates are read on the HTTPS port, not %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", srv.Listener.Addr().String())
	}
	now := time.Now()

	got, err := inspectCertificate(context.Background(), dial, roots, "example.com", now)
	if err != nil || !got.Valid || got.Error != "" || got.ExpiresAt != isoTime(leaf.NotAfter) {
		t.Fatalf("a trusted certificate for the right name: %+v %v", got, err)
	}
	if got, _ = inspectCertificate(context.Background(), dial, roots, "example.com", leaf.NotAfter.Add(time.Hour)); got.Valid || got.Error != "证书已过期" || got.ExpiresAt == "" {
		t.Fatalf("an expired certificate still reports its date: %+v", got)
	}
	if got, _ = inspectCertificate(context.Background(), dial, roots, "other.example.org", now); got.Valid || got.Error != "证书域名不匹配" || got.ExpiresAt == "" {
		t.Fatalf("a certificate for another name: %+v", got)
	}
	if got, _ = inspectCertificate(context.Background(), dial, nil, "example.com", now); got.Valid || got.Error != "证书未通过验证" || got.ExpiresAt == "" {
		t.Fatalf("an untrusted issuer: %+v", got)
	}
	refuse := func(context.Context, string, string) (net.Conn, error) { return nil, errNotPublic }
	if _, err := inspectCertificate(context.Background(), refuse, roots, "example.com", now); !errors.Is(err, errNotPublic) {
		t.Fatalf("a refused connection is an error, not a verdict: %v", err)
	}
	var _ tls.Certificate
}

type fakeReply struct {
	status int
	body   string
	err    error
}

// newFakeLookup answers RDAP requests from a table and records every URL asked for.
func newFakeLookup(routes map[string]fakeReply, whois func(string) whoisResult) (*domainLookup, *[]string, *time.Time) {
	var asked []string
	clock := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	l := &domainLookup{now: func() time.Time { return clock }}
	l.fetch = func(ctx context.Context, u *url.URL) (int, []byte, error) {
		asked = append(asked, u.String())
		reply, ok := routes[u.String()]
		if !ok {
			return 404, nil, nil
		}
		return reply.status, []byte(reply.body), reply.err
	}
	l.whois = func(ctx context.Context, domain string) whoisResult { return whois(domain) }
	return l, &asked, &clock
}

const (
	bootstrapURL  = "https://data.iana.org/rdap/dns.json"
	bootstrapBody = `{"services":[[["com","net"],["https://rdap.registry.test/com/"]],[["uk"],["https://rdap.uk.test/"]],[["cn"],[]],[["io"],["http://insecure.test/","https://rdap.io.test"]]]}`
	rdapBody      = `{"events":[{"eventAction":"registration","eventDate":"2010-05-01T00:00:00Z"},{"eventAction":"expiration","eventDate":"2027-05-01T00:00:00Z"},` +
		`{"eventAction":"expiration","eventDate":"2026-12-31T23:59:59Z"},{"eventAction":"last changed","eventDate":"2025-01-01T00:00:00Z"}],` +
		`"entities":[{"roles":["registrant"],"vcardArray":["vcard",[["fn",{},"text","Someone Private"]]]},` +
		`{"roles":["registrar"],"vcardArray":["vcard",[["version",{},"text","4.0"],["fn",{},"text"," Example Registrar, Inc. "]]]}]}`
)

func TestDomainExpiryFromRDAP(t *testing.T) {
	noWhois := func(string) whoisResult { t.Fatal("WHOIS must not be asked when RDAP answers"); return whoisResult{} }
	lookup, asked, _ := newFakeLookup(map[string]fakeReply{
		bootstrapURL: {status: 200, body: bootstrapBody},
		"https://rdap.registry.test/com/domain/example.com": {status: 200, body: rdapBody},
		"https://rdap.uk.test/domain/example.co.uk":         {status: 200, body: rdapBody},
		"https://rdap.io.test/domain/github.io":             {status: 200, body: rdapBody},
	}, noWhois)
	got := lookup.inspect(context.Background(), "www.shop.example.com")
	want := DomainExpiry{Domain: "example.com", ExpiresAt: "2026-12-31T23:59:59.000Z", RegisteredAt: "2010-05-01T00:00:00.000Z", Registrar: "Example Registrar, Inc.", Retryable: boolPtr(false)}
	if got != want && !(got.Domain == want.Domain && got.ExpiresAt == want.ExpiresAt && got.RegisteredAt == want.RegisteredAt && got.Registrar == want.Registrar && got.Error == "") {
		t.Fatalf("got %+v, want %+v (the earliest expiration, and the registrar rather than the registrant)", got, want)
	}
	if got := lookup.inspect(context.Background(), "a.b.example.co.uk"); got.Domain != "example.co.uk" || got.ExpiresAt == "" {
		t.Fatalf("a two-part suffix: %+v", got)
	}
	if got := lookup.inspect(context.Background(), "foo.github.io"); got.Domain != "github.io" || got.ExpiresAt == "" {
		t.Fatalf("only ICANN suffixes count, and the first https endpoint is used: %+v", got)
	}
	if (*asked)[1] != "https://rdap.registry.test/com/domain/example.com" {
		t.Fatalf("requests: %v", *asked)
	}
	// The bootstrap list is fetched once a day, not for every lookup.
	fetches := 0
	for _, u := range *asked {
		if u == bootstrapURL {
			fetches++
		}
	}
	if fetches != 1 {
		t.Fatalf("the IANA bootstrap was fetched %d times", fetches)
	}
}

func TestDomainExpiryFailureModes(t *testing.T) {
	cnCalls := 0
	whois := func(domain string) whoisResult {
		cnCalls++
		if domain != "example.com.cn" {
			t.Errorf("WHOIS asked about %q", domain)
		}
		return whoisResult{ExpiresAt: "2027-03-17"}
	}
	lookup, _, clock := newFakeLookup(map[string]fakeReply{
		bootstrapURL: {status: 200, body: bootstrapBody},
		"https://rdap.registry.test/com/domain/gone.com":     {status: 404},
		"https://rdap.registry.test/com/domain/noevents.com": {status: 200, body: `{"events":[{"eventAction":"registration","eventDate":"2010-01-01T00:00:00Z"}]}`},
		"https://rdap.registry.test/com/domain/garbage.com":  {status: 200, body: `not json`},
		"https://rdap.registry.test/com/domain/slow.com":     {err: context.DeadlineExceeded},
	}, whois)
	ctx := context.Background()
	notRetryable := func(d DomainExpiry) bool { return d.Retryable != nil && !*d.Retryable }
	if d := lookup.inspect(ctx, "localhost"); d.Error != "无法识别可注册域名" || !notRetryable(d) {
		t.Errorf("not a registrable name: %+v", d)
	}
	if d := lookup.inspect(ctx, "com"); d.Error != "无法识别可注册域名" {
		t.Errorf("a bare suffix: %+v", d)
	}
	if d := lookup.inspect(ctx, "example.xyz"); d.Error != "暂不支持该域名后缀的到期查询，请到注册商查看" || !notRetryable(d) {
		t.Errorf("a suffix without RDAP: %+v", d)
	}
	if d := lookup.inspect(ctx, "gone.com"); d.Error != "域名到期信息暂时不可用" || d.Retryable != nil {
		t.Errorf("a registry error is worth retrying: %+v", d)
	}
	if d := lookup.inspect(ctx, "noevents.com"); d.Error != "查询服务未返回该域名的到期时间，请到注册商查看" || !notRetryable(d) {
		t.Errorf("no expiration published: %+v", d)
	}
	if d := lookup.inspect(ctx, "garbage.com"); d.Error != "域名到期信息格式无效" {
		t.Errorf("an unreadable answer: %+v", d)
	}
	if d := lookup.inspect(ctx, "slow.com"); d.Error != "检查超时" {
		t.Errorf("a timeout: %+v", d)
	}
	if cnCalls != 0 {
		t.Fatal("WHOIS is only a fallback for .cn names")
	}
	if d := lookup.inspect(ctx, "shop.example.com.cn"); d.Domain != "example.com.cn" || d.ExpiresAt != "2027-03-17" || d.Error != "" || cnCalls != 1 {
		t.Errorf("a .cn name without RDAP falls back to WHOIS: %+v", d)
	}

	broken, _, _ := newFakeLookup(map[string]fakeReply{bootstrapURL: {status: 500}}, whois)
	if d := broken.inspect(ctx, "example.com"); d.Error != "域名到期信息暂时不可用" {
		t.Errorf("bootstrap unavailable: %+v", d)
	}
	invalid, _, _ := newFakeLookup(map[string]fakeReply{bootstrapURL: {status: 200, body: `{"services":[]}`}}, whois)
	if d := invalid.inspect(ctx, "example.com"); d.Error != "域名到期信息格式无效" {
		t.Errorf("bootstrap unreadable: %+v", d)
	}

	// After a day the bootstrap list is fetched again.
	fetches := 0
	counting, asked, tick := newFakeLookup(map[string]fakeReply{bootstrapURL: {status: 200, body: bootstrapBody}}, whois)
	for i := 0; i < 2; i++ {
		counting.inspect(ctx, "example.com")
	}
	*tick = tick.Add(25 * time.Hour)
	counting.inspect(ctx, "example.com")
	_ = clock
	for _, u := range *asked {
		if u == bootstrapURL {
			fetches++
		}
	}
	if fetches != 2 {
		t.Errorf("bootstrap fetched %d times over 25 hours, want 2", fetches)
	}
}

func TestCnWhoisParsing(t *testing.T) {
	record := func(extra string) string {
		return "Domain Name: example.cn\nROID: 20030311s10001s00033735-cn\nDomain Status: clientDeleteProhibited\nRegistrant: Private Person\n" +
			"Registrant Contact Email: private@example.invalid\nSponsoring Registrar: Some Registrar\nRegistration Time: 2003-03-17 12:20:05\n" + extra
	}
	for name, tc := range map[string]struct {
		domain, response, expires, failure string
		retryable                          bool
	}{
		"date only keeps the published day": {"example.cn", record("Expiration Time: 2027-03-17 12:48:36\n"), "2027-03-17", "", false},
		"windows line endings":              {"example.cn", strings.ReplaceAll(record("Expiration Time: 2027-03-17 12:48:36\n"), "\n", "\r\n"), "2027-03-17", "", false},
		"upper case query":                  {"Example.CN", record("Expiration Time: 2027-03-17 12:48:36\n"), "2027-03-17", "", false},
		"explicit UTC":                      {"example.cn", record("Expiration Time: 2027-03-17T12:48:36Z\n"), "2027-03-17T12:48:36.000Z", "", false},
		"explicit offset":                   {"example.cn", record("Expiration Time: 2027-03-17T12:48:36+08:00\n"), "2027-03-17T04:48:36.000Z", "", false},
		"largest offset":                    {"example.cn", record("Expiration Time: 2027-03-17 12:00:00+14:00\n"), "2027-03-16T22:00:00.000Z", "", false},
		"offset beyond +14:00":              {"example.cn", record("Expiration Time: 2027-03-17 12:00:00+14:30\n"), "", "WHOIS 返回的到期时间格式无效，请到注册商查看", true},
		"impossible date":                   {"example.cn", record("Expiration Time: 2027-02-30 10:00:00\n"), "", "WHOIS 返回的到期时间格式无效，请到注册商查看", true},
		"two expirations are ambiguous":     {"example.cn", record("Expiration Time: 2027-03-17 12:48:36\nExpiration Time: 2028-03-17 12:48:36\n"), "", "WHOIS 返回的到期时间格式无效，请到注册商查看", true},
		"no expiration":                     {"example.cn", record(""), "", "WHOIS 未返回该域名的到期时间，请到注册商查看", false},
		"another domain":                    {"example.cn", strings.Replace(record("Expiration Time: 2027-03-17 12:48:36\n"), "example.cn", "other.cn", 1), "", "WHOIS 返回的域名不匹配，未采用到期信息，请到注册商查看", true},
		"two records":                       {"example.cn", record("Domain Name: other.cn\nExpiration Time: 2027-03-17 12:48:36\n"), "", "WHOIS 返回的域名不匹配，未采用到期信息，请到注册商查看", true},
		"no such record":                    {"example.cn", "no matching record\n", "", "WHOIS 未返回该域名的记录，请到注册商查看", false},
		"empty answer":                      {"example.cn", "", "", "WHOIS 返回的信息格式无效，请稍后重试或到注册商查看", true},
		"busy":                              {"example.cn", "The system is busy, try again later\n", "", "WHOIS 服务暂时繁忙，请稍后重试或到注册商查看", true},
		"rate limited":                      {"example.cn", "Query quota exceeded\n", "", "WHOIS 查询受限，请稍后重试或到注册商查看", true},
		"oversized":                         {"example.cn", record("Expiration Time: 2027-03-17 12:48:36\n" + strings.Repeat("x", whoisLimit)), "", "WHOIS 响应内容过大，未采用到期信息，请到注册商查看", true},
		"not a .cn name":                    {"example.com", record("Expiration Time: 2027-03-17 12:48:36\n"), "", "WHOIS 兜底仅支持有效的 .cn 可注册域名", false},
		"a subdomain is not a registration": {"www.example.cn", record("Expiration Time: 2027-03-17 12:48:36\n"), "", "WHOIS 兜底仅支持有效的 .cn 可注册域名", false},
	} {
		got := parseCnWhois(tc.domain, tc.response)
		if got.ExpiresAt != tc.expires || got.Error != tc.failure || (tc.failure != "" && got.Retryable != tc.retryable) {
			t.Errorf("%s: got %+v, want expires=%q failure=%q retryable=%v", name, got, tc.expires, tc.failure, tc.retryable)
		}
	}
	for value, want := range map[string]string{"example.cn": "example.cn", "EXAMPLE.CN": "example.cn", "a.com.cn": "a.com.cn", "com.cn": "", "www.a.com.cn": "",
		"例子.cn": "", "bad_label.cn": "", "-a.cn": "", "a..cn": "", "example.com": ""} {
		if got := cnDomain(value); got != want {
			t.Errorf("cnDomain(%q) = %q, want %q", value, got, want)
		}
	}
}

// whoisServer answers one query on a local socket; the dial function ignores the real registry address.
func whoisServer(t *testing.T, reply func(net.Conn)) (dialFunc, <-chan string, *string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	queries := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadString('\n')
		queries <- line
		reply(conn)
	}()
	var target string
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		target = address
		return (&net.Dialer{}).DialContext(ctx, "tcp", ln.Addr().String())
	}, queries, &target
}

func TestInspectCnWhoisOverTheWire(t *testing.T) {
	dial, queries, target := whoisServer(t, func(c net.Conn) {
		io.WriteString(c, "Domain Name: example.cn\r\nExpiration Time: 2027-03-17 12:48:36\r\n")
	})
	got := inspectCnWhois(context.Background(), dial, "example.cn")
	if got.ExpiresAt != "2027-03-17" || got.Error != "" || *target != "whois.cnnic.cn:43" || <-queries != "example.cn\r\n" {
		t.Fatalf("%+v asked %s", got, *target)
	}

	dial, _, _ = whoisServer(t, func(c net.Conn) {})
	if got := inspectCnWhois(context.Background(), dial, "example.cn"); got.Error != "WHOIS 返回的信息格式无效，请稍后重试或到注册商查看" {
		t.Fatalf("a server that says nothing: %+v", got)
	}
	dial, _, _ = whoisServer(t, func(c net.Conn) { io.WriteString(c, strings.Repeat("x", whoisLimit+10)) })
	if got := inspectCnWhois(context.Background(), dial, "example.cn"); got.Error != "WHOIS 响应内容过大，未采用到期信息，请到注册商查看" {
		t.Fatalf("an oversized answer: %+v", got)
	}
	dial, _, _ = whoisServer(t, func(c net.Conn) { time.Sleep(2 * time.Second) })
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if got := inspectCnWhois(ctx, dial, "example.cn"); got.Error != "WHOIS 查询超时，请稍后重试或到注册商查看" || !got.Retryable {
		t.Fatalf("a server that never finishes: %+v", got)
	}
	for err, want := range map[error]string{
		errNotPublic:    "WHOIS 服务地址未通过公网安全校验，请稍后重试",
		&net.DNSError{}: "WHOIS 服务域名解析失败，请稍后重试",
		errors.New("x"): "无法连接 WHOIS 服务，当前网络可能限制 TCP 43 端口，请稍后重试或到注册商查看",
	} {
		failing := func(context.Context, string, string) (net.Conn, error) { return nil, err }
		if got := inspectCnWhois(context.Background(), failing, "example.cn"); got.Error != want {
			t.Errorf("%v: %+v", err, got)
		}
	}
	if got := inspectCnWhois(context.Background(), nil, "example.com"); got.Error != "WHOIS 兜底仅支持有效的 .cn 可注册域名" {
		t.Fatalf("a name that is not .cn is refused before any connection: %+v", got)
	}
}

func TestRegistrableDomain(t *testing.T) {
	for host, want := range map[string]string{"example.com": "example.com", "www.shop.example.com": "example.com", "a.b.example.co.uk": "example.co.uk",
		"foo.github.io": "github.io", "x.example.com.cn": "example.com.cn", "EXAMPLE.COM.": "example.com", "localhost": "", "com": "", "co.uk": "", "": ""} {
		got, ok := registrableDomain(host)
		if got != want || ok != (want != "") {
			t.Errorf("%q: %q %v, want %q", host, got, ok, want)
		}
	}
}

func TestExpiryRefreshSchedule(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	snapshot := func(age time.Duration, mutate func(*ExpirySnapshot)) *ExpirySnapshot {
		s := &ExpirySnapshot{CheckedAt: isoTime(now.Add(-age)), Domain: DomainExpiry{ExpiresAt: "2027-01-01T00:00:00.000Z"}, Certificate: CertificateResult{ExpiresAt: "2027-01-01T00:00:00.000Z", Valid: true}}
		if mutate != nil {
			mutate(s)
		}
		return s
	}
	for name, tc := range map[string]struct {
		s    *ExpirySnapshot
		want bool
	}{
		"never looked up":                {nil, true},
		"fresh success":                  {snapshot(time.Hour, nil), false},
		"a day later":                    {snapshot(25*time.Hour, nil), true},
		"transient failure, soon":        {snapshot(10*time.Minute, func(s *ExpirySnapshot) { s.Domain.Error = "检查超时" }), false},
		"transient failure, retry":       {snapshot(16*time.Minute, func(s *ExpirySnapshot) { s.Domain.Error = "检查超时" }), true},
		"permanent failure waits a day":  {snapshot(2*time.Hour, func(s *ExpirySnapshot) { s.Domain.Error, s.Domain.Retryable = "暂不支持", boolPtr(false) }), false},
		"permanent failure, a day later": {snapshot(25*time.Hour, func(s *ExpirySnapshot) { s.Domain.Error, s.Domain.Retryable = "暂不支持", boolPtr(false) }), true},
		"certificate problem is retried": {snapshot(16*time.Minute, func(s *ExpirySnapshot) { s.Certificate.Error = "证书已过期" }), true},
		"unreadable timestamp":           {snapshot(time.Hour, func(s *ExpirySnapshot) { s.CheckedAt = "garbage" }), true},
	} {
		if got := expiryRefreshDue(tc.s, now); got != tc.want {
			t.Errorf("%s: due=%v, want %v", name, got, tc.want)
		}
	}
}

func TestMergeExpiryKeepsWhatWasLearned(t *testing.T) {
	previous := &ExpirySnapshot{CheckedAt: "2026-10-30T12:00:00.000Z",
		Domain:      DomainExpiry{Domain: "example.com", ExpiresAt: "2027-01-01T00:00:00.000Z", Registrar: "Old Registrar", RegisteredAt: "2010-01-01T00:00:00.000Z"},
		Certificate: CertificateResult{ExpiresAt: "2026-12-01T00:00:00.000Z", Valid: true, Validation: "OV"}}
	current := ExpirySnapshot{CheckedAt: "2026-10-31T12:00:00.000Z",
		Domain:      DomainExpiry{Domain: "example.com", Error: "检查超时"},
		Certificate: CertificateResult{ExpiresAt: "2026-12-15T00:00:00.000Z", Valid: true}}
	got := mergeExpiry(previous, current)
	if got.Domain.ExpiresAt != "2027-01-01T00:00:00.000Z" || !got.Domain.Stale || got.Domain.LastSuccessAt != "2026-10-30T12:00:00.000Z" || got.Domain.Error != "检查超时" ||
		got.Domain.Registrar != "Old Registrar" || got.Domain.RegisteredAt != "2010-01-01T00:00:00.000Z" {
		t.Fatalf("a failed lookup keeps the last date and says it is old: %+v", got.Domain)
	}
	if got.Certificate.ExpiresAt != "2026-12-15T00:00:00.000Z" || got.Certificate.Stale || got.Certificate.LastSuccessAt != current.CheckedAt || got.Certificate.Validation != "OV" {
		t.Fatalf("a fresh certificate replaces the old one but keeps the validation level: %+v", got.Certificate)
	}
	recovered := mergeExpiry(&got, ExpirySnapshot{CheckedAt: "2026-11-01T12:00:00.000Z", Domain: DomainExpiry{Domain: "example.com", ExpiresAt: "2027-02-02T00:00:00.000Z"}, Certificate: current.Certificate})
	if recovered.Domain.Stale || recovered.Domain.ExpiresAt != "2027-02-02T00:00:00.000Z" || recovered.Domain.LastSuccessAt != "2026-11-01T12:00:00.000Z" || recovered.Domain.Registrar != "Old Registrar" {
		t.Fatalf("a later success clears the stale marker: %+v", recovered.Domain)
	}
	first := mergeExpiry(nil, ExpirySnapshot{CheckedAt: "2026-10-31T12:00:00.000Z", Domain: DomainExpiry{Error: "检查超时"}, Certificate: CertificateResult{Error: "无法连接网站"}})
	if first.Domain.Stale || first.Domain.ExpiresAt != "" || first.Certificate.Stale || first.Certificate.ExpiresAt != "" {
		t.Fatalf("with nothing learned yet nothing is invented: %+v", first)
	}
}
