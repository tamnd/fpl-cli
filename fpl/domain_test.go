package fpl

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "fpl" {
		t.Errorf("Scheme = %q, want fpl", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "fpl" {
		t.Errorf("Identity.Binary = %q, want fpl", info.Identity.Binary)
	}
}

func TestClassifyNumeric(t *testing.T) {
	typ, id, err := Domain{}.Classify("328")
	if err != nil {
		t.Fatalf("Classify(\"328\") error: %v", err)
	}
	if typ != "player" || id != "328" {
		t.Errorf("Classify(\"328\") = (%q, %q), want (player, 328)", typ, id)
	}
}

func TestClassifyGameweekPrefix(t *testing.T) {
	typ, id, err := Domain{}.Classify("gw:5")
	if err != nil {
		t.Fatalf("Classify(\"gw:5\") error: %v", err)
	}
	if typ != "gameweek" || id != "5" {
		t.Errorf("Classify(\"gw:5\") = (%q, %q), want (gameweek, 5)", typ, id)
	}
}

func TestClassifyLeaguePrefix(t *testing.T) {
	typ, id, err := Domain{}.Classify("l:314")
	if err != nil {
		t.Fatalf("Classify(\"l:314\") error: %v", err)
	}
	if typ != "league" || id != "314" {
		t.Errorf("Classify(\"l:314\") = (%q, %q), want (league, 314)", typ, id)
	}
}

func TestClassifyURL(t *testing.T) {
	typ, id, err := Domain{}.Classify("https://" + Host + "/element-summary/328/")
	if err != nil {
		t.Fatalf("Classify URL error: %v", err)
	}
	if typ != "player" || id != "328" {
		t.Errorf("Classify URL = (%q, %q), want (player, 328)", typ, id)
	}
}

func TestClassifyLeagueURL(t *testing.T) {
	typ, id, err := Domain{}.Classify("https://" + Host + "/leagues/314/standings/c")
	if err != nil {
		t.Fatalf("Classify league URL error: %v", err)
	}
	if typ != "league" || id != "314" {
		t.Errorf("Classify league URL = (%q, %q), want (league, 314)", typ, id)
	}
}

func TestClassifyUnrecognized(t *testing.T) {
	_, _, err := Domain{}.Classify("not-a-thing")
	if err == nil {
		t.Error("Classify(\"not-a-thing\") expected error, got nil")
	}
}

func TestLocatePlayer(t *testing.T) {
	got, err := Domain{}.Locate("player", "328")
	want := "https://" + Host + "/element-summary/328"
	if err != nil || got != want {
		t.Errorf("Locate(player, 328) = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateLeague(t *testing.T) {
	got, err := Domain{}.Locate("league", "314")
	want := "https://" + Host + "/leagues/314/standings/c"
	if err != nil || got != want {
		t.Errorf("Locate(league, 314) = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateGameweek(t *testing.T) {
	got, err := Domain{}.Locate("gameweek", "5")
	want := "https://" + Host + "/api/fixtures/?event=5"
	if err != nil || got != want {
		t.Errorf("Locate(gameweek, 5) = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("bogus", "1")
	if err == nil {
		t.Error("Locate(bogus, 1) expected error, got nil")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	// The fpl domain registers under scheme "fpl".
	_, ok := h.Domain("fpl")
	if !ok {
		t.Fatal("fpl domain not registered in kit host")
	}

	// ResolveOn: a bare numeric id resolves to a player URI.
	got, err := h.ResolveOn("fpl", "328")
	if err != nil {
		t.Fatalf("ResolveOn: %v", err)
	}
	if got.String() != "fpl://player/328" {
		t.Errorf("ResolveOn = %q, want fpl://player/328", got.String())
	}

	// ResolveOn: a gw: prefix resolves to a gameweek URI.
	got2, err := h.ResolveOn("fpl", "gw:1")
	if err != nil {
		t.Fatalf("ResolveOn gw: %v", err)
	}
	if got2.String() != "fpl://gameweek/1" {
		t.Errorf("ResolveOn gw:1 = %q, want fpl://gameweek/1", got2.String())
	}

	// ResolveOn: an l: prefix resolves to a league URI.
	got3, err := h.ResolveOn("fpl", "l:314")
	if err != nil {
		t.Fatalf("ResolveOn l: %v", err)
	}
	if got3.String() != "fpl://league/314" {
		t.Errorf("ResolveOn l:314 = %q, want fpl://league/314", got3.String())
	}
}
