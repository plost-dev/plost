package plost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Edge cases that complement backup_cloud_test.go: address building, endpoint normalisation,
// error wording, credential inheritance, local-zone scheduling and partial-file cleanup.

func TestAddressingEdgeCases(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  cloudSettings
		key  string
		want string
	}{
		{"aws virtual host", cloudSettings{Region: "eu-west-1", Bucket: "my-bucket"}, "p/backups/a b.plostbk", "https://my-bucket.s3.eu-west-1.amazonaws.com/p/backups/a%20b.plostbk"},
		{"custom endpoint with unicode key", cloudSettings{Endpoint: "https://minio.example.com:9000", Bucket: "b"}, "é/x", "https://minio.example.com:9000/b/%C3%A9/x"},
		{"endpoint base path", cloudSettings{Endpoint: "https://example.com/s3", Bucket: "b"}, "k", "https://example.com/s3/b/k"},
		{"explicit default port is dropped", cloudSettings{Endpoint: "https://example.com:443", Bucket: "b"}, "k", "https://example.com/b/k"},
		{"listing has no trailing slash", cloudSettings{Endpoint: "https://example.com", Bucket: "b"}, "", "https://example.com/b"},
		{"virtual host listing", cloudSettings{Region: "us-east-1", Bucket: "b"}, "", "https://b.s3.us-east-1.amazonaws.com/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, err := (&s3Client{cfg: tc.cfg}).target(tc.key, nil)
			if err != nil {
				t.Fatal(err)
			}
			if u.String() != tc.want {
				t.Fatalf("got %s, want %s", u, tc.want)
			}
		})
	}
	u, _ := (&s3Client{cfg: cloudSettings{Endpoint: "https://example.com", Bucket: "b"}}).target("", map[string]string{"prefix": "a b/é", "list-type": "2"})
	if u.RawQuery != "list-type=2&prefix=a%20b%2F%C3%A9" {
		t.Fatalf("canonical query is %q", u.RawQuery)
	}
}

func TestEndpointNormalization(t *testing.T) {
	for _, tc := range []struct {
		provider, value, want string
		ok                    bool
	}{
		{"r2", "", "", true},
		{"r2", "https://abc123.r2.cloudflarestorage.com/", "https://abc123.r2.cloudflarestorage.com", true},
		{"r2", "https://abc123.eu.r2.cloudflarestorage.com", "https://abc123.eu.r2.cloudflarestorage.com", true},
		{"s3", "https://S3.Example.COM/", "https://s3.example.com", true},
		{"s3", "https://s3.example.com:9000/base", "https://s3.example.com:9000/base", true},
		{"s3", "https://s3.example.com//double", "", false},
		{"s3", "https://169.254.169.254", "", false},
		{"s3", "https://100.64.0.1", "", false},
		{"s3", "https://[::ffff:127.0.0.1]", "", false},
		{"s3", "https://nas.local", "", false},
	} {
		got, err := validateEndpoint(tc.provider, tc.value, false)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("%s %q: got %q, %v; want %q ok=%v", tc.provider, tc.value, got, err, tc.want, tc.ok)
		}
	}
	if got, err := validateEndpoint("s3", "https://127.0.0.1:9000", true); err != nil || got != "https://127.0.0.1:9000" {
		t.Fatalf("the test seam should accept a loopback origin: %q %v", got, err)
	}
}

