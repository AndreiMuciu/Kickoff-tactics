package game

// After the league a top-four club plays on in Europe, with the same squad,
// budget and stadium: the champion in the European Crown, places 2-3 in the
// European Shield, 4th in the European Plate. Each cup has its own clubs and
// its own level. A group of four (three matchdays; the other group game is
// told, like the league's), the top two go through to a semi-final and a
// final, settled on the day (penalties if level). Europe costs no patience:
// a defeat only ends the run, and the cup's outcome is its epilogue.

const (
	CupGroupDays = 3                // group matchdays
	CupMatches   = CupGroupDays + 2 // plus a semi-final and a final
	CupThrough   = 2                // group places that go through
)

type CupDef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stars int    `json:"stars"`
	// The three group rivals (weakest first), the semi-final and the final opponent.
	Clubs [CupMatches]string `json:"clubs"`
	// Opponents' strength in the group, the semi-final and the final: outfield
	// players, dearest cost, two-star chance (%), three-star chance for cheap players (%).
	count, maxCost, star2, star3 [3]int
	keepers                      [3]string
}

// Tuned with TestBalanceSoloSeason.
var Cups = []CupDef{
	{ID: "crown", Name: "European Crown", Stars: 3,
		Clubs: [CupMatches]string{"Borussia Elmhorst", "Sporting Valmarés", "Inter Valdoria", "Athletic Montemayor", "Real Castellmar"},
		count: [3]int{6, 6, 6}, maxCost: [3]int{4, 4, 5}, star2: [3]int{50, 60, 60}, star3: [3]int{10, 15, 15},
		keepers: [3]string{"courtois", "oblak", "buffon"}},
	{ID: "shield", Name: "European Shield", Stars: 2,
		Clubs: [CupMatches]string{"Union Saint-Aubert", "Dinamo Kraljevac", "SV Rheinau", "Atlético Montoro", "Olympique Varenne"},
		count: [3]int{6, 6, 6}, maxCost: [3]int{3, 4, 4}, star2: [3]int{45, 55, 65}, star3: [3]int{10, 10, 15},
		keepers: [3]string{"alisson", "kahn", "vandersar"}},
	{ID: "plate", Name: "European Plate", Stars: 1,
		Clubs: [CupMatches]string{"FK Dravica", "KS Wisłanka", "Sporting Alvorada", "Rapid Lindenau", "Hellas Kalamos"},
		count: [3]int{5, 5, 6}, maxCost: [3]int{3, 4, 4}, star2: [3]int{40, 45, 50}, star3: [3]int{10, 10, 10},
		keepers: [3]string{"navas", "neuer", "ederson"}},
}

var CupByID = func() map[string]*CupDef {
	m := map[string]*CupDef{}
	for i := range Cups {
		m[Cups[i].ID] = &Cups[i]
	}
	return m
}()

// cupFor is the cup a final league position qualifies for, or "".
func cupFor(pos int) string {
	switch {
	case pos == 1:
		return "crown"
	case pos >= 2 && pos <= 3:
		return "shield"
	case pos == EuropePlaces:
		return "plate"
	}
	return ""
}

// Cup is a club's run in Europe.
type Cup struct {
	ID    string
	Group *League
	Out   string `bson:",omitempty"` // once over: group | semi | final | winner
}

func newCup(g *RNG, id string) *Cup {
	return &Cup{ID: id, Group: newTable(g, CupByID[id].Clubs[:CupGroupDays])}
}

// cupMatch is the number (1..CupMatches) of the current European match.
func (r *Run) cupMatch() int { return r.Round - MaxRounds }

func cupStage(m int) string {
	switch {
	case m <= CupGroupDays:
		return "group"
	case m == CupGroupDays+1:
		return "semi"
	}
	return "final"
}

// cupOpponent is the club met in European match m.
func (r *Run) cupOpponent(g *RNG, m int) Opponent {
	d := CupByID[r.Cup.ID]
	s := 0
	if m > CupGroupDays {
		s = m - CupGroupDays
	}
	return genTeam(g, d.Clubs[m-1], d.keepers[s], d.count[s], d.maxCost[s], d.star2[s], d.star3[s])
}

// cupAfter judges European match m once played: how the run ends, or "" if
// it goes on.
func (r *Run) cupAfter(m int, won bool) string {
	switch {
	case m < CupGroupDays:
		return ""
	case m == CupGroupDays:
		if r.Cup.Group.Position() > CupThrough {
			return "group"
		}
		return ""
	case won && m < CupMatches:
		return ""
	case won:
		return "winner"
	}
	return cupStage(m)
}

type CupView struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Stars int         `json:"stars"`
	Match int         `json:"match"` // the current (or last) European match, 1..CupMatches
	Stage string      `json:"stage"` // group | semi | final
	Out   string      `json:"out,omitempty"`
	Group *LeagueView `json:"group"`
}

func (r *Run) cupView() *CupView {
	if r.Cup == nil {
		return nil
	}
	d := CupByID[r.Cup.ID]
	m := min(max(r.cupMatch(), 1), CupMatches)
	v := &CupView{ID: d.ID, Name: d.Name, Stars: d.Stars, Match: m, Stage: cupStage(m), Out: r.Cup.Out, Group: r.Cup.Group.view()}
	v.Group.Europe, v.Group.Relegate = CupThrough, 0
	return v
}
