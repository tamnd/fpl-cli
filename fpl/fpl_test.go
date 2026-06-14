package fpl_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/fpl-cli/fpl"
)

func newTestClient(t *testing.T, mux *http.ServeMux) (*fpl.Client, string) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := fpl.NewClient()
	c.Rate = 0
	c.HTTP = &http.Client{Timeout: 5 * time.Second}
	return c, srv.URL
}

// bootstrapPayload returns a minimal bootstrap-static JSON response.
func bootstrapPayload() []byte {
	payload := map[string]any{
		"elements": []map[string]any{
			{
				"id": 328, "web_name": "Salah", "first_name": "Mohamed", "second_name": "Salah",
				"team": 9, "now_cost": 130, "total_points": 200, "form": "8.5",
				"selected_by_percent": "45.2",
			},
			{
				"id": 1, "web_name": "Haaland", "first_name": "Erling", "second_name": "Haaland",
				"team": 11, "now_cost": 155, "total_points": 190, "form": "7.0",
				"selected_by_percent": "60.1",
			},
		},
		"teams": []map[string]any{
			{"id": 9, "name": "Liverpool", "short_name": "LIV", "played": 30, "win": 22, "draw": 5, "loss": 3, "points": 71},
			{"id": 11, "name": "Man City", "short_name": "MCI", "played": 30, "win": 20, "draw": 4, "loss": 6, "points": 64},
		},
		"events": []map[string]any{
			{
				"id": 1, "name": "Gameweek 1", "deadline_time": "2024-08-16T17:30:00Z",
				"finished": true, "is_current": false, "average_entry_score": 52, "highest_score": 140,
			},
			{
				"id": 2, "name": "Gameweek 2", "deadline_time": "2024-08-23T17:30:00Z",
				"finished": false, "is_current": true, "average_entry_score": 0, "highest_score": 0,
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}

func TestGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	})
	c, srvURL := newTestClient(t, mux)

	body, err := c.Get(context.Background(), srvURL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	})
	c, srvURL := newTestClient(t, mux)
	c.Retries = 5

	body, err := c.Get(context.Background(), srvURL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
}

func TestBootstrapStaticPlayers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/bootstrap-static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bootstrapPayload())
	})
	c, srvURL := newTestClient(t, mux)
	// Override BaseURL via a patched client by pointing it at test server
	_ = srvURL // client uses Host; patch via custom transport below

	// Use a transport that rewrites the host to the test server
	c.HTTP.Transport = &rewriteTransport{target: srvURL}

	players, teams, gws, err := c.BootstrapStatic(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 2 {
		t.Errorf("len(players) = %d, want 2", len(players))
	}
	if players[0].Name != "Salah" {
		t.Errorf("players[0].Name = %q, want Salah", players[0].Name)
	}
	if players[0].Price != 13.0 {
		t.Errorf("players[0].Price = %v, want 13.0", players[0].Price)
	}
	if players[0].FullName != "Mohamed Salah" {
		t.Errorf("players[0].FullName = %q, want Mohamed Salah", players[0].FullName)
	}
	if len(teams) != 2 {
		t.Errorf("len(teams) = %d, want 2", len(teams))
	}
	if teams[0].Name != "Liverpool" {
		t.Errorf("teams[0].Name = %q, want Liverpool", teams[0].Name)
	}
	if len(gws) != 2 {
		t.Errorf("len(gws) = %d, want 2", len(gws))
	}
	if !gws[1].IsCurrent {
		t.Error("gws[1].IsCurrent should be true")
	}
}

