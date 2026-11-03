package plost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	fakeBucket = "test-bucket"
	fakeAccess = "TESTACCESSKEYID"
	fakeSecret = "test-only-storage-secret-must-not-leak"
)

// fakeS3 is a minimal path-style S3 endpoint. It checks the request shape, not the signature
// algorithm; the algorithm is pinned separately by AWS's published example.
type fakeS3 struct {
	srv     *httptest.Server
	mu      sync.Mutex
	objects map[string][]byte
	denyPut bool
	hits    int
}

func newFakeS3(t *testing.T) *fakeS3 {
	t.Helper()
	f := &fakeS3{objects: map[string][]byte{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func fail(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>secret detail %s</Message></Error>`, code, fakeSecret)
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits++
	auth := r.Header.Get("Authorization")
	_, signedList, _ := strings.Cut(auth, "SignedHeaders=")
	signedList, _, _ = strings.Cut(signedList, ",")
	signed := map[string]bool{}
	for _, name := range strings.Split(signedList, ";") {
		signed[name] = true
	}
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential="+fakeAccess+"/") || !signed["host"] || !signed["x-amz-content-sha256"] || !signed["x-amz-date"] || r.Header.Get("X-Amz-Date") == "" {
		fail(w, 403, "SignatureDoesNotMatch")
		return
	}
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if bucket != fakeBucket {
		fail(w, 404, "NoSuchBucket")
		return
	}
	switch {
	case r.Method == "PUT":
		if f.denyPut {
			fail(w, 403, "AccessDenied")
			return
		}
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Amz-Content-Sha256") != "UNSIGNED-PAYLOAD" || int64(len(body)) != r.ContentLength {
			fail(w, 400, "BadDigest")
			return
		}
		f.objects[key] = body
	case r.Method == "GET" && key == "":
		q := r.URL.Query()
		var keys []string
		for k := range f.objects {
			if strings.HasPrefix(k, q.Get("prefix")) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		start, _ := strconv.Atoi(q.Get("continuation-token"))
		size, _ := strconv.Atoi(q.Get("max-keys"))
		end := len(keys)
		if size > 0 && start+size < end {
			end = start + size
		}
		var out strings.Builder
		fmt.Fprintf(&out, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><IsTruncated>%t</IsTruncated>`, end < len(keys))
		if end < len(keys) {
			fmt.Fprintf(&out, "<NextContinuationToken>%d</NextContinuationToken>", end)
		}
		for _, k := range keys[min(start, len(keys)):end] {
			fmt.Fprintf(&out, "<Contents><Key>%s</Key><LastModified>2026-10-31T00:00:00.000Z</LastModified><Size>%d</Size></Contents>", k, len(f.objects[k]))
		}
		out.WriteString("</ListBucketResult>")
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, out.String())
	case r.Method == "GET":
		body, ok := f.objects[key]
		if !ok {
			fail(w, 404, "NoSuchKey")
			return
		}
		w.Write(body)
	default:
		fail(w, 405, "MethodNotAllowed")
	}
}

func (f *fakeS3) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeS3) settings(extra map[string]any) map[string]any {
	m := map[string]any{"provider": "s3", "endpoint": f.srv.URL, "region": "us-east-1", "bucket": fakeBucket, "prefix": "",
		"currentPassword": testPassword,
		"cloudEnabled":    true, "schedule": "off", "hour": 3, "minute": 0, "weekday": 0, "keep": 2}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// envBackupKeys stands in for the env file's backup credentials and password.
func (x *testApp) envBackupKeys() {
	x.app.cfg.BackupAccessKey, x.app.cfg.BackupSecretKey, x.app.cfg.BackupPassword = fakeAccess, fakeSecret, testBackupPassword
}

func (x *testApp) useFakeS3(f *fakeS3) {
	x.envBackupKeys()
	x.app.allowPrivateCloud = true
	x.app.newCloud = func(c cloudSettings) *s3Client { return &s3Client{cfg: c, http: f.srv.Client(), now: time.Now} }
}

// runAndWait starts a manual backup and waits for the single backup slot to free up.
func (x *testApp) runAndWait(t *testing.T) map[string]any {
	t.Helper()
	x.call(t, "POST", "/api/backup/run", map[string]any{}, 200)
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		var s map[string]any
		json.Unmarshal(x.call(t, "GET", "/api/backup/settings", nil, 200), &s)
		if s["running"] == false {
			return s
		}
	}
	t.Fatal("backup did not finish")
	return nil
}

