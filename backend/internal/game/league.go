package game

import (
	"math"
	"sort"
)

// The solo season is a league of ten clubs: yours and the nine rivals of
// clubNames, everyone meeting everyone once over nine matchdays. Points as in
// football: 3 for a win, 1 for a draw. Your matches are simulated; the rivals'
// matches against each other are told, not played: a scoreline drawn from
// each club's strength, so the table moves every matchday at no cost.

const (
	LeagueTeams    = MaxRounds + 1
	YourClub       = "Your Club"
	EuropePlaces   = 4 // places 2-4 go to Europe; 1 is the champion
	RelegatedPlace = LeagueTeams - 1
)

type LeagueTeam struct {
	Name     string  `json:"name"`
	You      bool    `json:"you,omitempty"`
	P        int     `json:"p"`
	W        int     `json:"w"`
	D        int     `json:"d"`
	L        int     `json:"l"`
	GF       int     `json:"gf"`
	GA       int     `json:"ga"`
	Pts      int     `json:"pts"`
	Strength float64 `json:"-"` // rivals only: how good the club is this season
}

type LeagueResult struct {
	Round int    `json:"round"`
	Home  string `json:"home"`
	Away  string `json:"away"`
	HG    int    `json:"hg"`
	AG    int    `json:"ag"`
}

type League struct {
	Teams   []LeagueTeam   `json:"teams"`
	Results []LeagueResult `json:"results"`
}

// NewLeague sets up the table. Rival k (1..9, the order you meet them) is
// stronger the later it comes, with a little form drawn for this season.
func NewLeague(g *RNG) *League { return newTable(g, clubNames[:]) }

// newTable is a round robin of your club and `rivals`, met in that order.
func newTable(g *RNG, rivals []string) *League {
	lg := &League{Teams: []LeagueTeam{{Name: YourClub, You: true}}}
	for k, name := range rivals {
		lg.Teams = append(lg.Teams, LeagueTeam{Name: name, Strength: float64(k+1) + (g.Float()-0.5)*2.5})
	}
	return lg
}

// fixtures returns the pairs of team indexes meeting on a matchday (1..n-1)
// of a round robin of n teams (n even). Circle method with your club fixed;
// teams are relabelled so that on matchday r you meet rival r, the rivals
// coming in rising strength.
func fixtures(n, round int) [][2]int {
	// Circle method over positions 0..n-1, position 0 fixed.
	rot := make([]int, n-1)
	for i := range rot {
		rot[i] = i + 1
	}
	var rounds [][][2]int
	for r := 0; r < n-1; r++ {
		var pairs [][2]int
		pairs = append(pairs, [2]int{0, rot[0]})
		for i := 1; i < n/2; i++ {
			pairs = append(pairs, [2]int{rot[i], rot[n-1-i]})
		}
		rounds = append(rounds, pairs)
		rot = append(rot[len(rot)-1:], rot[:len(rot)-1]...)
	}
	// Position met by team 0 on round r becomes rival r+1.
	label := make([]int, n)
	for r, pairs := range rounds {
		label[pairs[0][1]] = r + 1
	}
	out := [][2]int{}
	for _, p := range rounds[round-1] {
		out = append(out, [2]int{label[p[0]], label[p[1]]})
	}
	return out
}

func poisson(g *RNG, lambda float64) int {
	l, k, p := math.Exp(-lambda), 0, 1.0
	for {
		p *= g.Float()
		if p <= l || k > 9 {
			return k
		}
		k++
	}
}

// story scoreline between two rivals: the stronger side scores more on average.
func toldScore(g *RNG, a, b float64) (int, int) {
	diff := (a - b) * leagueEdge
	la := math.Min(3.2, math.Max(0.25, 1.35+diff))
	lb := math.Min(3.2, math.Max(0.25, 1.15-diff))
	return poisson(g, la), poisson(g, lb)
}

// leagueEdge is how much a point of strength moves the expected goals of the
// rivals' told matches. Tuned with TestBalanceSoloSeason.
var leagueEdge = 0.09

func (lg *League) record(round int, home, away, hg, ag int) {
	lg.Results = append(lg.Results, LeagueResult{Round: round, Home: lg.Teams[home].Name, Away: lg.Teams[away].Name, HG: hg, AG: ag})
	h, a := &lg.Teams[home], &lg.Teams[away]
	h.P, a.P = h.P+1, a.P+1
	h.GF, h.GA, a.GF, a.GA = h.GF+hg, h.GA+ag, a.GF+ag, a.GA+hg
	switch {
	case hg > ag:
		h.W, a.L, h.Pts = h.W+1, a.L+1, h.Pts+3
	case hg < ag:
		a.W, h.L, a.Pts = a.W+1, h.L+1, a.Pts+3
	default:
		h.D, a.D, h.Pts, a.Pts = h.D+1, a.D+1, h.Pts+1, a.Pts+1
	}
}

// playRound records your match of the matchday and tells the others.
func (lg *League) playRound(g *RNG, round, gf, ga int) {
	for _, p := range fixtures(len(lg.Teams), round) {
		if p[0] == 0 {
			lg.record(round, 0, p[1], gf, ga)
			continue
		}
		hg, ag := toldScore(g, lg.Teams[p[0]].Strength, lg.Teams[p[1]].Strength)
		lg.record(round, p[0], p[1], hg, ag)
	}
}

// Table is the standings: points, then goal difference, then goals scored.
func (lg *League) Table() []LeagueTeam {
	t := append([]LeagueTeam(nil), lg.Teams...)
	sort.SliceStable(t, func(i, j int) bool {
		a, b := t[i], t[j]
		if a.Pts != b.Pts {
			return a.Pts > b.Pts
		}
		if a.GF-a.GA != b.GF-b.GA {
			return a.GF-a.GA > b.GF-b.GA
		}
		if a.GF != b.GF {
			return a.GF > b.GF
		}
		return a.Name < b.Name
	})
	return t
}

// Mine is your club's row.
func (lg *League) Mine() LeagueTeam {
	for _, t := range lg.Teams {
		if t.You {
			return t
		}
	}
	return LeagueTeam{}
}

// Position is your current place in the table (1-based).
func (lg *League) Position() int {
	for i, t := range lg.Table() {
		if t.You {
			return i + 1
		}
	}
	return 0
}

// LeagueView is the table as the client sees it, with where the season stands.
type LeagueView struct {
	Table    []LeagueTeam   `json:"table"`
	Last     []LeagueResult `json:"last"` // the latest matchday's results
	Played   int            `json:"played"`
	Position int            `json:"position"`
	Points   int            `json:"points"`
	Europe   int            `json:"europe"`   // last European place
	Relegate int            `json:"relegate"` // first relegation place
}

func (lg *League) view() *LeagueView {
	v := &LeagueView{Table: lg.Table(), Last: []LeagueResult{}, Position: lg.Position(), Europe: EuropePlaces, Relegate: RelegatedPlace}
	if n := len(lg.Results); n > 0 {
		v.Played = lg.Results[n-1].Round
		for _, r := range lg.Results {
			if r.Round == v.Played {
				v.Last = append(v.Last, r)
			}
		}
	}
	v.Points = lg.Mine().Pts
	return v
}
