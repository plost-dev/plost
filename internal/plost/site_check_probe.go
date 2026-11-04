package plost

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

// Site checks look at a public website from the server: does it answer with HTTP 200, when does its
// certificate expire, and when does its domain registration expire. Nothing is installed on the site.

// siteCheckError carries a message that is already fit to show and store.
type siteCheckError string

func (e siteCheckError) Error() string { return string(e) }

func isoTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

type HTTPCheck struct {
	CheckedAt  string `json:"checkedAt"`
	OK         bool   `json:"ok"`
	Status     *int   `json:"status"`
	DurationMs *int   `json:"durationMs"`
	Error      string `json:"error"`
}

type DomainExpiry struct {
	Domain        string `json:"domain"`
	ExpiresAt     string `json:"expiresAt"`
	Error         string `json:"error"`
	RegisteredAt  string `json:"registeredAt,omitempty"`
	Registrar     string `json:"registrar,omitempty"`
	Retryable     *bool  `json:"retryable,omitempty"`
	Stale         bool   `json:"stale,omitempty"`
	LastSuccessAt string `json:"lastSuccessAt,omitempty"`
	// Source names where the expiration date came from: the registry's RDAP, CNNIC's WHOIS or who.ga.
	Source string `json:"source,omitempty"`
}

type CertificateResult struct {
	ExpiresAt     string `json:"expiresAt"`
	Valid         bool   `json:"valid"`
	Error         string `json:"error"`
	Validation    string `json:"validation,omitempty"`
	Stale         bool   `json:"stale,omitempty"`
	LastSuccessAt string `json:"lastSuccessAt,omitempty"`
}

// expiryLookupVersion goes up whenever the domain lookup can answer more than before (2: the who.ga
// fallback), so a stored "unsupported" answer from an older lookup is asked again at once instead of
// waiting out the day it would otherwise be kept.
const expiryLookupVersion = 2

type ExpirySnapshot struct {
	CheckedAt   string            `json:"checkedAt"`
	Lookup      int               `json:"lookup,omitempty"`
	Domain      DomainExpiry      `json:"domain"`
	Certificate CertificateResult `json:"certificate"`
}

// siteCheckURL builds the address to probe. The path must stay on the site itself.
func siteCheckURL(domain, path string) (*url.URL, error) {
	bad := siteCheckError("请输入本站的检查路径，例如 / 或 /health")
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || len(path) > 2048 || strings.Contains(path, "#") {
		return nil, bad
	}
	rel, err := url.Parse(path)
	if err != nil || rel.IsAbs() || rel.Host != "" || rel.User != nil || rel.Fragment != "" {
		return nil, bad
	}
	u := (&url.URL{Scheme: "https", Host: domain, Path: "/"}).ResolveReference(rel)
	if u.Host != domain || u.Scheme != "https" {
		return nil, bad
	}
	return u, nil
}

// validatePublicURL refuses anything but a standard web address on a public host. The dialer repeats the
// address check on the connection actually made, so a name that resolves to an internal address is refused too.
func validatePublicURL(u *url.URL) error {
	port := u.Port()
	standard := port == "" || (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80")
	if (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || !standard || u.Hostname() == "" || u.Fragment != "" {
		return siteCheckError("仅支持不含凭据的标准 HTTP/HTTPS 地址")
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil && !isPublicAddress(ip) {
		return siteCheckError("不允许检查内网或保留地址")
	}
	if blockedHost.MatchString(host) {
		return siteCheckError("不允许检查内网或保留地址")
	}
	return nil
}

// checkErrorMessage keeps addresses, stacks and response bodies out of what is shown and stored.
func checkErrorMessage(err error) string {
	var known siteCheckError
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	var netErr net.Error
	var op *net.OpError
	switch {
	case errors.As(err, &known):
		return string(known)
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return "检查超时"
	case errors.Is(err, errNotPublic):
		return "域名没有可安全访问的公网地址"
	case errors.As(err, &dns):
		return "域名解析失败"
	case errors.As(err, &cert):
		return "HTTPS 证书验证失败"
	case errors.As(err, &op) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		// Includes a peer that hangs up during the handshake, which is also how a proxy answers for a name that does not exist.
		return "无法连接网站"
	}
	return "检查失败，请稍后重试"
}

// redirectPolicy follows at most three redirects, never from HTTPS down to HTTP, and checks every target.
func redirectPolicy(validate func(*url.URL) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return siteCheckError("跳转次数过多")
		}
		if via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return siteCheckError("HTTPS 跳转到了不安全的 HTTP 地址")
		}
		return validate(req.URL)
	}
}