func (x *testApp) restoreFrom(t *testing.T, route, source, confirm string, expected int) []byte {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	mw.WriteField("source", source)
	mw.WriteField("password", testBackupPassword)
	mw.WriteField("currentPassword", testPassword)
	mw.WriteField("confirmation", confirm)
	mw.Close()
	req, _ := http.NewRequest("POST", x.server.URL+route, &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Plost-Request", "1")
	req.Header.Set("Origin", x.server.URL)
	resp, err := x.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != expected {
		t.Fatalf("restore from %s: status %d, want %d: %s", source, resp.StatusCode, expected, out)
	}
	return out
}

func (x *testApp) localNames(t *testing.T) []string {
	t.Helper()
	var files []backupFile
	json.Unmarshal(x.call(t, "GET", "/api/backup/local", nil, 200), &files)
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}
	return names
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }

func TestSigV4MatchesAWSPublishedExample(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	signRequest(req, emptyPayload, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("signature mismatch\n got %s\nwant %s", got, want)
	}
}

func TestRequestTargetsAndEncoding(t *testing.T) {
	aws := &s3Client{cfg: cloudSettings{Region: "eu-west-1", Bucket: "my-bucket"}}
	u, _ := aws.target("plost/backups/a b+c.plostbk", nil)
	if u.Host != "my-bucket.s3.eu-west-1.amazonaws.com" || u.EscapedPath() != "/plost/backups/a%20b%2Bc.plostbk" {
		t.Fatalf("virtual-hosted target %s%s", u.Host, u.EscapedPath())
	}
	dotted := &s3Client{cfg: cloudSettings{Region: "eu-west-1", Bucket: "my.dotted.bucket"}}
	if u, _ = dotted.target("k", nil); u.Host != "s3.eu-west-1.amazonaws.com" || u.Path != "/my.dotted.bucket/k" {
		t.Fatal("dotted buckets must use path style so TLS names match")
	}
	r2 := &s3Client{cfg: cloudSettings{Endpoint: "https://abc.r2.cloudflarestorage.com", Bucket: "backups"}}
	if u, _ = r2.target("", map[string]string{"prefix": "p/", "list-type": "2"}); u.Host != "abc.r2.cloudflarestorage.com" || u.Path != "/backups" || u.RawQuery != "list-type=2&prefix=p%2F" {
		t.Fatalf("path-style list target %s %s?%s", u.Host, u.Path, u.RawQuery)
	}
}

func TestPublicAddressPolicy(t *testing.T) {
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !isPublicAddress(mustAddr(ip)) {
			t.Errorf("%s should be public", ip)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "172.16.0.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "255.255.255.255",
		"::1", "::", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "224.0.0.1", "192.0.2.1", "198.18.0.1", "2001:db8::1", "64:ff9b::7f00:1"} {
		if isPublicAddress(mustAddr(ip)) {
			t.Errorf("%s must not be reachable as a storage address", ip)
		}
	}
}

func TestEndpointValidation(t *testing.T) {
	for provider, endpoint := range map[string]string{"s3": "https://s3.example.com", "r2": "https://abc123.r2.cloudflarestorage.com"} {
		if _, err := validateEndpoint(provider, endpoint, false); err != nil {
			t.Errorf("%s %s rejected: %v", provider, endpoint, err)
		}
	}
	for _, tc := range [][2]string{
		{"s3", "http://s3.example.com"}, {"s3", "https://user:pw@s3.example.com"}, {"s3", "https://s3.example.com?x=1"},
		{"s3", "https://s3.example.com#frag"}, {"s3", "https://localhost"}, {"s3", "https://127.0.0.1"}, {"s3", "https://10.0.0.5:9000"},
		{"s3", "https://[::1]"}, {"s3", "https://store.internal"}, {"s3", "https://store.local"}, {"s3", "ftp://s3.example.com"},
		{"r2", "https://s3.example.com"}, {"r2", "https://abc.r2.cloudflarestorage.com:8443"}, {"r2", "https://abc.r2.cloudflarestorage.com/bucket"},
	} {
		if _, err := validateEndpoint(tc[0], tc[1], false); err == nil {
			t.Errorf("%s endpoint %q was accepted", tc[0], tc[1])
		}
	}
}

