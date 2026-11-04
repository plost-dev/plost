package plost

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// cnip.io publishes the ip2region release it is using (the paid edition, updated monthly) at a small JSON file; the
// IP database page shows that version and when it was put in place under the mainland China option.

const (
	cnipVersionURL  = "https://api.cnip.io/ip2region/version.json"
	cnipVersionKeep = 6 * time.Hour    // how long a good answer is reused
	cnipVersionMiss = 10 * time.Minute // how soon a failed fetch is tried again
)

var cnipVersionForm = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// cnipRelease is what the page shows: the release date (Beijing time) and when cnip.io put that database in place.
type cnipRelease struct {
	Version   string `json:"version"`
	UpdatedAt int64  `json:"updatedAt"`
}

type cnipVersion struct {
	url     string
	client  *http.Client
	mu      sync.Mutex
	value   *cnipRelease
	until   time.Time
	loading bool
}

func newCNIPVersion(url string, client *http.Client) *cnipVersion {
	return &cnipVersion{url: url, client: client}
}

// get answers from memory. With nothing known yet it fetches once and waits (the client's own short timeout); with
// an answer that has gone stale it returns that answer and refreshes in the background.
func (c *cnipVersion) get(ctx context.Context) *cnipRelease {
	c.mu.Lock()
	value, fresh := c.value, time.Now().Before(c.until)
	if fresh || c.loading {
		c.mu.Unlock()
		return value
	}
	c.loading = true
	c.mu.Unlock()
	if value != nil {
		go c.refresh(context.WithoutCancel(ctx))
		return value
	}
	return c.refresh(ctx)
}

func (c *cnipVersion) refresh(ctx context.Context) *cnipRelease {
	got := c.fetch(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loading = false
	if got != nil {
		c.value, c.until = got, time.Now().Add(cnipVersionKeep)
	} else {
		c.until = time.Now().Add(cnipVersionMiss)
	}
	return c.value
}

func (c *cnipVersion) fetch(ctx context.Context) *cnipRelease {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
	if err != nil {
		return nil
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var doc cnipRelease
	if json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&doc) != nil || !cnipVersionForm.MatchString(doc.Version) || doc.UpdatedAt <= 0 {
		return nil
	}
	return &doc
}
