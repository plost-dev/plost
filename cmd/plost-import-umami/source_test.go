package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/plost-dev/plost/internal/plost"
)

// These tests need a throwaway PostgreSQL. Point UMAMI_TEST_DATABASE_URL at an empty scratch database,
// for example postgres://tester@127.0.0.1:54399/umami_test?sslmode=disable. Each test works in its own
// schema and drops it afterwards. Without the variable they are skipped.

const (
	w1, w2, w3 = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"
	signupID   = "99999999-9999-4999-8999-999999999999"
)

func scratch(t *testing.T) (admin *pgx.Conn, schema, dsn string) {
	t.Helper()
	base := os.Getenv("UMAMI_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("UMAMI_TEST_DATABASE_URL is not set")
	}
	suffix := make([]byte, 4)
	rand.Read(suffix)
	schema = "umami_" + hex.EncodeToString(suffix)
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema+"; SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(ctx)
	})
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return admin, schema, base + sep + "search_path=" + schema
}

const umamiTables = `
CREATE TABLE website (website_id uuid PRIMARY KEY, name varchar(100) NOT NULL, domain varchar(500), share_id varchar(50), created_at timestamptz DEFAULT now(), deleted_at timestamptz);
CREATE TABLE session (session_id uuid PRIMARY KEY, website_id uuid NOT NULL, browser varchar(20), os varchar(20), device varchar(20), screen varchar(11),
  language varchar(35), country char(2), region varchar(20), city varchar(50), created_at timestamptz DEFAULT now());
CREATE TABLE website_event (event_id uuid PRIMARY KEY, website_id uuid NOT NULL, session_id uuid NOT NULL, visit_id uuid, created_at timestamptz DEFAULT now(),
  url_path varchar(500) NOT NULL, url_query varchar(500), utm_source varchar(255), utm_medium varchar(255), utm_campaign varchar(255), gclid varchar(255),
  referrer_path varchar(500), referrer_query varchar(500), referrer_domain varchar(500), page_title varchar(500),
  event_type integer NOT NULL DEFAULT 1, event_name varchar(50), hostname varchar(100));
CREATE TABLE event_data (event_data_id uuid PRIMARY KEY, website_id uuid NOT NULL, website_event_id uuid NOT NULL, data_key varchar(500) NOT NULL,
  string_value varchar(500), number_value decimal(19,4), date_value timestamptz, data_type integer NOT NULL, created_at timestamptz DEFAULT now());
`