func TestDialRefusesNonPublicAddresses(t *testing.T) {
	f := newFakeS3(t)
	client := newS3Client(cloudSettings{Provider: "s3", Endpoint: f.srv.URL, Region: "us-east-1", Bucket: fakeBucket, AccessKey: fakeAccess, SecretKey: fakeSecret}, nil)
	_, err := client.list(context.Background(), "p/", 1)
	if err == nil {
		t.Fatal("loopback storage address was reachable")
	}
	if msg := cloudFailureMessage(err); msg != "存储地址必须解析到公网地址" {
		t.Fatalf("unexpected message %q (%v)", msg, err)
	}
	if f.hits != 0 {
		t.Fatal("request reached the loopback server")
	}
}

func TestBackupScheduleDueRules(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	if now.Weekday() != time.Wednesday {
		t.Fatal("fixture date changed")
	}
	at := func(d time.Time, h, m int) int64 {
		return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, time.UTC).Unix()
	}
	sunday := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	daily := backupSettings{Schedule: "daily", Hour: 3}
	weekly := backupSettings{Schedule: "weekly", Hour: 3, Weekday: 0}
	for _, tc := range []struct {
		name string
		s    backupSettings
		now  time.Time
		want bool
	}{
		{"daily before slot, never run", daily, now.Add(-10 * time.Hour), false},
		{"daily after slot, never run", daily, now, true},
		{"daily already ran today", backupSettings{Schedule: "daily", Hour: 3, LastRunAt: at(now, 3, 0) + 5}, now, false},
		{"daily ran yesterday", backupSettings{Schedule: "daily", Hour: 3, LastRunAt: at(now.AddDate(0, 0, -1), 3, 1)}, now, true},
		{"weekly this week done", backupSettings{Schedule: "weekly", Hour: 3, LastRunAt: at(sunday, 3, 30)}, now, false},
		{"weekly missed a week", backupSettings{Schedule: "weekly", Hour: 3, LastRunAt: at(sunday.AddDate(0, 0, -7), 3, 30)}, now, true},
		{"weekly on the day before the slot", weekly, time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC), true},
		{"weekly on the day before the slot, last week done", backupSettings{Schedule: "weekly", Hour: 3, LastRunAt: at(sunday.AddDate(0, 0, -7), 3, 30)}, time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC), false},
		{"hourly never run", backupSettings{Schedule: "hourly"}, now, true},
		{"hourly recent", backupSettings{Schedule: "hourly", LastRunAt: now.Add(-59 * time.Minute).Unix()}, now, false},
		{"hourly elapsed", backupSettings{Schedule: "hourly", LastRunAt: now.Add(-61 * time.Minute).Unix()}, now, true},
		{"off", backupSettings{Schedule: "off"}, now, false},
		{"retry backoff", backupSettings{Schedule: "hourly", LastStatus: "error", LastAttemptAt: now.Add(-5 * time.Minute).Unix()}, now, false},
		{"retry after backoff", backupSettings{Schedule: "hourly", LastStatus: "error", LastAttemptAt: now.Add(-20 * time.Minute).Unix()}, now, true},
	} {
		if got := backupDue(tc.now, tc.s); got != tc.want {
			t.Errorf("%s: due=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBackupSettingsAPI(t *testing.T) {
	f := newFakeS3(t)
	x := newTestApp(t)
	x.useFakeS3(f)
	for _, tc := range [][2]string{{"GET", "/api/backup/settings"}, {"PUT", "/api/backup/settings"}, {"POST", "/api/backup/cloud/test"}, {"POST", "/api/backup/cloud/scan"},
		{"POST", "/api/backup/run"}, {"GET", "/api/backup/local"}, {"GET", "/api/backup/local/plost-go-20261031T000000Z-00000000.plostbk"}} {
		x.call(t, tc[0], tc[1], map[string]any{}, 401)
	}
	x.restoreFrom(t, "/api/backup/validate", "local:plost-go-20261031T000000Z-00000000.plostbk", "", 401)
	x.setup(t)

	var fresh map[string]any
	json.Unmarshal(x.call(t, "GET", "/api/backup/settings", nil, 200), &fresh)
	if fresh["schedule"] != "off" || fresh["hasBackupPassword"] != true || fresh["cloudConfigured"] != false {
		t.Fatalf("unexpected defaults %v", fresh)
	}

	bad := func(extra map[string]any) { x.call(t, "PUT", "/api/backup/settings", f.settings(extra), 400) }
	bad(map[string]any{"currentPassword": "wrong-account-password"})
	bad(map[string]any{"schedule": "monthly"})
	bad(map[string]any{"hour": 24})
	bad(map[string]any{"keep": 91})
	// Keys and the backup password only come from the env file.
	bad(map[string]any{"backupPassword": "a-new-backup-password"})
	bad(map[string]any{"accessKey": "NEWKEY"})
	bad(map[string]any{"secretKey": "NEWSECRET"})
	bad(map[string]any{"endpoint": "http://insecure.example.com"})
	bad(map[string]any{"bucket": "Bad_Bucket"})
	bad(map[string]any{"prefix": "a/../b"})
	bad(map[string]any{"schedule": "daily", "keep": 0, "cloudEnabled": false})
	bad(map[string]any{"cloudEnabled": true, "bucket": ""})
	// Every attempt, even an invalid one, costs a password check and counts toward the limit.
	x.app.limits = newLimiter()

	body := x.call(t, "PUT", "/api/backup/settings", f.settings(map[string]any{"schedule": "daily", "hour": 4, "minute": 30, "weekday": 2}), 200)
	for _, secret := range []string{fakeSecret, fakeAccess, testBackupPassword} {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatal("settings response exposes a secret")
		}
	}
	var saved map[string]any
	json.Unmarshal(body, &saved)
	if saved["hasAccessKey"] != true || saved["hasSecretKey"] != true || saved["hasBackupPassword"] != true || saved["cloudConfigured"] != true || saved["effectivePrefix"] != "plost" || saved["schedule"] != "daily" {
		t.Fatalf("unexpected saved settings %v", saved)
	}

	var ak, sk, pw string
	if err := x.app.db.QueryRow("SELECT access_key,secret_key,backup_password FROM backup_settings").Scan(&ak, &sk, &pw); err != nil || ak+sk+pw != "" {
		t.Fatal("the database holds backup credentials", err)
	}
	for _, name := range []string{"plost.db", "plost.db-wal"} {
		raw, _ := os.ReadFile(filepath.Join(x.app.cfg.DataDir, name))
		for _, secret := range []string{fakeSecret, fakeAccess, testBackupPassword} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatalf("%s contains a secret", name)
			}
		}
	}
	// Without the env file's password a schedule cannot be saved.
	x.app.cfg.BackupPassword = ""
	bad(map[string]any{"schedule": "daily"})
}

