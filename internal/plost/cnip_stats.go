package plost

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// How much the mainland China lookups at api.cnip.io have been used, shown on the IP database page. Each lookup is
// one address not asked about in the last day (answers are kept that long), so Calls also counts the addresses.
// Only counts and city names are kept, in settings.cnip_stats; never an address. The numbers are written every few
// minutes and when the program stops, so a crash loses at most those few minutes.

const cnipStatsFlush = 5 * time.Minute

type cnipStats struct {
	mu      sync.Mutex
	Calls   int64           `json:"calls"`   // lookups sent to api.cnip.io
	Located int64           `json:"located"` // of those, answers that named a city
	Cities  map[string]bool `json:"cities"`  // every city named so far
	Since   int64           `json:"since"`   // when counting started
	dirty   bool
}

// cnipStatsView is what the page shows.
type cnipStatsView struct {
	Calls   int64 `json:"calls"`
	Located int64 `json:"located"`
	Cities  int   `json:"cities"`
	Since   int64 `json:"since"`
}

func (a *App) loadCNIPStats() *cnipStats {
	s := &cnipStats{}
	_ = json.Unmarshal([]byte(a.setting(context.Background(), "cnip_stats", "{}")), s)
	if s.Cities == nil {
		s.Cities = map[string]bool{}
	}
	if s.Since == 0 {
		s.Since, s.dirty = time.Now().Unix(), true
	}
	return s
}

func (s *cnipStats) record(p onlinePlace, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls++
	if ok && p.city != "" {
		s.Located++
		if len(s.Cities) < 5000 {
			s.Cities[p.city] = true
		}
	}
	s.dirty = true
}

func (s *cnipStats) view() cnipStatsView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cnipStatsView{Calls: s.Calls, Located: s.Located, Cities: len(s.Cities), Since: s.Since}
}

// flush writes the numbers when they changed. The caller holds a.mu for reading, as every database writer does.
func (a *App) flushCNIPStats(ctx context.Context) {
	s := a.cnipStats
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	body, err := json.Marshal(s)
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		return
	}
	if _, err := a.db.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES('cnip_stats',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", string(body)); err != nil {
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
	}
}

func (a *App) cnipStatsLoop() {
	defer a.workers.Done()
	ticker := time.NewTicker(cnipStatsFlush)
	defer ticker.Stop()
	for {
		select {
		case <-a.stop:
			a.mu.RLock()
			a.flushCNIPStats(context.Background())
			a.mu.RUnlock()
			return
		case <-ticker.C:
			a.mu.RLock()
			a.flushCNIPStats(a.jobCtx)
			a.mu.RUnlock()
		}
	}
}