func TestFixtures(t *testing.T) {
	fixturesPayload := []map[string]any{
		{
			"team_h": 9, "team_a": 11,
			"team_h_score": 2, "team_a_score": 1,
			"finished": true, "kickoff_time": "2024-08-17T14:00:00Z",
		},
	}
	fb, _ := json.Marshal(fixturesPayload)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/bootstrap-static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bootstrapPayload())
	})
	mux.HandleFunc("/api/fixtures/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("event") != "1" {
			http.Error(w, "bad event", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fb)
	})

	c, srvURL := newTestClient(t, mux)
	c.HTTP.Transport = &rewriteTransport{target: srvURL}

	fixtures, err := c.Fixtures(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("len(fixtures) = %d, want 1", len(fixtures))
	}
	if fixtures[0].TeamHome != "Liverpool" {
		t.Errorf("TeamHome = %q, want Liverpool", fixtures[0].TeamHome)
	}
	if fixtures[0].TeamAway != "Man City" {
		t.Errorf("TeamAway = %q, want Man City", fixtures[0].TeamAway)
	}
	if fixtures[0].ScoreHome != 2 || fixtures[0].ScoreAway != 1 {
		t.Errorf("Score = %d-%d, want 2-1", fixtures[0].ScoreHome, fixtures[0].ScoreAway)
	}
}

func TestPlayerSummary(t *testing.T) {
	summaryPayload := map[string]any{
		"history": []map[string]any{
			{"round": 1, "total_points": 12, "minutes": 90, "goals_scored": 1, "assists": 1, "clean_sheets": 0},
			{"round": 2, "total_points": 6, "minutes": 90, "goals_scored": 0, "assists": 0, "clean_sheets": 1},
		},
		"history_past": []map[string]any{},
	}
	pb, _ := json.Marshal(summaryPayload)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/element-summary/328/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(pb)
	})

	c, srvURL := newTestClient(t, mux)
	c.HTTP.Transport = &rewriteTransport{target: srvURL}

	history, err := c.PlayerSummary(context.Background(), 328)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("len(history) = %d, want 2", len(history))
	}
	if history[0].Points != 12 {
		t.Errorf("history[0].Points = %d, want 12", history[0].Points)
	}
	if history[0].Goals != 1 {
		t.Errorf("history[0].Goals = %d, want 1", history[0].Goals)
	}
}

func TestLeagueStandings(t *testing.T) {
	leaguePayload := map[string]any{
		"league": map[string]any{"name": "Test League"},
		"standings": map[string]any{
			"results": []map[string]any{
				{"rank": 1, "entry_name": "Team Alpha", "player_name": "Alice", "total": 1200},
				{"rank": 2, "entry_name": "Team Beta", "player_name": "Bob", "total": 1150},
			},
		},
	}
	lb, _ := json.Marshal(leaguePayload)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/leagues-classic/314/standings/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(lb)
	})

	c, srvURL := newTestClient(t, mux)
	c.HTTP.Transport = &rewriteTransport{target: srvURL}

	name, entries, err := c.LeagueStandings(context.Background(), 314)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Test League" {
		t.Errorf("league name = %q, want Test League", name)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].EntryName != "Team Alpha" {
		t.Errorf("entries[0].EntryName = %q, want Team Alpha", entries[0].EntryName)
	}
	if entries[0].Total != 1200 {
		t.Errorf("entries[0].Total = %d, want 1200", entries[0].Total)
	}
	if entries[1].Manager != "Bob" {
		t.Errorf("entries[1].Manager = %q, want Bob", entries[1].Manager)
	}
}

func TestEntry(t *testing.T) {
	entryPayload := map[string]any{
		"name": "Alice FC",
		"player_first_name": "Alice",
		"player_last_name": "Smith",
		"summary_overall_points": 1500,
		"summary_overall_rank": 100000,
	}
	eb, _ := json.Marshal(entryPayload)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/entry/1/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(eb)
	})

	c, srvURL := newTestClient(t, mux)
	c.HTTP.Transport = &rewriteTransport{target: srvURL}

	entry, err := c.Entry(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Name != "Alice FC" {
		t.Errorf("entry.Name = %q, want Alice FC", entry.Name)
	}
	if entry.SummaryOverallPoints != 1500 {
		t.Errorf("entry.SummaryOverallPoints = %d, want 1500", entry.SummaryOverallPoints)
	}
}

// rewriteTransport rewrites all requests to target host for testing.
type rewriteTransport struct {
	target string
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = rt.target[len("http://"):]
	return http.DefaultTransport.RoundTrip(req2)
}