var followPublicRedirect = redirectPolicy(validatePublicURL)

func newSiteHTTPClient(allow []netip.Prefix) *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, Control: outboundGuard(allow)}
	return &http.Client{
		CheckRedirect: followPublicRedirect,
		Transport: &http.Transport{
			// Environment proxies would make the dial-time address check see the proxy instead of the site.
			Proxy:                  nil,
			DialContext:            dialer.DialContext,
			ForceAttemptHTTP2:      true,
			TLSHandshakeTimeout:    8 * time.Second,
			ResponseHeaderTimeout:  10 * time.Second,
			MaxResponseHeaderBytes: 64 << 10,
			DisableKeepAlives:      true,
		},
	}
}

// probeHTTP makes one GET and reports whether the final answer is HTTP 200.
func probeHTTP(ctx context.Context, client *http.Client, u *url.URL, now func() time.Time) HTTPCheck {
	result := HTTPCheck{CheckedAt: isoTime(now())}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		result.Error = checkErrorMessage(err)
		return result
	}
	req.Header.Set("User-Agent", "Plost-SiteCheck/1.0")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.Error = checkErrorMessage(err)
		return result
	}
	resp.Body.Close() // only the status matters
	status, ms := resp.StatusCode, int(time.Since(start).Milliseconds())
	result.Status, result.DurationMs, result.OK = &status, &ms, status == http.StatusOK
	if !result.OK {
		result.Error = fmt.Sprintf("HTTP %d", status)
	}
	return result
}

// certificateValidation reads the CA/Browser Forum policy identifier; EV outranks OV, which outranks DV.
func certificateValidation(cert *x509.Certificate) string {
	have := map[string]bool{}
	for _, policy := range cert.Policies {
		have[policy.String()] = true
	}
	switch {
	case have["2.23.140.1.1"]:
		return "EV"
	case have["2.23.140.1.2.2"]:
		return "OV"
	case have["2.23.140.1.2.1"]:
		return "DV"
	}
	return ""
}

type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// inspectCertificate reads the expiry of the certificate a host serves, even an expired or untrusted one.
// No HTTP request or credential is sent on this connection; whether it is valid is reported separately.
func inspectCertificate(ctx context.Context, dial dialFunc, roots *x509.CertPool, host string, now time.Time) (CertificateResult, error) {
	raw, err := dial(ctx, "tcp", net.JoinHostPort(host, "443"))
	if err != nil {
		return CertificateResult{}, err
	}
	defer raw.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}
	conn := tls.Client(raw, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	if err := conn.HandshakeContext(ctx); err != nil {
		return CertificateResult{}, err
	}
	chain := conn.ConnectionState().PeerCertificates
	if len(chain) == 0 {
		return CertificateResult{Error: "证书未通过验证"}, nil
	}
	leaf := chain[0]
	intermediates := x509.NewCertPool()
	for _, c := range chain[1:] {
		intermediates.AddCert(c)
	}
	_, chainErr := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: now})
	hostErr := leaf.VerifyHostname(host)
	expired, notYet := !now.Before(leaf.NotAfter), now.Before(leaf.NotBefore)
	valid := chainErr == nil && hostErr == nil && !expired && !notYet
	result := CertificateResult{ExpiresAt: isoTime(leaf.NotAfter), Valid: valid, Validation: certificateValidation(leaf)}
	switch {
	case valid:
	case expired:
		result.Error = "证书已过期"
	case hostErr != nil:
		result.Error = "证书域名不匹配"
	default:
		result.Error = "证书未通过验证"
	}
	return result, nil
}

