package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/plost-dev/plost/internal/plost"
)

// umamiDB reads Umami's PostgreSQL in one read-only, repeatable-read transaction, so every
// query in a run sees the same snapshot even while Umami keeps collecting.
type umamiDB struct {
	conn    *pgx.Conn
	tx      pgx.Tx
	columns map[string]string // "table.column" -> data_type
	created string            // expression giving website_event.created_at as timestamptz
}

func openUmami(ctx context.Context, dsn string) (*umamiDB, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Umami 数据库：%w", err)
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		conn.Close(ctx)
		return nil, err
	}
	db := &umamiDB{conn: conn, tx: tx, columns: map[string]string{}}
	if err := db.detect(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (db *umamiDB) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = db.tx.Rollback(ctx)
	_ = db.conn.Close(ctx)
}

func (db *umamiDB) has(table, column string) bool { _, ok := db.columns[table+"."+column]; return ok }

// detect refuses databases that are not a recent Umami instead of guessing at their layout.
func (db *umamiDB) detect(ctx context.Context) error {
	rows, err := db.tx.Query(ctx, `SELECT table_name, column_name, data_type FROM information_schema.columns
 WHERE table_schema = current_schema() AND table_name IN ('website','website_event','session','event_data')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var table, column, kind string
		if err := rows.Scan(&table, &column, &kind); err != nil {
			return err
		}
		db.columns[table+"."+column] = kind
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for table, required := range map[string][]string{
		"website":       {"website_id", "name", "domain"},
		"website_event": {"event_id", "website_id", "session_id", "created_at", "event_type"},
		"session":       {"session_id"},
	} {
		for _, column := range required {
			if !db.has(table, column) {
				if !db.has(table, required[0]) {
					return fmt.Errorf("这不像 Umami 的数据库：当前模式里没有表 %s", table)
				}
				return fmt.Errorf("Umami 的 %s 表缺少 %s 列，无法识别这个版本", table, column)
			}
		}
	}
	if !db.has("website_event", "visit_id") {
		return errors.New("这个 Umami 的 website_event 表没有 visit_id（访问编号）列，无法可靠地还原访问次数；请先把 Umami 升级到记录访问编号的版本")
	}
	db.created = "e.created_at"
	if db.columns["website_event.created_at"] == "timestamp without time zone" {
		db.created = "(e.created_at AT TIME ZONE 'UTC')" // Umami stores UTC
	}
	return nil
}

// text selects an optional text column, or an empty string when this Umami version lacks it.
func (db *umamiDB) text(alias, table, column string) string {
	if db.has(table, column) {
		return "COALESCE(" + alias + "." + column + "::text, '')"
	}
	return "''"
}

func (db *umamiDB) hasEventData() bool {
	for _, c := range []string{"website_event_id", "data_key", "data_type", "string_value", "number_value", "date_value"} {
		if !db.has("event_data", c) {
			return false
		}
	}
	return true
}

func nullable(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (db *umamiDB) window() string {
	return "AND ($2::timestamptz IS NULL OR " + db.created + " >= $2) AND ($3::timestamptz IS NULL OR " + db.created + " < $3)"
}

func (db *umamiDB) Websites(ctx context.Context) ([]plost.UmamiWebsite, error) {
	query := "SELECT website_id::text, COALESCE(name, ''), COALESCE(domain, '') FROM website"
	if db.has("website", "deleted_at") {
		query += " WHERE deleted_at IS NULL"
	}
	rows, err := db.tx.Query(ctx, query+" ORDER BY name, website_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plost.UmamiWebsite
	for rows.Next() {
		var w plost.UmamiWebsite
		if err := rows.Scan(&w.ID, &w.Name, &w.Domain); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (db *umamiDB) EventTypeCounts(ctx context.Context, websiteID string, w plost.ImportWindow) (map[int]int64, error) {
	rows, err := db.tx.Query(ctx, "SELECT e.event_type, count(*) FROM website_event e WHERE e.website_id = $1 "+db.window()+" GROUP BY 1",
		websiteID, nullable(w.Since), nullable(w.Until))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int64{}
	for rows.Next() {
		var kind int32
		var n int64
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, err
		}
		out[int(kind)] = n
	}
	return out, rows.Err()
}

// DayCounts is deliberately a different query from Events: it is the independent figure the import is checked against.
func (db *umamiDB) DayCounts(ctx context.Context, websiteID, timezone string, w plost.ImportWindow) ([]plost.UmamiDay, error) {
	rows, err := db.tx.Query(ctx, `SELECT to_char((`+db.created+` AT TIME ZONE $4::text)::date, 'YYYY-MM-DD'),
 count(*) FILTER (WHERE e.event_type = 1),
 count(*) FILTER (WHERE e.event_type = 2),
 count(DISTINCT e.session_id::text) FILTER (WHERE e.event_type = 1),
 count(DISTINCT COALESCE(e.visit_id::text, e.session_id::text)) FILTER (WHERE e.event_type = 1)
 FROM website_event e WHERE e.website_id = $1 `+db.window()+` GROUP BY 1 ORDER BY 1`,
		websiteID, nullable(w.Since), nullable(w.Until), timezone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plost.UmamiDay
	for rows.Next() {
		var d plost.UmamiDay
		if err := rows.Scan(&d.Day, &d.Pageviews, &d.CustomRaw, &d.Visitors, &d.Visits); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type rawProp struct {
	Key    string   `json:"k"`
	Type   int      `json:"t"`
	String *string  `json:"s"`
	Number *float64 `json:"n"`
	Date   *string  `json:"d"`
}

func parseTimestamp(s string) *time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999", "2006-01-02 15:04:05.999999"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return &t
		}
	}
	return nil
}

func (db *umamiDB) Events(ctx context.Context, websiteID string, w plost.ImportWindow, fn func(plost.UmamiEvent) error) error {
	props, join := "NULL::jsonb", ""
	if db.hasEventData() {
		// Only custom events carry properties that Plost keeps.
		props = "p.props"
		join = `LEFT JOIN LATERAL (SELECT jsonb_agg(jsonb_build_object('k', d.data_key, 't', d.data_type, 's', d.string_value, 'n', d.number_value, 'd', d.date_value) ORDER BY d.data_key) AS props
 FROM event_data d WHERE d.website_event_id = e.event_id) p ON e.event_type = 2`
	}
	query := `SELECT e.event_id::text, e.website_id::text, e.session_id::text, COALESCE(e.visit_id::text, ''), ` + db.created + `, e.event_type,
 ` + db.text("e", "website_event", "event_name") + `, ` + db.text("e", "website_event", "hostname") + `, ` + db.text("e", "website_event", "url_path") + `,
 ` + db.text("e", "website_event", "referrer_domain") + `, ` + db.text("e", "website_event", "referrer_path") + `, ` + db.text("e", "website_event", "page_title") + `,
 ` + db.text("e", "website_event", "utm_source") + `, ` + db.text("e", "website_event", "utm_medium") + `, ` + db.text("e", "website_event", "utm_campaign") + `,
 ` + db.text("s", "session", "browser") + `, ` + db.text("s", "session", "os") + `, ` + db.text("s", "session", "device") + `,
 ` + db.text("s", "session", "screen") + `, ` + db.text("s", "session", "language") + `, ` + db.text("s", "session", "country") + `, ` + props + `
 FROM website_event e LEFT JOIN session s ON s.session_id = e.session_id ` + join + `
 WHERE e.website_id = $1 AND e.event_type IN (1, 2) ` + db.window() + ` ORDER BY ` + db.created + `, e.event_id`
	rows, err := db.tx.Query(ctx, query, websiteID, nullable(w.Since), nullable(w.Until))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e plost.UmamiEvent
		var kind int32
		var raw []byte
		if err := rows.Scan(&e.ID, &e.WebsiteID, &e.SessionID, &e.VisitID, &e.CreatedAt, &kind, &e.Name, &e.Hostname, &e.Path, &e.ReferrerDomain, &e.ReferrerPath,
			&e.Title, &e.UTMSource, &e.UTMMedium, &e.UTMCampaign, &e.Browser, &e.OS, &e.Device, &e.Screen, &e.Language, &e.Country, &raw); err != nil {
			return err
		}
		e.Type = int(kind)
		if len(raw) > 0 {
			var list []rawProp
			if err := json.Unmarshal(raw, &list); err != nil {
				return fmt.Errorf("事件 %s 的属性无法解析：%w", e.ID, err)
			}
			for _, p := range list {
				prop := plost.UmamiProp{Key: p.Key, Type: p.Type, String: p.String, Number: p.Number}
				if p.Date != nil {
					prop.Date = parseTimestamp(*p.Date)
				}
				e.Props = append(e.Props, prop)
			}
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

var _ plost.UmamiSource = (*umamiDB)(nil)