func TestCloudTestReportsSafeErrors(t *testing.T) {
	f := newFakeS3(t)
	x := newTestApp(t)
	x.useFakeS3(f)
	x.setup(t)
	x.call(t, "PUT", "/api/backup/settings", f.settings(nil), 200)

	probe := func(extra map[string]any, status int, message string) {
		t.Helper()
		in := f.settings(extra)
		for _, k := range []string{"backupPassword", "currentPassword", "cloudEnabled", "schedule", "hour", "minute", "weekday", "keep"} {
			delete(in, k)
		}
		body := x.call(t, "POST", "/api/backup/cloud/test", in, status)
		for _, leak := range []string{fakeSecret, "127.0.0.1", "secret detail"} {
			if bytes.Contains(body, []byte(leak)) {
				t.Fatalf("error response leaks %q: %s", leak, body)
			}
		}
		if message != "" && !bytes.Contains(body, []byte(message)) {
			t.Fatalf("want message %q, got %s", message, body)
		}
	}
	probe(map[string]any{"accessKey": "", "secretKey": ""}, 200, `"verified":["list"]`)
	probe(map[string]any{"accessKey": "", "secretKey": "", "bucket": "missing-bucket"}, 502, "未找到存储桶")
	probe(map[string]any{"accessKey": "WRONGKEYID", "secretKey": "wrong"}, 502, "认证或权限失败")
	probe(map[string]any{"accessKey": "", "secretKey": "", "endpoint": "http://plain.example.com"}, 400, "")
	// Saved credentials are never sent to a different endpoint.
	probe(map[string]any{"accessKey": "", "secretKey": "", "endpoint": "https://elsewhere.example.com"}, 400, "重新填写")
}