// registrableDomain is the domain a person registers, judged by ICANN-managed suffixes only.
func registrableDomain(host string) (string, bool) {
	labels := strings.Split(strings.ToLower(strings.TrimSuffix(host, ".")), ".")
	for i := range labels {
		candidate := strings.Join(labels[i:], ".")
		if suffix, icann := publicsuffix.PublicSuffix(candidate); icann && suffix == candidate {
			if i == 0 {
				return "", false
			}
			return strings.Join(labels[i-1:], "."), true
		}
	}
	return "", false
}

func rdapEarliest(events []rdapEvent, action string) string {
	var best time.Time
	for _, e := range events {
		if e.Action != action {
			continue
		}
		if t, err := time.Parse(time.RFC3339, e.Date); err == nil && (best.IsZero() || t.Before(best)) {
			best = t
		}
	}
	if best.IsZero() {
		return ""
	}
	return isoTime(best)
}

type rdapEvent struct {
	Action string `json:"eventAction"`
	Date   string `json:"eventDate"`
}

type rdapDomain struct {
	Events   []rdapEvent `json:"events"`
	Entities []struct {
		Roles []string        `json:"roles"`
		VCard json.RawMessage `json:"vcardArray"`
	} `json:"entities"`
}

// rdapRegistrar finds the registrar's formatted name in the vCard of its entity.
func (d rdapDomain) registrar() string {
	for _, entity := range d.Entities {
		isRegistrar := false
		for _, role := range entity.Roles {
			isRegistrar = isRegistrar || role == "registrar"
		}
		var card []any
		if !isRegistrar || json.Unmarshal(entity.VCard, &card) != nil || len(card) < 2 {
			continue
		}
		items, _ := card[1].([]any)
		for _, item := range items {
			if fields, _ := item.([]any); len(fields) >= 4 && fields[0] == "fn" {
				if name, _ := fields[3].(string); strings.TrimSpace(name) != "" {
					return short(strings.TrimSpace(name), 200)
				}
			}
		}
	}
	return ""
}

type rdapBootstrap struct {
	Services [][][]string `json:"services"`
}

// domainLookup asks the registry that runs a domain's suffix first: its RDAP service (IANA is asked once a
// day which one that is), then CNNIC's WHOIS for .cn. Only when those have no date is who.ga asked.
type domainLookup struct {
	fetch     fetchFunc
	whois     func(ctx context.Context, domain string) whoisResult
	now       func() time.Time
	whogaBase string // tests point this at a local server; empty means api.who.ga

	mu      sync.Mutex
	boot    *rdapBootstrap
	expires time.Time
}

func (l *domainLookup) bootstrap(ctx context.Context) (*rdapBootstrap, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.boot != nil && l.now().Before(l.expires) {
		return l.boot, nil
	}
	status, body, err := l.fetch(ctx, &url.URL{Scheme: "https", Host: "data.iana.org", Path: "/rdap/dns.json"})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, siteCheckError("域名到期信息暂时不可用")
	}
	var boot rdapBootstrap
	if json.Unmarshal(body, &boot) != nil || len(boot.Services) == 0 {
		return nil, siteCheckError("域名到期信息格式无效")
	}
	l.boot, l.expires = &boot, l.now().Add(24*time.Hour)
	return l.boot, nil
}

