package plost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	whoisHost  = "whois.cnnic.cn"
	whoisLimit = 64 * 1024
)

type whoisResult struct {
	ExpiresAt string
	Error     string
	Retryable bool
}

func whoisFailure(message string, retryable bool) whoisResult {
	return whoisResult{Error: message, Retryable: retryable}
}

var (
	cnLabelPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	cnTimePattern   = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})(Z|[+-]\d{2}:?\d{2})?$`)
	whoisLimited    = regexp.MustCompile(`(?i)quota\s+exceeded|rate[ -]?limit|too many (?:queries|requests)|access denied|queries?[^\r\n]*limit[^\r\n]*exceeded`)
	whoisBusy       = regexp.MustCompile(`(?i)system is busy|try again later|temporarily unavailable|service unavailable`)
	whoisNoRecord   = regexp.MustCompile(`(?i)no matching record|the info does not exist|no match for|no entries found`)
	whoisDomainName = regexp.MustCompile(`(?im)^Domain Name:[ \t]*([^\r\n]*)`)
	whoisExpiration = regexp.MustCompile(`(?im)^Expiration Time:[ \t]*([^\r\n]*)`)
)

// cnDomain returns the registrable .cn domain, or "" when the value is not one.
func cnDomain(value string) string {
	for i := 0; i < len(value); i++ {
		if value[i] >= 0x80 {
			return ""
		}
	}
	domain := strings.ToLower(value)
	if len(domain) > 253 || !strings.HasSuffix(domain, ".cn") {
		return ""
	}
	for _, label := range strings.Split(domain, ".") {
		if !cnLabelPattern.MatchString(label) {
			return ""
		}
	}
	if registrable, ok := registrableDomain(domain); !ok || registrable != domain {
		return ""
	}
	return domain
}

// cnExpirationDate reads CNNIC's plain timestamp. It documents no timezone, so a timestamp without one keeps
// only the published calendar date instead of inventing a precise instant.
func cnExpirationDate(value string) string {
	m := cnTimePattern.FindStringSubmatch(value)
	if m == nil {
		return ""
	}
	n := make([]int, 6)
	for i := range n {
		n[i], _ = strconv.Atoi(m[i+1])
	}
	year, month, day, hour, minute, second := n[0], n[1], n[2], n[3], n[4], n[5]
	if year < 1900 || month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || second > 59 {
		return ""
	}
	wall := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	if wall.Year() != year || int(wall.Month()) != month || wall.Day() != day {
		return ""
	}
	zone := m[7]
	if zone == "" {
		return fmt.Sprintf("%s-%s-%s", m[1], m[2], m[3])
	}
	offset := 0
	if zone != "Z" {
		compact := strings.ReplaceAll(zone, ":", "")
		hours, _ := strconv.Atoi(compact[1:3])
		minutes, _ := strconv.Atoi(compact[3:5])
		if hours > 14 || minutes > 59 || (hours == 14 && minutes != 0) {
			return ""
		}
		offset = hours*60 + minutes
		if zone[0] == '-' {
			offset = -offset
		}
	}
	return isoTime(wall.Add(-time.Duration(offset) * time.Minute))
}

// parseCnWhois reads only the requested record's explicit expiration and never keeps contact data.
func parseCnWhois(domain, response string) whoisResult {
	expected := cnDomain(domain)
	if expected == "" {
		return whoisFailure("WHOIS 兜底仅支持有效的 .cn 可注册域名", false)
	}
	if len(response) > whoisLimit {
		return whoisFailure("WHOIS 响应内容过大，未采用到期信息，请到注册商查看", true)
	}
	if whoisLimited.MatchString(response) {
		return whoisFailure("WHOIS 查询受限，请稍后重试或到注册商查看", true)
	}
	if whoisBusy.MatchString(response) {
		return whoisFailure("WHOIS 服务暂时繁忙，请稍后重试或到注册商查看", true)
	}
	var names []string
	for _, m := range whoisDomainName.FindAllStringSubmatch(response, -1) {
		names = append(names, strings.ToLower(strings.TrimSpace(m[1])))
	}
	if len(names) == 0 {
		if whoisNoRecord.MatchString(response) {
			return whoisFailure("WHOIS 未返回该域名的记录，请到注册商查看", false)
		}
		return whoisFailure("WHOIS 返回的信息格式无效，请稍后重试或到注册商查看", true)
	}
	if len(names) != 1 || names[0] != expected {
		return whoisFailure("WHOIS 返回的域名不匹配，未采用到期信息，请到注册商查看", true)
	}
	var dates []string
	for _, m := range whoisExpiration.FindAllStringSubmatch(response, -1) {
		dates = append(dates, strings.TrimSpace(m[1]))
	}
	if len(dates) == 0 {
		return whoisFailure("WHOIS 未返回该域名的到期时间，请到注册商查看", false)
	}
	expires := ""
	if len(dates) == 1 {
		expires = cnExpirationDate(dates[0])
	}
	if expires == "" {
		return whoisFailure("WHOIS 返回的到期时间格式无效，请到注册商查看", true)
	}
	return whoisResult{ExpiresAt: expires}
}

func whoisTransportFailure(err error) whoisResult {
	var dns *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return whoisFailure("WHOIS 查询超时，请稍后重试或到注册商查看", true)
	case errors.As(err, &dns):
		return whoisFailure("WHOIS 服务域名解析失败，请稍后重试", true)
	case errors.Is(err, errNotPublic):
		return whoisFailure("WHOIS 服务地址未通过公网安全校验，请稍后重试", true)
	}
	return whoisFailure("无法连接 WHOIS 服务，当前网络可能限制 TCP 43 端口，请稍后重试或到注册商查看", true)
}

// inspectCnWhois talks to one fixed registry endpoint: no user-chosen servers and no referrals.
func inspectCnWhois(ctx context.Context, dial dialFunc, domain string) whoisResult {
	query := cnDomain(domain)
	if query == "" {
		return whoisFailure("WHOIS 兜底仅支持有效的 .cn 可注册域名", false)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := dial(ctx, "tcp", net.JoinHostPort(whoisHost, "43"))
	if err != nil {
		return whoisTransportFailure(err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err = io.WriteString(conn, query+"\r\n"); err != nil {
		return whoisTransportFailure(err)
	}
	// A valid-looking prefix is not accepted before the server has finished its answer.
	body, err := io.ReadAll(io.LimitReader(conn, whoisLimit+1))
	if err != nil {
		return whoisTransportFailure(err)
	}
	return parseCnWhois(query, string(body))
}