func seed(t *testing.T, admin *pgx.Conn) {
	t.Helper()
	const (
		s1, s2, s3, s5, orphan = "51111111-0000-4000-8000-000000000001", "51111111-0000-4000-8000-000000000002", "51111111-0000-4000-8000-000000000003", "52222222-0000-4000-8000-000000000001", "59999999-0000-4000-8000-000000000009"
		v1, v2, v3, v9         = "61111111-0000-4000-8000-000000000001", "61111111-0000-4000-8000-000000000002", "61111111-0000-4000-8000-000000000003", "61111111-0000-4000-8000-000000000009"
	)
	event := func(id, site, session, visit, at, path, referrer, utm string, kind int, name string) string {
		nullable := func(s string) string {
			if s == "" {
				return "NULL"
			}
			return "'" + s + "'"
		}
		if id == "" {
			id = "gen_random_uuid()"
		} else {
			id = "'" + id + "'"
		}
		return fmt.Sprintf("(%s,'%s','%s',%s,'%s','%s','',%s,%s,'/search','Title',%d,%s,'shop.example.com')",
			id, site, session, nullable(visit), at, path, nullable(utm), nullable(referrer), kind, nullable(name))
	}
	var w2events []string
	for i, at := range []string{"2026-11-01 03:30:00+00", "2026-11-01 05:30:00+00", "2026-11-01 06:30:00+00", "2026-11-02 04:59:59+00", "2026-11-02 05:00:00+00"} {
		w2events = append(w2events, event("", w2, s5, fmt.Sprintf("62222222-0000-4000-8000-00000000000%d", i+1), at, "/ny", "", "", 1, ""))
	}
	script := umamiTables + fmt.Sprintf(`
INSERT INTO website(website_id,name,domain,deleted_at) VALUES ('%[1]s','Shop','shop.example.com',NULL),('%[2]s','NYC','nyc.example.com',NULL),('%[3]s','Gone','gone.example.com',now());
INSERT INTO session(session_id,website_id,browser,os,device,screen,language,country,region,city) VALUES
 ('%[4]s','%[1]s','chrome','Windows 10','desktop','1920x1080','en-US','US','US-NY','New York'),
 ('%[5]s','%[1]s','ios','iOS','mobile','390x844','zh-CN','CN',NULL,NULL),
 ('%[6]s','%[1]s','firefox','Linux','laptop','1366x768','de-DE','DE','DE-BE','Berlin'),
 ('%[7]s','%[2]s','chrome','Mac OS','laptop','1440x900','en-US','US','US-NY','New York');
INSERT INTO website_event(event_id,website_id,session_id,visit_id,created_at,url_path,url_query,utm_source,referrer_domain,referrer_path,page_title,event_type,event_name,hostname) VALUES
 %[8]s;
INSERT INTO event_data(event_data_id,website_id,website_event_id,data_key,string_value,number_value,date_value,data_type) VALUES
 (gen_random_uuid(),'%[1]s','%[9]s','plan','pro',NULL,NULL,1),
 (gen_random_uuid(),'%[1]s','%[9]s','seats',NULL,3,NULL,2),
 (gen_random_uuid(),'%[1]s','%[9]s','trial','true',NULL,NULL,3),
 (gen_random_uuid(),'%[1]s','%[9]s','when',NULL,NULL,'2026-10-21 08:00:00+00',4);`,
		w1, w2, w3, s1, s2, s3, s5,
		strings.Join(append([]string{
			event("", w1, s1, v1, "2026-10-20 03:00:00+00", "/", "www.google.com", "", 1, ""),
			event("", w1, s1, v1, "2026-10-20 03:05:00+00", "/a%20b", "", "", 1, ""),
			event("", w1, s2, v2, "2026-10-20 04:00:00+00", "/", "", "newsletter", 1, ""),
			// Either side of local midnight in Asia/Shanghai.
			event("", w1, s3, v3, "2026-10-20 15:59:59.999999+00", "/late", "", "", 1, ""),
			event("", w1, s3, v3, "2026-10-20 16:00:00+00", "/early", "", "", 1, ""),
			event("", w1, s2, "", "2026-10-21 02:00:00+00", "/old", "", "", 1, ""),        // recorded before visits existed
			event("", w1, orphan, v9, "2026-10-21 03:00:00+00", "/orphan", "", "", 1, ""), // its session row is gone
			event(signupID, w1, s1, v1, "2026-10-21 05:00:00+00", "/join", "", "", 2, "signup"),
			event("", w1, s1, v1, "2026-10-21 05:30:00+00", "/x", "", "", 3, "link-click"),
			event("", w1, s1, v1, "2026-10-21 05:40:00+00", "/x", "", "", 2, "pageview"),
		}, w2events...), ",\n "), signupID)
	if _, err := admin.Exec(context.Background(), script); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestReadsUmamiVersionTwo(t *testing.T) {
	admin, _, dsn := scratch(t)
	seed(t, admin)
	ctx := context.Background()
	db, err := openUmami(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sites, err := db.Websites(ctx)
	if err != nil || len(sites) != 2 || sites[0].Name != "NYC" || sites[1].Domain != "shop.example.com" {
		t.Fatalf("soft-deleted sites must not be offered: %+v %v", sites, err)
	}
	var seen []plost.UmamiEvent
	if err := db.Events(ctx, w1, plost.ImportWindow{}, func(e plost.UmamiEvent) error { seen = append(seen, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 9 {
		t.Fatalf("expected 7 pageviews and 2 custom events, got %d events", len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i].CreatedAt.Before(seen[i-1].CreatedAt) {
			t.Fatal("events must arrive in time order")
		}
	}
	by := map[string]plost.UmamiEvent{}
	for _, e := range seen {
		by[e.Path+"/"+e.Name] = e
	}
	if e := by["//"]; e.Browser != "chrome" && e.Browser != "ios" || e.Hostname != "shop.example.com" {
		t.Fatalf("session attributes and hostname should be joined: %+v", e)
	}
	if e := by["/old/"]; e.VisitID != "" || e.SessionID == "" {
		t.Fatalf("an event without a visit id keeps an empty one: %+v", e)
	}
	if e := by["/orphan/"]; e.Browser != "" || e.OS != "" || e.Country != "" {
		t.Fatalf("an event whose session row is gone has no attributes: %+v", e)
	}
	if e := by["/a%20b/"]; e.Path != "/a%20b" || e.Title != "Title" {
		t.Fatalf("path must be passed through for Plost to decode: %+v", e)
	}
	signup := by["/join/signup"]
	got := map[string]plost.UmamiProp{}
	for _, p := range signup.Props {
		got[p.Key] = p
	}
	if len(got) != 4 || got["plan"].String == nil || *got["plan"].String != "pro" || got["seats"].Number == nil || *got["seats"].Number != 3 ||
		got["trial"].Type != 3 || got["trial"].String == nil || *got["trial"].String != "true" ||
		got["when"].Date == nil || !got["when"].Date.Equal(time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("event properties: %+v", signup.Props)
	}
	if by["/x/pageview"].Props != nil || by["/old/"].Props != nil {
		t.Fatal("only custom events carry properties")
	}

	window := plost.ImportWindow{Since: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)}
	counts, err := db.EventTypeCounts(ctx, w1, window)
	if err != nil || counts[1] != 2 || counts[2] != 2 || counts[3] != 1 {
		t.Fatalf("type counts inside a window: %v %v", counts, err)
	}
	if all, _ := db.EventTypeCounts(ctx, w1, plost.ImportWindow{}); all[1] != 7 || all[2] != 2 || all[3] != 1 {
		t.Fatalf("type counts: %v", all)
	}

	// Calendar days follow the site's time zone, including the 25-hour day when daylight saving time ends.
	days, err := db.DayCounts(ctx, w2, "America/New_York", plost.ImportWindow{})
	want := []plost.UmamiDay{{Day: "2026-10-31", Pageviews: 1, Visitors: 1, Visits: 1}, {Day: "2026-11-01", Pageviews: 3, Visitors: 1, Visits: 3}, {Day: "2026-11-02", Pageviews: 1, Visitors: 1, Visits: 1}}
	if err != nil || fmt.Sprint(days) != fmt.Sprint(want) {
		t.Fatalf("New York days\n got %+v\nwant %+v (%v)", days, want, err)
	}
	days, err = db.DayCounts(ctx, w1, "Asia/Shanghai", plost.ImportWindow{})
	want = []plost.UmamiDay{{Day: "2026-10-20", Pageviews: 4, Visitors: 3, Visits: 3}, {Day: "2026-10-21", Pageviews: 3, CustomRaw: 2, Visitors: 3, Visits: 3}}
	if err != nil || fmt.Sprint(days) != fmt.Sprint(want) {
		t.Fatalf("Shanghai days (an event without a visit id counts its session as the visit)\n got %+v\nwant %+v (%v)", days, want, err)
	}

	// Last, because a refused write aborts the transaction. SQLSTATE 25006 is read_only_sql_transaction;
	// the message itself is localised by the server.
	var pgErr *pgconn.PgError
	if _, err := db.tx.Exec(ctx, "DELETE FROM website_event"); !errors.As(err, &pgErr) || pgErr.Code != "25006" {
		t.Fatalf("the Umami connection must be read-only, got %v", err)
	}
}

func TestImportsFromRealPostgres(t *testing.T) {
	admin, _, dsn := scratch(t)
	seed(t, admin)
	ctx := context.Background()
	dir := t.TempDir()
	app, err := plost.Open(plost.Config{DataDir: dir, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	app.Close()

	run := func(opts plost.ImportOptions) plost.ImportReport {
		t.Helper()
		db, err := openUmami(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		imp, err := plost.OpenImporter(dir, opts.Apply)
		if err != nil {
			t.Fatal(err)
		}
		defer imp.Close()
		rep, err := imp.Run(ctx, db, opts)
		if err != nil {
			t.Fatalf("%v\n%+v", err, rep.Sites)
		}
		return rep
	}
	shop := plost.ImportOptions{Sites: map[string]string{w1: "shop.example.com"}, CreateSites: true, Timezone: "Asia/Shanghai"}
	dry := run(shop)
	if dry.Apply || dry.Sites[0].Pageviews != 7 || dry.Sites[0].Custom != 1 || dry.Sites[0].Inserted != 0 || len(dry.Unmapped) != 1 || dry.Unmapped[0].Domain != "nyc.example.com" {
		t.Fatalf("dry run: %+v", dry)
	}
	shop.Apply = true
	rep := run(shop)
	s := rep.Sites[0]
	if s.Inserted != 8 || s.Skipped["reserved_event_name"] != 1 || s.OtherTypes["3"] != 1 || s.WithoutVisitID != 1 || len(s.Problems) != 0 || len(s.Days) != 2 {
		t.Fatalf("shop import: %+v", s)
	}
	if again := run(shop).Sites[0]; again.Inserted != 0 || again.AlreadyImported != 8 {
		t.Fatalf("a second import must change nothing: %+v", again)
	}

	nyc := plost.ImportOptions{Sites: map[string]string{w2: "nyc.example.com"}, CreateSites: true, Timezone: "America/New_York", Apply: true}
	n := run(nyc).Sites[0]
	if n.Inserted != 5 || len(n.Days) != 3 || n.Days[1].Day != "2026-11-01" || n.Days[1].Pageviews != 3 || len(n.Problems) != 0 {
		t.Fatalf("New York import: %+v", n)
	}
	// Plost wrote the same numbers back that Umami computed, checked in the importer against PostgreSQL itself.
	if n.Days[1].Visits != 3 || n.Days[1].Visitors != 1 {
		t.Fatalf("daily visitors and visits: %+v", n.Days[1])
	}
}

func TestRefusesAnUnsupportedUmami(t *testing.T) {
	admin, _, dsn := scratch(t)
	ctx := context.Background()
	if _, err := openUmami(ctx, dsn); err == nil || !strings.Contains(err.Error(), "不像 Umami") {
		t.Fatalf("an empty database must be refused as not Umami, got %v", err)
	}
	// An Umami from before visits were recorded cannot give a trustworthy visit count.
	if _, err := admin.Exec(ctx, strings.Replace(umamiTables, "visit_id uuid,", "", 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := openUmami(ctx, dsn); err == nil || !strings.Contains(err.Error(), "visit_id") {
		t.Fatalf("a missing visit_id must be refused with an explanation, got %v", err)
	}
}
