package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"be-demo/internal/store"
)

// Use a file so SQLite behaves as in production.
func newHandler(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st)
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func mustDo(t *testing.T, h http.Handler, method, path, body string, want int) {
	t.Helper()
	if rec := do(t, h, method, path, body); rec.Code != want {
		t.Fatalf("%s %s %s: status %d, want %d; body %s", method, path, body, rec.Code, want, rec.Body)
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

func TestCreatePlayer(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"valid", `{"username":"player123"}`, http.StatusCreated},
		{"trims whitespace", `{"username":"  spaced  "}`, http.StatusCreated},
		{"empty", `{"username":""}`, http.StatusBadRequest},
		{"whitespace only", `{"username":"   "}`, http.StatusBadRequest},
		{"missing", `{}`, http.StatusBadRequest},
		{"invalid characters", `{"username":"bad name!"}`, http.StatusBadRequest},
		{"too long", `{"username":"` + strings.Repeat("a", 33) + `"}`, http.StatusBadRequest},
		{"wrong type", `{"username":123}`, http.StatusBadRequest},
		{"malformed JSON", `{"username":`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
		{"unknown field", `{"username":"x","admin":true}`, http.StatusBadRequest},
		{"trailing data", `{"username":"x"}{}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newHandler(t), http.MethodPost, "/players", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
			if rec.Code != http.StatusCreated {
				if got := decode[map[string]string](t, rec); got["error"] == "" {
					t.Fatalf("missing error message: %s", rec.Body)
				}
			}
		})
	}
}

func TestCreatePlayerResponse(t *testing.T) {
	rec := do(t, newHandler(t), http.MethodPost, "/players", `{"username":"  player123 "}`)
	got := decode[store.Player](t, rec)
	if got.Username != "player123" {
		t.Errorf("username = %q, want player123", got.Username)
	}
	if got.CreatedAt.IsZero() || got.CreatedAt.Location().String() != "UTC" {
		t.Errorf("created_at = %v, want a UTC timestamp", got.CreatedAt)
	}
}

func TestCreatePlayerDuplicate(t *testing.T) {
	h := newHandler(t)
	mustDo(t, h, http.MethodPost, "/players", `{"username":"player123"}`, http.StatusCreated)

	for _, name := range []string{"player123", "PLAYER123"} {
		rec := do(t, h, http.MethodPost, "/players", `{"username":"`+name+`"}`)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: status %d, want 409; body %s", name, rec.Code, rec.Body)
		}
	}
}

func TestOversizedBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"oversized string", `{"username":"` + strings.Repeat("a", maxBodyBytes) + `"}`},
		// Valid JSON, then the limit is hit while checking for trailing data.
		{"oversized trailing whitespace", `{"username":"player123"}` + strings.Repeat(" ", maxBodyBytes)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, newHandler(t), http.MethodPost, "/players", tt.body)
			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status %d, want 413; body %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestSubmitScore(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"valid", `{"username":"player123","score":1500}`, http.StatusCreated},
		{"zero", `{"username":"player123","score":0}`, http.StatusCreated},
		{"username case-insensitive", `{"username":"PLAYER123","score":1}`, http.StatusCreated},
		{"unknown player", `{"username":"ghost","score":1500}`, http.StatusNotFound},
		{"negative", `{"username":"player123","score":-1}`, http.StatusBadRequest},
		{"float", `{"username":"player123","score":1.5}`, http.StatusBadRequest},
		{"string", `{"username":"player123","score":"1500"}`, http.StatusBadRequest},
		{"overflow", `{"username":"player123","score":99999999999999999999}`, http.StatusBadRequest},
		{"null score", `{"username":"player123","score":null}`, http.StatusBadRequest},
		{"missing score", `{"username":"player123"}`, http.StatusBadRequest},
		{"empty username", `{"username":"","score":10}`, http.StatusBadRequest},
		{"unknown field", `{"username":"player123","score":10,"cheat":true}`, http.StatusBadRequest},
		{"malformed JSON", `{"score":`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHandler(t)
			mustDo(t, h, http.MethodPost, "/players", `{"username":"player123"}`, http.StatusCreated)
			rec := do(t, h, http.MethodPost, "/scores", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestSubmitScoreResponse(t *testing.T) {
	h := newHandler(t)
	mustDo(t, h, http.MethodPost, "/players", `{"username":"player123"}`, http.StatusCreated)
	got := decode[store.Score](t, do(t, h, http.MethodPost, "/scores", `{"username":"player123","score":1500}`))
	if got.Username != "player123" || got.Score != 1500 || got.CreatedAt.IsZero() {
		t.Errorf("response = %+v, want player123 / 1500 with a timestamp", got)
	}
}

func leaderboard(t *testing.T, h http.Handler, query string) []store.Entry {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/leaderboard"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body %s", rec.Code, rec.Body)
	}
	return decode[leaderboardResponse](t, rec).Leaderboard
}

func TestLeaderboardEmpty(t *testing.T) {
	rec := do(t, newHandler(t), http.MethodGet, "/leaderboard", "")
	if got, want := strings.TrimSpace(rec.Body.String()), `{"leaderboard":[],"total":0}`; got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
}

func TestLeaderboardRanking(t *testing.T) {
	h := newHandler(t)
	for _, name := range []string{"alice", "bob", "carol", "dave", "idle"} {
		mustDo(t, h, http.MethodPost, "/players", `{"username":"`+name+`"}`, http.StatusCreated)
	}
	for _, s := range []struct {
		name  string
		score int
	}{
		{"alice", 900},
		{"alice", 1500}, // best score
		{"alice", 100},  // a later, lower score must not replace it
		{"bob", 1200},
		{"dave", 1200}, // ties with bob; username breaks the tie
		{"carol", 0},
	} {
		body := fmt.Sprintf(`{"username":%q,"score":%d}`, s.name, s.score)
		mustDo(t, h, http.MethodPost, "/scores", body, http.StatusCreated)
	}

	got := leaderboard(t, h, "")
	want := []store.Entry{
		{Username: "alice", Score: 1500},
		{Username: "bob", Score: 1200},
		{Username: "dave", Score: 1200},
		{Username: "carol", Score: 0},
	} // "idle" has no scores and is left out
	if !slices.Equal(got, want) {
		t.Fatalf("leaderboard = %v, want %v", got, want)
	}

	if got := leaderboard(t, h, "?limit=1"); len(got) != 1 || got[0].Username != "alice" {
		t.Fatalf("limit=1: got %v, want only alice", got)
	}
}

// Player p{i} scores i, so ranking reverses insertion order.
func seedRanked(t *testing.T, h http.Handler, n int) {
	t.Helper()
	for i := range n {
		name := fmt.Sprintf("p%02d", i)
		mustDo(t, h, http.MethodPost, "/players", `{"username":"`+name+`"}`, http.StatusCreated)
		mustDo(t, h, http.MethodPost, "/scores", fmt.Sprintf(`{"username":%q,"score":%d}`, name, i), http.StatusCreated)
	}
}

func TestLeaderboardDefaultLimit(t *testing.T) {
	h := newHandler(t)
	seedRanked(t, h, 15)
	if got := leaderboard(t, h, ""); len(got) != defaultLimit {
		t.Fatalf("default: %d entries, want %d", len(got), defaultLimit)
	}
	if got := leaderboard(t, h, "?limit=100"); len(got) != 15 {
		t.Fatalf("limit=100: %d entries, want 15", len(got))
	}
}

func TestLeaderboardPaging(t *testing.T) {
	h := newHandler(t)
	seedRanked(t, h, 15)
	mustDo(t, h, http.MethodPost, "/players", `{"username":"idle"}`, http.StatusCreated) // unscored: not counted

	tests := []struct {
		query     string
		wantCount int
		wantFirst string
	}{
		{"?limit=10&offset=0", 10, "p14"},
		{"?limit=10&offset=10", 5, "p04"},
		{"?limit=5&offset=14", 1, "p00"},
		{"?offset=15", 0, ""},
		{"?offset=1000", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/leaderboard"+tt.query, "")
			got := decode[leaderboardResponse](t, rec)
			if got.Total != 15 {
				t.Errorf("total = %d, want 15", got.Total)
			}
			if len(got.Leaderboard) != tt.wantCount {
				t.Fatalf("%d entries, want %d", len(got.Leaderboard), tt.wantCount)
			}
			if tt.wantCount > 0 && got.Leaderboard[0].Username != tt.wantFirst {
				t.Errorf("first = %s, want %s", got.Leaderboard[0].Username, tt.wantFirst)
			}
		})
	}
}

func TestLeaderboardInvalidParams(t *testing.T) {
	h := newHandler(t)
	for _, q := range []string{
		"limit=", "limit=0", "limit=-1", "limit=101", "limit=abc", "limit=1.5", "limit=10abc",
		"offset=", "offset=-1", "offset=abc", "offset=1.5", "offset=99999999999999999999",
	} {
		rec := do(t, h, http.MethodGet, "/leaderboard?"+q, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", q, rec.Code)
		}
	}
}

func TestUI(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, http.MethodGet, "/", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("GET /: status %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "<title>Mini Game Leaderboard</title>") {
		t.Fatal("GET /: page title missing")
	}
	// Only "/" serves the UI; other paths are not swallowed by it.
	if rec := do(t, h, http.MethodGet, "/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET /nope: status %d, want 404", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	rec := do(t, newHandler(t), http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK || decode[map[string]string](t, rec)["status"] != "ok" {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
}

func TestConcurrentRequests(t *testing.T) {
	srv := httptest.NewServer(newHandler(t))
	defer srv.Close()

	post := func(path, body string) (int, error) {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			return 0, err
		}
		resp.Body.Close()
		return resp.StatusCode, nil
	}

	const n = 20
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			code, err := post("/players", `{"username":"racer"}`)
			if err != nil {
				t.Error(err)
			}
			codes <- code
		})
	}
	wg.Wait()
	close(codes)

	count := map[int]int{}
	for c := range codes {
		count[c]++
	}
	if count[http.StatusCreated] != 1 || count[http.StatusConflict] != n-1 {
		t.Fatalf("status counts = %v, want one 201 and %d 409s", count, n-1)
	}

	for i := range n {
		wg.Go(func() {
			code, err := post("/scores", fmt.Sprintf(`{"username":"racer","score":%d}`, i))
			if err != nil || code != http.StatusCreated {
				t.Errorf("score %d: status %d, err %v", i, code, err)
			}
		})
		wg.Go(func() {
			resp, err := http.Get(srv.URL + "/leaderboard")
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			var body leaderboardResponse
			if resp.StatusCode != http.StatusOK {
				t.Errorf("leaderboard: status %d, want 200", resp.StatusCode)
			} else if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Errorf("leaderboard: decode: %v", err)
			}
		})
	}
	wg.Wait()

	got := leaderboard(t, srv.Config.Handler, "")
	if len(got) != 1 || got[0].Score != n-1 {
		t.Fatalf("leaderboard = %v, want racer with %d", got, n-1)
	}
}