func TestCloudBackupEndToEnd(t *testing.T) {
	f := newFakeS3(t)
	x := newTestApp(t)
	x.useFakeS3(f)
	x.setup(t)
	x.addSite(t, "example.com")
	x.event(t, "example.com", "persist-visitor-123", "/saved")
	x.call(t, "PUT", "/api/backup/settings", f.settings(map[string]any{"keep": 2}), 200)

	status := x.runAndWait(t)
	if status["lastStatus"] != "manual" || status["lastError"] != "" || status["lastBackup"] == "" {
		t.Fatalf("manual run did not succeed: %v", status)
	}
	names := x.localNames(t)
	if len(names) != 1 || names[0] != status["lastBackup"] || !backupNameExpr.MatchString(names[0]) {
		t.Fatalf("unexpected local backups %v", names)
	}
	key := "plost/backups/" + names[0]
	if f.count("plost/backups/") != 1 {
		t.Fatal("backup was not uploaded")
	}
	// The uploaded object is a real, restorable encrypted backup.
	var plain bytes.Buffer
	if err := decryptBackup(&plain, bytes.NewReader(f.objects[key]), testBackupPassword); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored.db")
	os.WriteFile(restored, plain.Bytes(), 0600)
	if err := validateDatabase(context.Background(), restored); err != nil {
		t.Fatalf("uploaded backup is not a valid database: %v", err)
	}
	if decryptBackup(io.Discard, bytes.NewReader(f.objects[key]), "not-the-backup-password") == nil {
		t.Fatal("backup decrypts with the wrong password")
	}

	var scanned []backupFile
	json.Unmarshal(x.call(t, "POST", "/api/backup/cloud/scan", map[string]any{}, 200), &scanned)
	if len(scanned) != 1 || scanned[0].Name != names[0] {
		t.Fatalf("cloud scan returned %v", scanned)
	}

	// Local retention prunes; the cloud copy is never deleted automatically.
	for i := 0; i < 3; i++ {
		x.runAndWait(t)
	}
	if got := len(x.localNames(t)); got != 2 {
		t.Fatalf("kept %d local backups, want 2", got)
	}
	if f.count("plost/backups/") != 4 {
		t.Fatalf("cloud holds %d backups, want 4", f.count("plost/backups/"))
	}

	// A listed local download is the same encrypted file.
	latest := x.localNames(t)[0]
	file := x.call(t, "GET", "/api/backup/local/"+latest, nil, 200)
	if !bytes.HasPrefix(file, []byte(backupMagic)) {
		t.Fatal("local download is not a backup")
	}
	x.call(t, "GET", "/api/backup/local/"+strings.ReplaceAll("../plost.db", "/", "%2F"), nil, 404)

	// Restore straight from the cloud listing.
	x.addSite(t, "after-backup.com")
	cloudName := scanned[0].Name
	x.restoreFrom(t, "/api/backup/validate", "cloud:"+cloudName, "", 200)
	x.restoreFrom(t, "/api/backup/restore", "cloud:"+cloudName, "", 400)
	x.restoreFrom(t, "/api/backup/restore", "cloud:"+cloudName, "RESTORE", 200)
	x.call(t, "GET", "/api/sites", nil, 401)
	x.call(t, "POST", "/api/login", map[string]string{"email": "owner@example.test", "password": testPassword}, 200)
	var sites []map[string]any
	json.Unmarshal(x.call(t, "GET", "/api/sites", nil, 200), &sites)
	if len(sites) != 1 {
		t.Fatalf("restore from cloud left %d sites, want 1", len(sites))
	}
	var after map[string]any
	json.Unmarshal(x.call(t, "GET", "/api/backup/settings", nil, 200), &after)
	if after["hasBackupPassword"] != true || after["hasAccessKey"] != true {
		t.Fatalf("restore lost readable backup settings: %v", after)
	}

	// Restore from a local scheduled file, and refuse names that are not ours.
	x.app.limits = newLimiter()
	x.restoreFrom(t, "/api/backup/validate", "local:"+latest, "", 200)
	x.restoreFrom(t, "/api/backup/validate", "local:../../plost.db", "", 400)
	x.restoreFrom(t, "/api/backup/validate", "cloud:../plost/backups/x", "", 400)
	x.restoreFrom(t, "/api/backup/validate", "ftp:"+latest, "", 400)
}