func (l *domainLookup) inspect(ctx context.Context, hostname string) DomainExpiry {
	domain, ok := registrableDomain(hostname)
	if !ok {
		return DomainExpiry{Error: "无法识别可注册域名", Retryable: boolPtr(false)}
	}
	official := l.rdap(ctx, domain)
	if official.ExpiresAt != "" {
		official.Source = "rdap"
		return official
	}
	unsupported := official.Error == rdapUnsupported
	// .com.cn and other .cn names share CNNIC's official WHOIS service, which fills in when RDAP has nothing.
	if strings.HasSuffix(domain, ".cn") {
		result := l.whois(ctx, domain)
		if result.Error == "" {
			return DomainExpiry{Domain: domain, ExpiresAt: result.ExpiresAt, Retryable: boolPtr(false), Source: "cnnic"}
		}
		official = DomainExpiry{Domain: domain, Error: result.Error, Retryable: boolPtr(result.Retryable)}
		unsupported = false
	}
	fallback := l.whoga(ctx, domain)
	if fallback.ExpiresAt != "" {
		return fallback
	}
	// The registry's own answer explains a failure best, unless it has no service for the suffix at all.
	out := official
	if unsupported {
		out = fallback
	}
	out.Domain = domain
	if retryable(official) || retryable(fallback) {
		out.Retryable = nil
	}
	return out
}

const rdapUnsupported = "暂不支持该域名后缀的到期查询，请到注册商查看"

// retryable treats an unmarked failure as transient, like expiryRefreshDue does.
func retryable(d DomainExpiry) bool { return d.Error != "" && (d.Retryable == nil || *d.Retryable) }

func boolPtr(v bool) *bool { return &v }

func (l *domainLookup) rdap(ctx context.Context, domain string) DomainExpiry {
	tld := domain[strings.LastIndex(domain, ".")+1:]
	boot, err := l.bootstrap(ctx)
	if err != nil {
		return DomainExpiry{Domain: domain, Error: checkErrorMessage(err)}
	}
	endpoint := ""
	for _, service := range boot.Services {
		if len(service) != 2 || !listHas(service[0], tld) {
			continue
		}
		for _, candidate := range service[1] {
			if strings.HasPrefix(candidate, "https://") {
				endpoint = candidate
				break
			}
		}
		break
	}
	if endpoint == "" {
		return DomainExpiry{Domain: domain, Error: rdapUnsupported, Retryable: boolPtr(false)}
	}
	base, err := url.Parse(endpoint)
	if err != nil {
		return DomainExpiry{Domain: domain, Error: "域名到期信息格式无效"}
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	status, body, err := l.fetch(ctx, base.ResolveReference(&url.URL{Path: "domain/" + domain}))
	if err != nil {
		return DomainExpiry{Domain: domain, Error: checkErrorMessage(err)}
	}
	if status != http.StatusOK {
		return DomainExpiry{Domain: domain, Error: "域名到期信息暂时不可用"}
	}
	var payload rdapDomain
	if json.Unmarshal(body, &payload) != nil {
		return DomainExpiry{Domain: domain, Error: "域名到期信息格式无效"}
	}
	expires := rdapEarliest(payload.Events, "expiration")
	if expires == "" {
		return DomainExpiry{Domain: domain, Error: "查询服务未返回该域名的到期时间，请到注册商查看", Retryable: boolPtr(false)}
	}
	return DomainExpiry{Domain: domain, ExpiresAt: expires, RegisteredAt: rdapEarliest(payload.Events, "registration"), Registrar: payload.registrar(), Retryable: boolPtr(false)}
}

func listHas(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// siteChecker bundles the two network probes so tests can replace them.
type siteChecker struct {
	http   func(ctx context.Context, u *url.URL) HTTPCheck
	expiry func(ctx context.Context, host string) ExpirySnapshot
}

func newSiteChecker(allow []netip.Prefix) siteChecker {
	client := newSiteHTTPClient(allow)
	dialer := &net.Dialer{Timeout: 8 * time.Second, Control: outboundGuard(allow)}
	lookup := &domainLookup{now: time.Now, fetch: publicFetch(client, validatePublicURL)}
	lookup.whois = func(ctx context.Context, domain string) whoisResult {
		return inspectCnWhois(ctx, dialer.DialContext, domain)
	}
	return siteChecker{
		http: func(ctx context.Context, u *url.URL) HTTPCheck {
			if err := validatePublicURL(u); err != nil {
				return HTTPCheck{CheckedAt: isoTime(time.Now()), Error: checkErrorMessage(err)}
			}
			return probeHTTP(ctx, client, u, time.Now)
		},
		expiry: func(ctx context.Context, host string) ExpirySnapshot {
			return checkExpiry(ctx, lookup, dialer.DialContext, nil, host)
		},
	}
}

type fetchFunc func(ctx context.Context, u *url.URL) (int, []byte, error)

// publicFetch reads a lookup service's JSON answer through the site-check client, so RDAP and who.ga
// requests pass the same public-address checks as the site probes, both on the URL and at connect time.
func publicFetch(client *http.Client, validate func(*url.URL) error) fetchFunc {
	return func(ctx context.Context, u *url.URL) (int, []byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := validate(u); err != nil {
			return 0, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("User-Agent", "Plost-SiteCheck/1.0")
		req.Header.Set("Accept", "application/rdap+json, application/json")
		req.Header.Set("Accept-Encoding", "identity")
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		if err != nil {
			return 0, nil, err
		}
		if len(body) > 1<<20 {
			return 0, nil, siteCheckError("响应内容过大")
		}
		return resp.StatusCode, body, nil
	}
}

func checkExpiry(ctx context.Context, lookup *domainLookup, dial dialFunc, roots *x509.CertPool, host string) ExpirySnapshot {
	var domain DomainExpiry
	var cert CertificateResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); domain = lookup.inspect(ctx, host) }()
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		result, err := inspectCertificate(ctx, dial, roots, host, time.Now())
		if err != nil {
			result = CertificateResult{Error: checkErrorMessage(err)}
		}
		cert = result
	}()
	wg.Wait()
	return ExpirySnapshot{CheckedAt: isoTime(time.Now()), Lookup: expiryLookupVersion, Domain: domain, Certificate: cert}
}

