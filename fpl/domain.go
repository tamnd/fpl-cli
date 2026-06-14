package fpl

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes fpl as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/fpl-cli/fpl"
//
// The init below registers it; the host then dereferences fpl:// URIs by
// routing to the operations Register installs. The same Domain also builds the
// standalone fpl binary (see cli.NewApp), so the binary and a host share one
// source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the fpl driver.
type Domain struct{}

// Info describes the scheme, hostnames, and binary identity.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "fpl",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "fpl",
			Short:  "A command line for Fantasy Premier League.",
			Long: `A command line for Fantasy Premier League.

fpl reads public Fantasy Premier League data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/fpl-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name: "players", Group: "read", List: true,
		Summary: "List all FPL players",
	}, listPlayers)

	kit.Handle(app, kit.OpMeta{
		Name: "teams", Group: "read", List: true,
		Summary: "List all Premier League teams",
	}, listTeams)

	kit.Handle(app, kit.OpMeta{
		Name: "gameweeks", Group: "read", List: true,
		Summary: "List all FPL gameweeks",
	}, listGameweeks)

	kit.Handle(app, kit.OpMeta{
		Name: "fixtures", Group: "read", List: true,
		Summary: "List fixtures for a gameweek",
		Args:    []kit.Arg{{Name: "gameweek", Help: "gameweek number"}},
	}, listFixtures)

	kit.Handle(app, kit.OpMeta{
		Name: "player-history", Group: "read", List: true,
		Summary: "Show a player's gameweek history",
		Args:    []kit.Arg{{Name: "id", Help: "player element ID"}},
	}, listPlayerHistory)

	kit.Handle(app, kit.OpMeta{
		Name: "standings", Group: "read", List: true,
		Summary: "Show classic league standings",
		Args:    []kit.Arg{{Name: "league", Help: "league ID"}},
	}, listStandings)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- input structs ---

type playersInput struct {
	Limit  int     `kit:"flag,inherit"`
	Client *Client `kit:"inject"`
}

type teamsInput struct {
	Client *Client `kit:"inject"`
}

type gameweeksInput struct {
	Client *Client `kit:"inject"`
}

type fixturesInput struct {
	Gameweek int     `kit:"arg" help:"gameweek number"`
	Client   *Client `kit:"inject"`
}

type playerHistoryInput struct {
	ID     int     `kit:"arg" help:"player element ID"`
	Client *Client `kit:"inject"`
}

type standingsInput struct {
	League int     `kit:"arg" help:"league ID"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listPlayers(ctx context.Context, in playersInput, emit func(*Player) error) error {
	players, _, _, err := in.Client.BootstrapStatic(ctx)
	if err != nil {
		return err
	}
	for i := range players {
		if in.Limit > 0 && i >= in.Limit {
			break
		}
		if err := emit(&players[i]); err != nil {
			return err
		}
	}
	return nil
}

func listTeams(ctx context.Context, in teamsInput, emit func(*Team) error) error {
	_, teams, _, err := in.Client.BootstrapStatic(ctx)
	if err != nil {
		return err
	}
	for i := range teams {
		if err := emit(&teams[i]); err != nil {
			return err
		}
	}
	return nil
}

func listGameweeks(ctx context.Context, in gameweeksInput, emit func(*Gameweek) error) error {
	_, _, gameweeks, err := in.Client.BootstrapStatic(ctx)
	if err != nil {
		return err
	}
	for i := range gameweeks {
		if err := emit(&gameweeks[i]); err != nil {
			return err
		}
	}
	return nil
}

func listFixtures(ctx context.Context, in fixturesInput, emit func(*Fixture) error) error {
	fixtures, err := in.Client.Fixtures(ctx, in.Gameweek)
	if err != nil {
		return err
	}
	for i := range fixtures {
		if err := emit(&fixtures[i]); err != nil {
			return err
		}
	}
	return nil
}

func listPlayerHistory(ctx context.Context, in playerHistoryInput, emit func(*PlayerHistory) error) error {
	history, err := in.Client.PlayerSummary(ctx, in.ID)
	if err != nil {
		return err
	}
	for i := range history {
		if err := emit(&history[i]); err != nil {
			return err
		}
	}
	return nil
}

func listStandings(ctx context.Context, in standingsInput, emit func(*StandingEntry) error) error {
	_, entries, err := in.Client.LeagueStandings(ctx, in.League)
	if err != nil {
		return err
	}
	for i := range entries {
		if err := emit(&entries[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: pure string functions ---

// Classify turns an accepted input into (uriType, id).
// Numeric string → "player"; "gw:" prefix → "gameweek"; "l:" prefix → "league".
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)

	// full URL on this host
	if strings.HasPrefix(input, "https://"+Host) || strings.HasPrefix(input, "http://"+Host) {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(input, "https://"+Host), "http://"+Host)
		trimmed = strings.Trim(trimmed, "/")
		return classifyPath(trimmed)
	}

	// prefixed shorthand
	if strings.HasPrefix(input, "gw:") {
		id = strings.TrimPrefix(input, "gw:")
		if _, err2 := strconv.Atoi(id); err2 != nil {
			return "", "", errs.Usage("gameweek id must be numeric: %q", id)
		}
		return "gameweek", id, nil
	}
	if strings.HasPrefix(input, "l:") {
		id = strings.TrimPrefix(input, "l:")
		if _, err2 := strconv.Atoi(id); err2 != nil {
			return "", "", errs.Usage("league id must be numeric: %q", id)
		}
		return "league", id, nil
	}

	// bare numeric → player
	if _, err2 := strconv.Atoi(input); err2 == nil {
		return "player", input, nil
	}

	return "", "", errs.Usage("unrecognized fpl reference: %q", input)
}

func classifyPath(path string) (uriType, id string, err error) {
	parts := strings.SplitN(path, "/", 2)
	switch parts[0] {
	case "element-summary":
		if len(parts) < 2 {
			return "", "", errs.Usage("missing player id in path")
		}
		return "player", strings.Trim(parts[1], "/"), nil
	case "leagues":
		if len(parts) < 2 {
			return "", "", errs.Usage("missing league id in path")
		}
		id = strings.Split(strings.Trim(parts[1], "/"), "/")[0]
		return "league", id, nil
	}
	return "", "", errs.Usage("unrecognized fpl path: %q", path)
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "player":
		return fmt.Sprintf("https://%s/element-summary/%s", Host, id), nil
	case "league":
		return fmt.Sprintf("https://%s/leagues/%s/standings/c", Host, id), nil
	case "gameweek":
		return fmt.Sprintf("https://%s/api/fixtures/?event=%s", Host, id), nil
	}
	return "", errs.Usage("fpl has no resource type %q", uriType)
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