func TestCloudFailureMessagesHideProviderDetails(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&s3Error{Status: 403, Code: "SignatureDoesNotMatch"}, "认证或权限失败，请检查密钥和存储桶权限"},
		{&s3Error{Status: 401}, "认证或权限失败，请检查密钥和存储桶权限"},
		{&s3Error{Status: 404, Code: "NoSuchBucket"}, "未找到存储桶或备份文件，请检查存储桶名称和 Endpoint"},
		{&s3Error{Status: 301, Code: "PermanentRedirect"}, "存储桶区域不匹配，请检查 Region 和 Endpoint"},
		{&s3Error{Status: 500, Code: "InternalError"}, "无法连接对象存储，请检查 Endpoint、Region、存储桶和权限后重试"},
		{context.DeadlineExceeded, "连接存储服务超时，请检查 Endpoint 或稍后重试"},
		{errors.New("Get \"https://bucket.example.com/key?X-Amz-Signature=abc\": boom"), "无法连接对象存储，请检查 Endpoint、Region、存储桶和权限后重试"},
	} {
		if got := cloudFailureMessage(tc.err); got != tc.want || strings.Contains(got, "http") {
			t.Errorf("%v: got %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestResolveCloudCredentialRules(t *testing.T) {
	saved := cloudSettings{Provider: "s3", Region: "us-east-1", Bucket: "saved-bucket", AccessKey: "SAVEDAK", SecretKey: "SAVEDSK"}
	base := cloudInput{Provider: "s3", Region: "us-east-1", Bucket: "saved-bucket"}
	with := func(f func(*cloudInput)) cloudInput { in := base; f(&in); return in }

	got, err := resolveCloud(base, saved, false)
	if err != nil || got.AccessKey != "SAVEDAK" || got.SecretKey != "SAVEDSK" {
		t.Fatalf("same destination should keep saved credentials: %+v %v", got, err)
	}
	got, err = resolveCloud(with(func(in *cloudInput) { in.AccessKey = "NEWAK" }), saved, false)
	if err != nil || got.AccessKey != "NEWAK" || got.SecretKey != "SAVEDSK" {
		t.Fatalf("a single replaced key should keep the other: %+v %v", got, err)
	}
	for name, in := range map[string]cloudInput{
		"region change": with(func(in *cloudInput) { in.Region = "eu-west-1" }),
		"endpoint set":  with(func(in *cloudInput) { in.Endpoint = "https://s3.example.com" }),
		"provider":      {Provider: "r2", Bucket: "saved-bucket", Endpoint: "https://abc.r2.cloudflarestorage.com"},
	} {
		if _, err := resolveCloud(in, saved, false); err == nil || !strings.Contains(err.Error(), "重新填写") {
			t.Errorf("%s: saved credentials must not follow a new destination (%v)", name, err)
		}
	}
	in := with(func(in *cloudInput) { in.Region, in.AccessKey, in.SecretKey = "eu-west-1", "NEWAK", "NEWSK" })
	if got, err = resolveCloud(in, saved, false); err != nil || got.AccessKey != "NEWAK" || got.SecretKey != "NEWSK" {
		t.Fatalf("new destination with new credentials: %+v %v", got, err)
	}
	if got, err = resolveCloud(cloudInput{Provider: "r2", Endpoint: "https://abc.r2.cloudflarestorage.com", Bucket: "bucket"}, cloudSettings{}, false); err != nil || got.Region != "auto" {
		t.Fatalf("r2 should default to the auto region: %+v %v", got, err)
	}
	if got, err = resolveCloud(with(func(in *cloudInput) { in.Prefix = " /a/b/ " }), saved, false); err != nil || got.Prefix != "a/b" {
		t.Fatalf("prefix should be trimmed: %+v %v", got, err)
	}
	for name, mutate := range map[string]func(*cloudInput){
		"parent segment":  func(in *cloudInput) { in.Prefix = "a/../b" },
		"empty segment":   func(in *cloudInput) { in.Prefix = "a//b" },
		"control char":    func(in *cloudInput) { in.Prefix = "a\x00b" },
		"bad bucket":      func(in *cloudInput) { in.Bucket = "Bad_Bucket" },
		"bad region":      func(in *cloudInput) { in.Region = "US East" },
		"bad provider":    func(in *cloudInput) { in.Provider = "gcs" },
		"space in key":    func(in *cloudInput) { in.AccessKey = "has space" },
		"newline in key":  func(in *cloudInput) { in.SecretKey = "line\nbreak" },
		"insecure origin": func(in *cloudInput) { in.Endpoint = "http://s3.example.com" },
	} {
		if _, err := resolveCloud(with(mutate), saved, false); err == nil {
			t.Errorf("%s: invalid input accepted", name)
		}
	}
}

// The schedule is evaluated in the server's zone, which is rarely UTC.
func TestBackupDueInServerZone(t *testing.T) {
	zone := time.FixedZone("test", 8*3600)
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 10, day, hour, minute, 0, 0, zone) }
	if at(28, 12, 0).Weekday() != time.Wednesday {
		t.Fatal("test calendar is wrong")
	}
	last := func(when time.Time) backupSettings {
		return backupSettings{LastRunAt: when.Unix(), LastAttemptAt: when.Unix(), LastStatus: "schedule"}
	}
	daily := func(s backupSettings) backupSettings { s.Schedule, s.Hour = "daily", 3; return s }
	weekly := func(s backupSettings) backupSettings { s.Schedule, s.Hour = "weekly", 3; return s }
	for _, tc := range []struct {
		name string
		now  time.Time
		s    backupSettings
		want bool
	}{
		{"before the slot", at(28, 2, 59), daily(last(at(27, 3, 0))), false},
		{"at the slot", at(28, 3, 0), daily(last(at(27, 3, 1))), true},
		{"first run waits for the slot", at(28, 2, 0), daily(backupSettings{}), false},
		{"first run after the slot", at(28, 4, 0), daily(backupSettings{}), true},
		{"catches up after downtime", at(28, 15, 0), daily(last(at(26, 3, 1))), true},
		{"weekly done since the slot", at(28, 12, 0), weekly(last(at(25, 3, 5))), false},
		{"weekly overdue", at(28, 12, 0), weekly(last(at(18, 3, 5))), true},
		{"weekly on its day, before the time", at(25, 2, 0), weekly(last(at(18, 3, 5))), false},
		{"weekly on its day, after the time", at(25, 4, 0), weekly(last(at(18, 3, 5))), true},
	} {
		if got := backupDue(tc.now, tc.s); got != tc.want {
			t.Errorf("%s: due=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPruneLocalHandlesPartialFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, age time.Duration) {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-age)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"plost-go-20261029T030000Z-aaaaaaaa.plostbk", "plost-go-20261030T030000Z-bbbbbbbb.plostbk", "plost-go-20261031T030000Z-cccccccc.plostbk", "plost-go-20261101T030000Z.plostbk"} {
		write(name, time.Minute)
	}
	write("notes.txt", time.Minute)
	write("before-restore-20261031T030000-aaaa.db", time.Minute)
	write("plost-go-20261031T030000Z-dddddddd.plostbk.part", 3*time.Hour)
	write("plost-go-20261031T040000Z-eeeeeeee.plostbk.part", time.Minute)

	pruneLocal(dir, 2)
	left := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		left[e.Name()] = true
	}
	for _, name := range []string{"plost-go-20261101T030000Z.plostbk", "plost-go-20261031T030000Z-cccccccc.plostbk", "notes.txt", "before-restore-20261031T030000-aaaa.db", "plost-go-20261031T040000Z-eeeeeeee.plostbk.part"} {
		if !left[name] {
			t.Errorf("%s should have been kept", name)
		}
	}
	for _, name := range []string{"plost-go-20261029T030000Z-aaaaaaaa.plostbk", "plost-go-20261030T030000Z-bbbbbbbb.plostbk", "plost-go-20261031T030000Z-dddddddd.plostbk.part"} {
		if left[name] {
			t.Errorf("%s should have been removed", name)
		}
	}
	pruneLocal(dir, 0)
	if entries, _ = os.ReadDir(dir); len(entries) != 3 {
		t.Fatalf("keep=0 should remove every finished scheduled backup, %d entries remain", len(entries))
	}
}