// expiryRefreshDue: successful lookups live for a day; transient failures get another try in 15 minutes.
func expiryRefreshDue(s *ExpirySnapshot, now time.Time) bool {
	if s == nil {
		return true
	}
	if s.Lookup < expiryLookupVersion && s.Domain.ExpiresAt == "" {
		return true
	}
	retryable := s.Domain.Retryable == nil || *s.Domain.Retryable
	failed := (s.Domain.Error != "" && retryable) || s.Certificate.Error != ""
	checked, err := time.Parse(time.RFC3339, s.CheckedAt)
	if err != nil {
		return true
	}
	wait := 24 * time.Hour
	if failed {
		wait = 15 * time.Minute
	}
	return !now.Before(checked.Add(wait))
}

// mergeExpiry keeps each last observed date independently while retaining the latest error.
func mergeExpiry(previous *ExpirySnapshot, current ExpirySnapshot) ExpirySnapshot {
	var oldDomain DomainExpiry
	var oldCert CertificateResult
	prevChecked := ""
	if previous != nil {
		oldDomain, oldCert, prevChecked = previous.Domain, previous.Certificate, previous.CheckedAt
	}
	d := current.Domain
	if d.RegisteredAt == "" {
		d.RegisteredAt = oldDomain.RegisteredAt
	}
	if d.Registrar == "" {
		d.Registrar = oldDomain.Registrar
	}
	switch {
	case d.ExpiresAt != "":
		d.Stale, d.LastSuccessAt = false, current.CheckedAt
	case d.Error != "" && oldDomain.ExpiresAt != "":
		d.ExpiresAt, d.Stale, d.LastSuccessAt = oldDomain.ExpiresAt, true, firstNonEmpty(oldDomain.LastSuccessAt, prevChecked)
		d.Source = oldDomain.Source
	}
	c := current.Certificate
	if c.Validation == "" {
		c.Validation = oldCert.Validation
	}
	switch {
	case c.ExpiresAt != "":
		c.Stale, c.LastSuccessAt = false, current.CheckedAt
	case c.Error != "" && oldCert.ExpiresAt != "":
		c.ExpiresAt, c.Stale, c.LastSuccessAt = oldCert.ExpiresAt, true, firstNonEmpty(oldCert.LastSuccessAt, prevChecked)
	}
	return ExpirySnapshot{CheckedAt: current.CheckedAt, Lookup: current.Lookup, Domain: d, Certificate: c}
}
