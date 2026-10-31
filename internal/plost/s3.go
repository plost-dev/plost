package plost

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var errNotPublic = errors.New("storage address is not public")

var reservedRanges = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "100::/64"} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// isPublicAddress rejects loopback, private, link-local, shared, documentation and other non-routable ranges.
func isPublicAddress(ip netip.Addr) bool {
	ip = ip.Unmap().WithZone("")
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}
	for _, p := range reservedRanges {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// outboundGuard runs for the address actually being dialed, so a hostname that resolves to an internal
// address (or changes after validation) cannot be used. allow lists networks the operator has chosen to
// permit anyway, such as the fake-IP range some local proxy tools hand out for every name.
func outboundGuard(allow []netip.Prefix) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return errNotPublic
		}
		if isPublicAddress(ip) {
			return nil
		}
		ip = ip.Unmap().WithZone("")
		for _, network := range allow {
			if network.Contains(ip) {
				return nil
			}
		}
		return errNotPublic
	}
}

type s3Client struct {
	cfg  cloudSettings
	http *http.Client
	now  func() time.Time
}

func newS3Client(cfg cloudSettings, allow []netip.Prefix) *s3Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second, Control: outboundGuard(allow)}
	transport := &http.Transport{
		// Environment proxies would make the dial-time address check see the proxy instead of the storage host.
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
		DisableKeepAlives:     true,
	}
	return &s3Client{cfg: cfg, now: time.Now, http: &http.Client{Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type s3Error struct {
	Status int
	Code   string
}

func (e *s3Error) Error() string { return fmt.Sprintf("storage returned %d %s", e.Status, e.Code) }

type s3Object struct {
	Key      string
	Size     int64
	Modified time.Time
}

func awsEncode(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' || (c == '/' && keepSlash) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// target builds the request URL. Custom endpoints and R2 use path-style addressing;
// the default AWS endpoint uses virtual-hosted style unless the bucket name contains dots.
func (c *s3Client) target(key string, query map[string]string) (*url.URL, error) {
	var host, base string
	if c.cfg.Endpoint == "" {
		host = "s3." + c.cfg.Region + ".amazonaws.com"
		if strings.Contains(c.cfg.Bucket, ".") {
			base = "/" + c.cfg.Bucket
		} else {
			host = c.cfg.Bucket + "." + host
		}
	} else {
		u, err := url.Parse(c.cfg.Endpoint)
		if err != nil || u.Host == "" {
			return nil, errors.New("invalid endpoint")
		}
		host = u.Host
		base = strings.TrimRight(u.Path, "/") + "/" + c.cfg.Bucket
	}
	host = strings.TrimSuffix(host, ":443")
	path := base + "/" + key
	if key == "" {
		if path = base; path == "" {
			path = "/"
		}
	}
	pairs := make([]string, 0, len(query))
	for k, v := range query {
		pairs = append(pairs, awsEncode(k, false)+"="+awsEncode(v, false))
	}
	sort.Strings(pairs)
	return &url.URL{Scheme: "https", Host: host, Path: path, RawPath: awsEncode(path, true), RawQuery: strings.Join(pairs, "&")}, nil
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// signRequest adds an AWS Signature Version 4 Authorization header for the S3 service.
func signRequest(req *http.Request, payloadHash, access, secret, region string, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	day := stamp[:8]
	req.Header.Set("X-Amz-Date", stamp)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	values := map[string]string{"host": req.URL.Host}
	names := []string{"host"}
	for k, v := range req.Header {
		if lk := strings.ToLower(k); lk == "range" || lk == "content-type" || strings.HasPrefix(lk, "x-amz-") {
			names = append(names, lk)
			values[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	sort.Strings(names)
	var headers strings.Builder
	for _, n := range names {
		headers.WriteString(n + ":" + values[n] + "\n")
	}
	signed := strings.Join(names, ";")
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	canonical := strings.Join([]string{req.Method, path, req.URL.RawQuery, headers.String(), signed, payloadHash}, "\n")
	digest := sha256.Sum256([]byte(canonical))
	scope := day + "/" + region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(digest[:])
	key := hmacSHA256([]byte("AWS4"+secret), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+access+"/"+scope+", SignedHeaders="+signed+", Signature="+hex.EncodeToString(hmacSHA256(key, toSign)))
}

const emptyPayload = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func (c *s3Client) do(ctx context.Context, method, key string, query map[string]string, body io.Reader, size int64) (*http.Response, error) {
	u, err := c.target(key, query)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	payload := emptyPayload
	if body != nil {
		// Backups are streamed from disk over HTTPS, so the body is not hashed before sending.
		payload = "UNSIGNED-PAYLOAD"
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	region := c.cfg.Region
	if region == "" {
		region = "auto"
	}
	signRequest(req, payload, c.cfg.AccessKey, c.cfg.SecretKey, region, c.now())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		var e struct {
			Code string `xml:"Code"`
		}
		_ = xml.Unmarshal(raw, &e)
		return nil, &s3Error{Status: resp.StatusCode, Code: e.Code}
	}
	return resp, nil
}

// list returns up to max objects under prefix, or every object (bounded) when max is zero.
func (c *s3Client) list(ctx context.Context, prefix string, max int) ([]s3Object, error) {
	var out []s3Object
	token := ""
	for {
		query := map[string]string{"list-type": "2", "prefix": prefix, "max-keys": "1000"}
		if max > 0 {
			query["max-keys"] = strconv.Itoa(max)
		}
		if token != "" {
			query["continuation-token"] = token
		}
		resp, err := c.do(ctx, http.MethodGet, "", query, nil, 0)
		if err != nil {
			return nil, err
		}
		var page struct {
			Truncated bool   `xml:"IsTruncated"`
			Next      string `xml:"NextContinuationToken"`
			Contents  []struct {
				Key      string    `xml:"Key"`
				Size     int64     `xml:"Size"`
				Modified time.Time `xml:"LastModified"`
			} `xml:"Contents"`
		}
		err = xml.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			out = append(out, s3Object{Key: o.Key, Size: o.Size, Modified: o.Modified})
		}
		if max > 0 || !page.Truncated {
			return out, nil
		}
		if page.Next == "" || len(out) >= 5000 {
			return nil, errors.New("backup folder listing is too large or cannot be paged")
		}
		token = page.Next
	}
}

func (c *s3Client) put(ctx context.Context, key string, f *os.File, size int64) error {
	resp, err := c.do(ctx, http.MethodPut, key, nil, f, size)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return resp.Body.Close()
}

func (c *s3Client) get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, key, nil, nil, 0)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// cloudFailureMessage never forwards provider response bodies, request URLs or credential material.
func cloudFailureMessage(err error) string {
	var s3 *s3Error
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return "连接存储服务超时，请检查 Endpoint 或稍后重试"
	case errors.Is(err, errNotPublic):
		return "存储地址必须解析到公网地址"
	case errors.As(err, &dns):
		return "无法解析存储地址，请检查 Endpoint"
	case errors.As(err, &cert):
		return "存储地址的 HTTPS 证书验证失败"
	case errors.As(err, &s3):
		switch {
		case s3.Status == 401 || s3.Status == 403 || strings.Contains("AccessDenied InvalidAccessKeyId SignatureDoesNotMatch InvalidToken ExpiredToken", s3.Code) && s3.Code != "":
			return "认证或权限失败，请检查密钥和存储桶权限"
		case s3.Status == 404 || s3.Code == "NoSuchBucket":
			return "未找到存储桶或备份文件，请检查存储桶名称和 Endpoint"
		case s3.Status == 301 || s3.Status == 307 || strings.Contains("PermanentRedirect AuthorizationHeaderMalformed IllegalLocationConstraint", s3.Code) && s3.Code != "":
			return "存储桶区域不匹配，请检查 Region 和 Endpoint"
		}
	}
	return "无法连接对象存储，请检查 Endpoint、Region、存储桶和权限后重试"
}