func TestFailedUploadIsReportedWithoutLeaks(t *testing.T) {
	f := newFakeS3(t)
	f.denyPut = true
	x := newTestApp(t)
	x.useFakeS3(f)
	x.setup(t)
	x.call(t, "PUT", "/api/backup/settings", f.settings(map[string]any{"keep": 1}), 200)
	status := x.runAndWait(t)
	message, _ := status["lastError"].(string)
	if status["lastStatus"] != "error" || !strings.Contains(message, "上传到云存储失败") || !strings.Contains(message, "认证或权限失败") {
		t.Fatalf("failure not reported: %v", status)
	}
	for _, leak := range []string{fakeSecret, "127.0.0.1", "secret detail", testBackupPassword} {
		if strings.Contains(message, leak) {
			t.Fatalf("stored error leaks %q", leak)
		}
	}
	if len(x.localNames(t)) != 1 {
		t.Fatal("the local copy should be kept when only the upload fails")
	}
	// Keeping zero local copies and failing to upload must not leave an orphan behind.
	x.call(t, "PUT", "/api/backup/settings", f.settings(map[string]any{"keep": 0}), 200)
	x.runAndWait(t)
	if len(x.localNames(t)) != 0 {
		t.Fatal("local files remain although retention is zero")
	}
}

func TestScheduledTickRunsOnceAndPrunes(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.envBackupKeys()
	x.call(t, "PUT", "/api/backup/settings", map[string]any{"provider": "r2", "currentPassword": testPassword,
		"cloudEnabled": false, "schedule": "hourly", "keep": 2}, 200)
	now := time.Now()
	x.app.scheduleTick(now)
	if len(x.localNames(t)) != 1 {
		t.Fatal("due schedule did not create a backup")
	}
	var s map[string]any
	json.Unmarshal(x.call(t, "GET", "/api/backup/settings", nil, 200), &s)
	if s["lastStatus"] != "schedule" || s["running"] != false {
		t.Fatalf("unexpected status %v", s)
	}
	x.app.scheduleTick(now.Add(10 * time.Minute))
	if len(x.localNames(t)) != 1 {
		t.Fatal("schedule ran again within the hour")
	}
	x.app.scheduleTick(now.Add(61 * time.Minute))
	x.app.scheduleTick(now.Add(130 * time.Minute))
	x.app.scheduleTick(now.Add(200 * time.Minute))
	if got := len(x.localNames(t)); got != 2 {
		t.Fatalf("retention kept %d, want 2", got)
	}
	// Only our own files are ever pruned.
	foreign := filepath.Join(x.app.scheduledDir(), "notes.txt")
	os.WriteFile(foreign, []byte("keep me"), 0600)
	pruneLocal(x.app.scheduledDir(), 0)
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("pruning removed a file it does not own")
	}
}

func TestOneBackupAtATimeAndShutdown(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.envBackupKeys()
	x.call(t, "PUT", "/api/backup/settings", map[string]any{"provider": "r2", "currentPassword": testPassword,
		"cloudEnabled": false, "schedule": "off", "keep": 1}, 200)
	if !x.app.beginBackup() {
		t.Fatal("slot should be free")
	}
	x.call(t, "POST", "/api/backup/run", map[string]any{}, 409)
	x.app.backupRunning.Store(false)
	x.runAndWait(t)

	// After Close begins, no new job may start.
	x.app.jobsMu.Lock()
	x.app.closing = true
	x.app.jobsMu.Unlock()
	x.call(t, "POST", "/api/backup/run", map[string]any{}, 503)
	if x.app.backupRunning.Load() {
		t.Fatal("slot leaked after a refused start")
	}
	x.app.jobsMu.Lock()
	x.app.closing = false
	x.app.jobsMu.Unlock()
}
