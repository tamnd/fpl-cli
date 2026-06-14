// Package fpl is the library behind the fpl command line:
// the HTTP client, request shaping, and the typed data models for the
// Fantasy Premier League public API.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package fpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Host is the Fantasy Premier League API host.
const Host = "fantasy.premierleague.com"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// DefaultUserAgent identifies the client to FPL.
const DefaultUserAgent = "fpl/dev (+https://github.com/tamnd/fpl-cli)"

// Client talks to the FPL API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// Config holds optional overrides from the kit config.
type Config struct {
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults for FPL.
func DefaultConfig() Config {
	return Config{
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   5,
		Timeout:   30 * time.Second,
	}
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- output types ---

// Player is one FPL element (player).
type Player struct {
	ID          int     `kit:"id" json:"id"`
	Name        string  `json:"name"`
	FullName    string  `json:"full_name"`
	TeamID      int     `json:"team_id"`
	Price       float64 `json:"price"`
	TotalPoints int     `json:"total_points"`
	Form        string  `json:"form"`
	SelectedBy  string  `json:"selected_by"`
}

// Team is one Premier League club.
type Team struct {
	ID        int    `kit:"id" json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Played    int    `json:"played"`
	Won       int    `json:"won"`
	Drawn     int    `json:"drawn"`
	Lost      int    `json:"lost"`
	Points    int    `json:"points"`
}

// Gameweek is one FPL event.
type Gameweek struct {
	ID           int    `kit:"id" json:"id"`
	Name         string `json:"name"`
	DeadlineTime string `json:"deadline_time"`
	Finished     bool   `json:"finished"`
	IsCurrent    bool   `json:"is_current"`
	AverageScore int    `json:"average_score"`
	HighestScore int    `json:"highest_score"`
}

// Fixture is one match in a gameweek.
type Fixture struct {
	TeamHome    string `json:"team_home"`
	TeamAway    string `json:"team_away"`
	ScoreHome   int    `json:"score_home"`
	ScoreAway   int    `json:"score_away"`
	Finished    bool   `json:"finished"`
	KickoffTime string `json:"kickoff_time"`
}

// PlayerHistory is one gameweek's performance for a player.
type PlayerHistory struct {
	Round       int `json:"round"`
	Points      int `json:"points"`
	Minutes     int `json:"minutes"`
	Goals       int `json:"goals_scored"`
	Assists     int `json:"assists"`
	CleanSheets int `json:"clean_sheets"`
}

// StandingEntry is one row in a classic league standings table.
type StandingEntry struct {
	Rank      int    `json:"rank"`
	EntryName string `json:"entry_name"`
	Manager   string `json:"manager"`
	Total     int    `json:"total"`
}

// --- wire types for bootstrap-static ---

type wireBootstrap struct {
	Elements []wirePlayer `json:"elements"`
	Teams    []Team       `json:"teams"`
	Events   []wireEvent  `json:"events"`
}

type wirePlayer struct {
	ID                int    `json:"id"`
	WebName           string `json:"web_name"`
	FirstName         string `json:"first_name"`
	SecondName        string `json:"second_name"`
	Team              int    `json:"team"`
	NowCost           int    `json:"now_cost"`
	TotalPoints       int    `json:"total_points"`
	Form              string `json:"form"`
	SelectedByPercent string `json:"selected_by_percent"`
}

type wireEvent struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	DeadlineTime      string `json:"deadline_time"`
	Finished          bool   `json:"finished"`
	IsCurrent         bool   `json:"is_current"`
	AverageEntryScore int    `json:"average_entry_score"`
	HighestScore      int    `json:"highest_score"`
}

type wireFixture struct {
	TeamH      int    `json:"team_h"`
	TeamA      int    `json:"team_a"`
	TeamHScore int    `json:"team_h_score"`
	TeamAScore int    `json:"team_a_score"`
	Finished   bool   `json:"finished"`
	KickoffTime string `json:"kickoff_time"`
}

type wirePlayerSummary struct {
	History []wireHistory `json:"history"`
}

type wireHistory struct {
	Round       int `json:"round"`
	TotalPoints int `json:"total_points"`
	Minutes     int `json:"minutes"`
	GoalsScored int `json:"goals_scored"`
	Assists     int `json:"assists"`
	CleanSheets int `json:"clean_sheets"`
}

type wireEntry struct {
	Name              string `json:"name"`
	PlayerFirstName   string `json:"player_first_name"`
	PlayerLastName    string `json:"player_last_name"`
	SummaryOverallPoints int `json:"summary_overall_points"`
	SummaryOverallRank   int `json:"summary_overall_rank"`
}

type wireLeague struct {
	League    wireLeagueInfo  `json:"league"`
	Standings wireStandings   `json:"standings"`
}

type wireLeagueInfo struct {
	Name string `json:"name"`
}

type wireStandings struct {
	Results []wireStandingEntry `json:"results"`
}

type wireStandingEntry struct {
	Rank      int    `json:"rank"`
	EntryName string `json:"entry_name"`
	PlayerName string `json:"player_name"`
	Total     int    `json:"total"`
}

// --- API methods ---

// BootstrapStatic fetches the large bootstrap-static payload and returns
// players, teams, and gameweeks.
func (c *Client) BootstrapStatic(ctx context.Context) ([]Player, []Team, []Gameweek, error) {
	body, err := c.Get(ctx, BaseURL+"/api/bootstrap-static/")
	if err != nil {
		return nil, nil, nil, err
	}
	var w wireBootstrap
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, nil, nil, fmt.Errorf("decode bootstrap-static: %w", err)
	}

	players := make([]Player, len(w.Elements))
	for i, wp := range w.Elements {
		players[i] = Player{
			ID:          wp.ID,
			Name:        wp.WebName,
			FullName:    wp.FirstName + " " + wp.SecondName,
			TeamID:      wp.Team,
			Price:       float64(wp.NowCost) / 10.0,
			TotalPoints: wp.TotalPoints,
			Form:        wp.Form,
			SelectedBy:  wp.SelectedByPercent,
		}
	}

	gameweeks := make([]Gameweek, len(w.Events))
	for i, we := range w.Events {
		gameweeks[i] = Gameweek{
			ID:           we.ID,
			Name:         we.Name,
			DeadlineTime: we.DeadlineTime,
			Finished:     we.Finished,
			IsCurrent:    we.IsCurrent,
			AverageScore: we.AverageEntryScore,
			HighestScore: we.HighestScore,
		}
	}

	return players, w.Teams, gameweeks, nil
}

// Fixtures fetches fixtures for a given gameweek. It also fetches bootstrap to
// resolve team IDs to names.
func (c *Client) Fixtures(ctx context.Context, gw int) ([]Fixture, error) {
	// fetch team map from bootstrap
	_, teams, _, err := c.BootstrapStatic(ctx)
	if err != nil {
		return nil, err
	}
	teamMap := make(map[int]string, len(teams))
	for _, t := range teams {
		teamMap[t.ID] = t.Name
	}

	body, err := c.Get(ctx, fmt.Sprintf("%s/api/fixtures/?event=%d", BaseURL, gw))
	if err != nil {
		return nil, err
	}
	var wf []wireFixture
	if err := json.Unmarshal(body, &wf); err != nil {
		return nil, fmt.Errorf("decode fixtures: %w", err)
	}

	out := make([]Fixture, len(wf))
	for i, f := range wf {
		out[i] = Fixture{
			TeamHome:    teamMap[f.TeamH],
			TeamAway:    teamMap[f.TeamA],
			ScoreHome:   f.TeamHScore,
			ScoreAway:   f.TeamAScore,
			Finished:    f.Finished,
			KickoffTime: f.KickoffTime,
		}
	}
	return out, nil
}

// PlayerSummary fetches the gameweek history for a player by element ID.
func (c *Client) PlayerSummary(ctx context.Context, id int) ([]PlayerHistory, error) {
	body, err := c.Get(ctx, fmt.Sprintf("%s/api/element-summary/%d/", BaseURL, id))
	if err != nil {
		return nil, err
	}
	var ws wirePlayerSummary
	if err := json.Unmarshal(body, &ws); err != nil {
		return nil, fmt.Errorf("decode element-summary: %w", err)
	}
	out := make([]PlayerHistory, len(ws.History))
	for i, h := range ws.History {
		out[i] = PlayerHistory{
			Round:       h.Round,
			Points:      h.TotalPoints,
			Minutes:     h.Minutes,
			Goals:       h.GoalsScored,
			Assists:     h.Assists,
			CleanSheets: h.CleanSheets,
		}
	}
	return out, nil
}

// Entry fetches an FPL manager/team entry by ID.
func (c *Client) Entry(ctx context.Context, id int) (*wireEntry, error) {
	body, err := c.Get(ctx, fmt.Sprintf("%s/api/entry/%d/", BaseURL, id))
	if err != nil {
		return nil, err
	}
	var we wireEntry
	if err := json.Unmarshal(body, &we); err != nil {
		return nil, fmt.Errorf("decode entry: %w", err)
	}
	return &we, nil
}

// LeagueStandings fetches standings for a classic league by ID.
func (c *Client) LeagueStandings(ctx context.Context, id int) (string, []StandingEntry, error) {
	body, err := c.Get(ctx, fmt.Sprintf("%s/api/leagues-classic/%d/standings/", BaseURL, id))
	if err != nil {
		return "", nil, err
	}
	var wl wireLeague
	if err := json.Unmarshal(body, &wl); err != nil {
		return "", nil, fmt.Errorf("decode league standings: %w", err)
	}
	entries := make([]StandingEntry, len(wl.Standings.Results))
	for i, r := range wl.Standings.Results {
		entries[i] = StandingEntry{
			Rank:      r.Rank,
			EntryName: r.EntryName,
			Manager:   r.PlayerName,
			Total:     r.Total,
		}
	}
	return wl.League.Name, entries, nil
}
