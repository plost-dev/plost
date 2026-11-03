package plost

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestAPIKeys(t *testing.T) {
	x := newTestApp(t)
	x.setup(t)
	x.addSite(t, "example.com")
	x.addSite(t, "other.com")
	x.call(t, "POST", "/api/api-keys", map[string]string{"domain": "example.com", "name": " "}, 400)
	x.call(t, "POST", "/api/api-keys", map[string]string{"domain": "missing.com", "name": "Blog"}, 404)
	var created struct {
		Key string `json:"key"`
	}
	json.Unmarshal(x.call(t, "POST", "/api/api-keys", map[string]string{"domain": "example.com", "name": "Blog"}, 200), &created)
	if !strings.HasPrefix(created.Key, "plst_") || len(created.Key) != 53 {
		t.Fatalf("key format: %q", created.Key)
	}
	stats := func(key, query string) int {
		req, _ := http.NewRequest("GET", x.server.URL+"/api/v1/stats"+query, nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp := mustDo(t, x.client, req)
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := stats(created.Key, "?period=live"); code != 200 {
		t.Fatalf("key read: %d", code)
	}
	if code := stats(created.Key, "?domain=other.com"); code != 403 {
		t.Fatalf("key scope: %d", code)
	}
	var list struct {
		Keys []struct {
			ID         int64  `json:"id"`
			Domain     string `json:"domain"`
			Hint       string `json:"hint"`
			LastUsedAt int64  `json:"lastUsedAt"`
		} `json:"keys"`
	}
	body := x.call(t, "GET", "/api/api-keys", nil, 200)
	json.Unmarshal(body, &list)
	if len(list.Keys) != 1 || list.Keys[0].Domain != "example.com" || list.Keys[0].Hint != created.Key[49:] || list.Keys[0].LastUsedAt == 0 {
		t.Fatalf("list: %s", body)
	}
	if strings.Contains(string(body), created.Key) {
		t.Fatal("list returned the full key")
	}
	x.call(t, "DELETE", "/api/api-keys/"+strconv.FormatInt(list.Keys[0].ID, 10), nil, 200)
	if code := stats(created.Key, ""); code != 401 {
		t.Fatalf("deleted key still works: %d", code)
	}
	x.call(t, "POST", "/api/logout", nil, 200)
	x.call(t, "GET", "/api/api-keys", nil, 401)
}
