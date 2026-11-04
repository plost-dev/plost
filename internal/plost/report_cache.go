package plost

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Reports of long ranges are kept for a while, because the dashboard asks again every ten seconds and a year of a
// busy site takes far longer than that to count. Ranges of a week or less take a second or two and are always counted afresh; who is online now
// is never cached (see report).
//
// Each range is computed once at a time, in the background under its own deadline: when a request gives up waiting,
// the work goes on, and the page's next automatic refresh finds it ready. A kept report past its freshness is still
// shown at once while a newer one is counted, until it is too old to show.
const (
	reportCacheSize  = 32
	reportComputeMax = 3 * time.Minute
)

// reportTTL is how long a report of the range counts as fresh; 0 means it is not cached.
func reportTTL(rng Range) time.Duration {
	switch span := rng.To.Sub(rng.From); {
	case span <= 8*24*time.Hour:
		return 0
	case span <= 32*24*time.Hour:
		return time.Minute
	default:
		return 5 * time.Minute
	}
}

// reportStaleMax is how long past its freshness a report may still be shown while a newer one is counted.
func reportStaleMax(ttl time.Duration) time.Duration { return 10 * ttl }

type reportCache struct {
	mu      sync.Mutex
	entries map[string]reportEntry
	flights map[string]*reportFlight
}

type reportEntry struct {
	at     time.Time
	report Report
}

type reportFlight struct {
	done   chan struct{}
	report Report
	err    error
}

func reportKey(epoch uint64, site Site, q url.Values) string {
	// A deleted site's id can be given to a new one, and the time zone moves every range.
	return strconv.FormatUint(epoch, 10) + "|" + strconvID(site.ID) + "|" + strconvID(site.CreatedAt) + "|" + site.Timezone + "|" + q.Encode()
}

// cachedReport returns the statistics of the range from the cache when it can, otherwise waits for them to be counted.
// The caller holds a.mu for reading.
func (a *App) cachedReport(ctx context.Context, site Site, rng Range, q url.Values) (Report, error) {
	ttl := reportTTL(rng)
	if ttl == 0 {
		return a.computeReport(ctx, site, rng, q)
	}
	key := reportKey(a.epoch, site, q)
	c := &a.reports
	c.mu.Lock()
	entry, ok := c.entries[key]
	age := time.Since(entry.at)
	if ok && age < ttl {
		c.mu.Unlock()
		return entry.report, nil
	}
	flight := c.flights[key]
	if flight == nil {
		flight = &reportFlight{done: make(chan struct{})}
		if c.flights == nil {
			c.flights = map[string]*reportFlight{}
		}
		c.flights[key] = flight
		go a.fillReport(key, flight, site, rng, q)
	}
	c.mu.Unlock()
	if ok && age < ttl+reportStaleMax(ttl) {
		return entry.report, nil
	}
	select {
	case <-flight.done:
		return flight.report, flight.err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Report{}, failure(503, "数据较多，仍在统计，请稍后刷新")
		}
		return Report{}, ctx.Err()
	}
}

func (a *App) fillReport(key string, flight *reportFlight, site Site, rng Range, q url.Values) {
	// Restoring a backup waits for this like for any other reader, and closing the program cancels it.
	a.mu.RLock()
	ctx, cancel := context.WithTimeout(a.jobCtx, reportComputeMax)
	flight.report, flight.err = a.computeReport(ctx, site, rng, q)
	cancel()
	a.mu.RUnlock()
	c := &a.reports
	c.mu.Lock()
	delete(c.flights, key)
	if flight.err == nil {
		if c.entries == nil {
			c.entries = map[string]reportEntry{}
		}
		c.entries[key] = reportEntry{at: time.Now(), report: flight.report}
		// The oldest go first; long-range reports are few, and each one is a few hundred kilobytes at most.
		for len(c.entries) > reportCacheSize {
			oldest := ""
			for k, e := range c.entries {
				if oldest == "" || e.at.Before(c.entries[oldest].at) {
					oldest = k
				}
			}
			delete(c.entries, oldest)
		}
	}
	c.mu.Unlock()
	close(flight.done)
}
