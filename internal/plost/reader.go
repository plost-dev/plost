package plost

import (
	"context"
	"database/sql"
	"runtime"
	"sync"
)

// Report queries run on a separate pool of read-only connections. With WAL they see every committed event, a long
// report no longer queues collection writes and logins behind it on the single writing connection, and a report's
// independent queries can run side by side: a 28-day dashboard of a busy site is two dozen scans of the same range,
// which one connection after another took longer than the request deadline.
func readerConns() int {
	return min(max(runtime.NumCPU(), 2), 6)
}

func openReader(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqliteURL(path, "query_only(ON)"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(readerConns())
	db.SetMaxIdleConns(readerConns())
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// parallel runs the tasks at once and returns the first error; a failure cancels the context the others run under.
// The reader pool's size bounds how many queries really run together.
func parallel(ctx context.Context, tasks ...func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	var once sync.Once
	var first error
	for _, task := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := task(ctx); err != nil {
				once.Do(func() { first = err; cancel() })
			}
		}()
	}
	wg.Wait()
	return first
}

// collectRows reads name/value rows, the shape most report lists share.
func collectRows(rows *sql.Rows, err error) ([]Row, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var v Row
		if err := rows.Scan(&v.Name, &v.Value); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
