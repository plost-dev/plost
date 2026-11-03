package plost

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A signed-in page hears "change" once a visit is stored; signed out, the stream is refused.
func TestChangeStream(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")

	r, _ := http.NewRequest("GET", x.server.URL+"/api/changes", nil)
	stream := &http.Client{Jar: x.client.Jar} // No timeout: the stream stays open.
	resp, err := stream.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	lines := make(chan string, 16)
	go func() {
		scan := bufio.NewScanner(resp.Body)
		for scan.Scan() {
			lines <- scan.Text()
		}
		close(lines)
	}()
	x.event(t, "example.com", "visitor-1", "/")
	deadline := time.After(5 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before a change")
			}
			if line == "event: change" {
				goto signedOut
			}
		case <-deadline:
			t.Fatal("no change event after a stored visit")
		}
	}
signedOut:
	out, err := http.Get(x.server.URL + "/api/changes")
	if err != nil {
		t.Fatal(err)
	}
	out.Body.Close()
	if out.StatusCode != 401 {
		t.Fatalf("signed out: %d", out.StatusCode)
	}
}
