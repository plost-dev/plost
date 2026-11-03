package plost

import (
	"io"
	"net/http"
	"sync"
	"time"
)

// changeHub tells the open admin pages that new visits were stored, so they reload at once instead of waiting for
// their own timer. It carries no data: a page that hears of a change asks for its figures as usual.
type changeHub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
	done chan struct{}
	once sync.Once
}

func newChangeHub() *changeHub {
	return &changeHub{subs: map[chan struct{}]struct{}{}, done: make(chan struct{})}
}

func (h *changeHub) subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// notify never waits: a page that has not caught up yet already has a change pending, which covers this one too.
func (h *changeHub) notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *changeHub) close() { h.once.Do(func() { close(h.done) }) }

// changeEvery keeps a busy instance from reloading the open pages more than once in this long.
const changeEvery = 3 * time.Second

// changeStream is a server-sent event stream: "change" whenever visits were stored, at most once per changeEvery,
// and a comment every 25 seconds so proxies keep the idle connection open. It holds no lock while it waits.
func (a *App) changeStream(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	_, err := a.user(r)
	a.mu.RUnlock()
	if err != nil {
		writeError(w, failure(401, "请先登录"))
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{}) // The server's write timeout would cut the stream after a minute.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	changes, cancel := a.changes.subscribe()
	defer cancel()
	send := func(s string) bool {
		if _, err := io.WriteString(w, s); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send("retry: 5000\n\n") {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	var last time.Time
	for {
		select {
		case <-r.Context().Done():
			return
		case <-a.changes.done:
			return
		case <-ping.C:
			if !send(": ping\n\n") {
				return
			}
		case <-changes:
			if wait := time.Until(last.Add(changeEvery)); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-r.Context().Done():
					timer.Stop()
					return
				case <-a.changes.done:
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			last = time.Now()
			if !send("event: change\ndata: {}\n\n") {
				return
			}
		}
	}
}
