package game

import (
	"fmt"
	"sort"
)

// After the league the run goes on (rounds 10+), with the same squad, budget
// and stadium.
//
// A top-four club plays in Europe: the champion in the European Crown, 2nd and
// 3rd in the European Shield, 4th in the European Plate. Each cup draws its 15
// opponents at random from its own pool of clubs (invented clubs in real
// cities, the country in brackets): the Crown from the strongest football
// countries, the Shield from a few of those and many mid-level ones, the Plate
// from the smallest countries with one or two clubs from big ones. Four groups
// of four over three matchdays; the top two of each go to the quarter-finals,
// then semi-finals and a final, settled on the night (penalties if level).
// Only your matches are played; every other match in the cup is told, so all
// groups and the whole bracket move as you go. Europe costs no patience: a
// knockout defeat, or a group finish outside the top two, ends the run.
//
// A relegated club gets one last chance: a play-off mini group with three
// second-division clubs. The owner's patience drops to 1, so any defeat is the
// sack; only first place brings the club straight back up.

const (
	CupGroupDays = 3 // group matchdays
	cupGroups    = 4
	CupThrough   = 2 // group places that go through
	PlayoffID    = "playoff"
)

// A club in a cup's pool. Its rating (0-10) sets its line-up and its told results.
type cupClub struct {
	name, country string
	rating        float64
}

type CupDef struct {
	ID      string
	Name    string
	Stars   int
	Playoff bool
	pool    []cupClub
}

var Cups = []CupDef{
	{ID: "crown", Name: "European Crown", Stars: 3, pool: []cupClub{
		{"Athletic Manchester", "England", 9.5}, {"London Wanderers", "England", 9}, {"Liverpool Rovers", "England", 8.5}, {"Newcastle Athletic", "England", 7},
		{"Deportivo Madrid", "Spain", 9.5}, {"Atlético Barcelona", "Spain", 9}, {"Racing Sevilla", "Spain", 7.5}, {"Unión Valencia", "Spain", 7},
		{"Sporting Milano", "Italy", 9}, {"Virtus Torino", "Italy", 8}, {"Ardor Napoli", "Italy", 8}, {"Fortitudo Roma", "Italy", 7.5},
		{"Union München", "Germany", 9.5}, {"Borussia Köln", "Germany", 8}, {"Concordia Hamburg", "Germany", 7},
		{"Olympique Paris", "France", 9}, {"Racing Lyon", "France", 7.5}, {"Stade Marseille", "France", 7.5},
		{"União Lisboa", "Portugal", 8}, {"Sporting Porto", "Portugal", 7.5},
		{"Sparta Amsterdam", "Netherlands", 7.5}, {"Victoria Eindhoven", "Netherlands", 7},
		{"Racing Gent", "Belgium", 6.5}, {"Anadolu Istanbul", "Turkey", 7}, {"Bohemia Praha", "Czechia", 6.5},
	}},
	{ID: "shield", Name: "European Shield", Stars: 2, pool: []cupClub{
		{"Leeds Wanderers", "England", 6.5}, {"Deportivo Bilbao", "Spain", 6.5}, {"Pro Genova", "Italy", 6}, {"Rot-Weiss Stuttgart", "Germany", 6},
		{"Stade Bordeaux", "France", 5.5}, {"União Braga", "Portugal", 5.5}, {"Racing Liège", "Belgium", 5.5}, {"Ankara Gençlik", "Turkey", 5.5},
		{"Sparta Kraków", "Poland", 4.5}, {"Górnik Wrocław", "Poland", 4}, {"Progresul Cluj", "Romania", 4.5}, {"Victoria Timișoara", "Romania", 4},
		{"Vasas Debrecen", "Hungary", 4}, {"Ethnikos Thessaloniki", "Greece", 5}, {"Admira Graz", "Austria", 5}, {"Aberdeen Athletic", "Scotland", 4.5},
		{"Lausanne Olympic", "Switzerland", 4.5}, {"Dinamo Novi Sad", "Serbia", 4.5}, {"Jadran Split", "Croatia", 5}, {"Fremad Aarhus", "Denmark", 4.5},
		{"Fram Bergen", "Norway", 4}, {"Metalist Lviv", "Ukraine", 4.5}, {"Brage Malmö", "Sweden", 4}, {"Slavoj Brno", "Czechia", 5},
	}},
	{ID: "plate", Name: "European Plate", Stars: 1, pool: []cupClub{
		{"Codru Chișinău", "Moldova", 3.5}, {"Nistru Tiraspol", "Moldova", 4}, {"Esportiu Andorra la Vella", "Andorra", 2}, {"Virtus Serravalle", "San Marino", 2},
		{"Europa Point", "Gibraltar", 2.5}, {"Marsa Rovers", "Malta", 3}, {"Tórshavn United", "Faroe Islands", 2.5}, {"Racing Esch", "Luxembourg", 3},
		{"Prishtina United", "Kosovo", 3.5}, {"Dinamo Durrës", "Albania", 3.5}, {"Skopje City", "North Macedonia", 3.5}, {"Podgorica Sport", "Montenegro", 3.5},
		{"Yerevan Spartak", "Armenia", 3.5}, {"Tbilisi Sports", "Georgia", 4}, {"Steppe Almaty", "Kazakhstan", 4}, {"Caspian Baku", "Azerbaijan", 4.5},
		{"Neris Vilnius", "Lithuania", 3}, {"Riga Baltic", "Latvia", 3}, {"Tallinna Merkuur", "Estonia", 3}, {"Reykjavík Ungmenna", "Iceland", 3.5},
		{"Llanelli Athletic", "Wales", 3}, {"Derry Rangers", "Northern Ireland", 3}, {"Paphos Ethnikos", "Cyprus", 4.5}, {"Slovan Celje", "Slovenia", 4.5},
		{"Iskra Sofia", "Bulgaria", 4.5}, {"Borac Mostar", "Bosnia and Herzegovina", 4.5},
		{"Ciudad Albacete", "Spain", 6.5}, {"Sportivo Bari", "Italy", 6},
	}},
	{ID: PlayoffID, Name: "Promotion play-off", Playoff: true, pool: []cupClub{
		{"Allotment Athletic", "", 1}, {"Laundrette Rovers", "", 1.5}, {"Canal Street FC", "", 2},
		{"Bypass Town", "", 2.5}, {"Retail Park United", "", 3}, {"Pylon Wanderers", "", 3.5},
	}},
}

var CupByID = func() map[string]*CupDef {
	m := map[string]*CupDef{}
	for i := range Cups {
		m[Cups[i].ID] = &Cups[i]
	}
	return m
}()

// cupFor is where a final league position sends the club next, or "".
func cupFor(pos int) string {
	switch {
	case pos == 1:
		return "crown"
	case pos == ShieldPlace:
		return "shield"
	case pos > ShieldPlace && pos <= EuropePlaces:
		return "plate"
	case pos >= RelegatedPlace:
		return PlayoffID
	}
	return ""
}

func (c cupClub) label() string {
	if c.country == "" {
		return c.name
	}
	return fmt.Sprintf("%s (%s)", c.name, c.country)
}

// Tie is a knockout match. Stage: quarter | semi | final.
type Tie struct {
	Stage  string `json:"stage"`
	Home   string `json:"home"`
	Away   string `json:"away"`
	HG     int    `json:"hg"`
	AG     int    `json:"ag"`
	Pens   bool   `json:"pens,omitempty"`
	Winner string `json:"winner,omitempty"` // empty until played
}

// Cup is the run after the league: Europe, or the play-off.
type Cup struct {
	ID     string
	Groups []*League // Groups[Mine] holds your club, at index 0
	Mine   int
	Ties   []Tie  `bson:",omitempty"` // 4 quarter-finals, 2 semi-finals, the final
	Out    string `bson:",omitempty"` // once over: group | quarter | semi | final | winner; play-off: promoted | stayed | sacked
}

var knockoutStages = []string{"quarter", "semi", "final"}

// matches is how many matches the run after the league can last.
func (c *Cup) matches() int {
	if CupByID[c.ID].Playoff {
		return CupGroupDays
	}
	return CupGroupDays + len(knockoutStages)
}

func newCup(g *RNG, id string) *Cup {
	d := CupByID[id]
	pool := append([]cupClub(nil), d.pool...)
	for i := len(pool) - 1; i > 0; i-- {
		j := g.Intn(i + 1)
		pool[i], pool[j] = pool[j], pool[i]
	}
	groups := 1
	if !d.Playoff {
		groups = cupGroups
	}
	c := &Cup{ID: id, Mine: g.Intn(groups)}
	k := 0
	for gi := 0; gi < groups; gi++ {
		lg := &League{}
		if gi == c.Mine {
			lg.Teams = append(lg.Teams, LeagueTeam{Name: YourClub, You: true})
		}
		for len(lg.Teams) < 4 {
			// A little form for this season around the club's usual level.
			lg.Teams = append(lg.Teams, LeagueTeam{Name: pool[k].label(), Strength: pool[k].rating + (g.Float()-0.5)*1.5})
			k++
		}
		c.Groups = append(c.Groups, lg)
	}
	return c
}

// cupMatch is the number of the current match after the league (1..).
func (r *Run) cupMatch() int { return r.Round - MaxRounds }

func cupStage(m int) string {
	if m <= CupGroupDays {
		return "group"
	}
	return knockoutStages[min(m-CupGroupDays, len(knockoutStages))-1]
}

func stageIndex(stage string) int {
	for i, s := range knockoutStages {
		if s == stage {
			return i
		}
	}
	return -1
}

func (c *Cup) strength(name string) float64 {
	for _, lg := range c.Groups {
		for _, t := range lg.Teams {
			if t.Name == name {
				return t.Strength
			}
		}
	}
	return 0
}

// tellGroups plays matchday m in every group but yours: told, not simulated.
func (c *Cup) tellGroups(g *RNG, m int) {
	for gi, lg := range c.Groups {
		if gi == c.Mine {
			continue
		}
		for _, p := range fixtures(len(lg.Teams), m) {
			hg, ag := toldScore(g, lg.Teams[p[0]].Strength, lg.Teams[p[1]].Strength)
			lg.record(m, p[0], p[1], hg, ag)
		}
	}
}

func (c *Cup) drawn(stage string) bool {
	for _, t := range c.Ties {
		if t.Stage == stage {
			return true
		}
	}
	return false
}

// draw makes the ties of a knockout stage: quarter-finals A1-B2, B1-A2,
// C1-D2, D1-C2; semi-finals the winners of the 1st and 3rd, 2nd and 4th
// quarter-finals; then the final.
func (c *Cup) draw(stage string) {
	if c.drawn(stage) {
		return
	}
	if stage == "quarter" {
		tab := make([][]LeagueTeam, len(c.Groups))
		for i, lg := range c.Groups {
			tab[i] = lg.Table()
		}
		for _, p := range [][2]int{{0, 1}, {1, 0}, {2, 3}, {3, 2}} {
			c.Ties = append(c.Ties, Tie{Stage: stage, Home: tab[p[0]][0].Name, Away: tab[p[1]][1].Name})
		}
		return
	}
	var prev []Tie
	for _, t := range c.Ties {
		if t.Stage == knockoutStages[stageIndex(stage)-1] {
			prev = append(prev, t)
		}
	}
	if stage == "semi" {
		c.Ties = append(c.Ties, Tie{Stage: stage, Home: prev[0].Winner, Away: prev[2].Winner}, Tie{Stage: stage, Home: prev[1].Winner, Away: prev[3].Winner})
		return
	}
	c.Ties = append(c.Ties, Tie{Stage: stage, Home: prev[0].Winner, Away: prev[1].Winner})
}

// settle records a tie's score; a level tie is won on penalties by the home
// side when homePens is true.
func settle(t *Tie, hg, ag int, homePens bool) {
	t.HG, t.AG = hg, ag
	switch {
	case hg > ag:
		t.Winner = t.Home
	case ag > hg:
		t.Winner = t.Away
	default:
		t.Pens = true
		t.Winner = t.Away
		if homePens {
			t.Winner = t.Home
		}
	}
}

// tellStage tells every unplayed tie of a stage that does not involve you;
// in a shootout the stronger side is a little more likely to win.
func (c *Cup) tellStage(g *RNG, stage string) {
	for i := range c.Ties {
		t := &c.Ties[i]
		if t.Stage != stage || t.Winner != "" || t.Home == YourClub || t.Away == YourClub {
			continue
		}
		a, b := c.strength(t.Home), c.strength(t.Away)
		hg, ag := toldScore(g, a, b)
		settle(t, hg, ag, g.Float() < 0.5+(a-b)*0.03)
	}
}

// tellRest finishes the cup without you, so the story can say who won it.
func (c *Cup) tellRest(g *RNG, from string) {
	for i := max(stageIndex(from), 0); i < len(knockoutStages); i++ {
		c.draw(knockoutStages[i])
		c.tellStage(g, knockoutStages[i])
	}
}

// yourTie is your tie in a knockout stage, or nil.
func (c *Cup) yourTie(stage string) *Tie {
	for i := range c.Ties {
		if t := &c.Ties[i]; t.Stage == stage && (t.Home == YourClub || t.Away == YourClub) {
			return t
		}
	}
	return nil
}

// cupOpponent is the club met in match m after the league.
func (r *Run) cupOpponent(g *RNG, m int) Opponent {
	c := r.Cup
	if CupByID[c.ID].Playoff {
		// Second-division clubs, as strong as the league's later fixtures.
		o := genOpponent(g, playoffFixture+m-1)
		o.Name = c.Groups[0].Teams[m].Name
		return o
	}
	var name string
	if m <= CupGroupDays {
		name = c.Groups[c.Mine].Teams[m].Name
	} else if t := c.yourTie(cupStage(m)); t.Home == YourClub {
		name = t.Away
	} else {
		name = t.Home
	}
	return ratedTeam(g, name, c.strength(name))
}

// cupKeepers, weakest first: a European club's keeper follows its rating.
var cupKeepers = []string{"pickford", "onana", "emi", "navas", "raya", "neuer", "alisson", "ederson", "kahn", "donnarumma", "vandersar", "courtois", "oblak", "buffon"}

// Tuned with TestBalanceSoloSeason: how a rating turns into a line-up, and
// which league fixture the play-off's first opponent matches.
var (
	cupStar2Base, cupStar2Step = 38.0, 3.5
	playoffFixture             = 6
)

// ratedTeam builds a European club's line-up from its rating (0-10).
func ratedTeam(g *RNG, name string, rating float64) Opponent {
	rt := min(max(rating, 0), 10)
	count := 5 + int(rt/4.5)
	maxCost := min(3+int(rt/4), 5)
	star2 := int(cupStar2Base + cupStar2Step*rt)
	star3 := int(5 + 1.5*rt)
	keeper := cupKeepers[min(int(rt*float64(len(cupKeepers))/10.5), len(cupKeepers)-1)]
	return genTeam(g, name, keeper, count, maxCost, star2, star3)
}

// playCup applies your match m after the league and tells everything else
// that happened around it. It returns how the run ends, or "" if it goes on.
func (r *Run) playCup(g *RNG, m int, h HistoryEntry) string {
	c := r.Cup
	if m <= CupGroupDays {
		c.Groups[c.Mine].playRound(g, m, h.GF, h.GA)
		c.tellGroups(g, m)
		if m < CupGroupDays {
			return ""
		}
		pos := c.Groups[c.Mine].Position()
		if CupByID[c.ID].Playoff {
			if pos == 1 {
				return "promoted"
			}
			return "stayed"
		}
		c.draw("quarter")
		if pos > CupThrough {
			c.tellRest(g, "quarter")
			return "group"
		}
		return ""
	}
	// The other ties of the stage are played the same night as yours.
	stage := cupStage(m)
	c.tellStage(g, stage)
	t := c.yourTie(stage)
	hg, ag := h.GF, h.GA
	if t.Away == YourClub {
		hg, ag = ag, hg
	}
	settle(t, hg, ag, (t.Home == YourClub) == h.Won)
	if !h.Won {
		c.tellRest(g, stage)
		return stage
	}
	if stage == "final" {
		return "winner"
	}
	c.draw(knockoutStages[stageIndex(stage)+1])
	return ""
}

type GroupView struct {
	Letter string         `json:"letter"`
	Table  []LeagueTeam   `json:"table"`
	Last   []LeagueResult `json:"last"`
}

type CupView struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Stars   int         `json:"stars"`
	Playoff bool        `json:"playoff,omitempty"`
	Match   int         `json:"match"`   // the current (or last) match after the league, 1..matches
	Matches int         `json:"matches"` // how many it can last
	Stage   string      `json:"stage"`   // group | quarter | semi | final
	Out     string      `json:"out,omitempty"`
	Group   *LeagueView `json:"group"` // yours
	Groups  []GroupView `json:"groups"`
	Mine    int         `json:"mine"`
	Ties    []Tie       `json:"ties"`
	Winner  string      `json:"winner,omitempty"` // who lifted the cup, once known
}

func (r *Run) cupView() *CupView {
	c := r.Cup
	if c == nil {
		return nil
	}
	d := CupByID[c.ID]
	m := min(max(r.cupMatch(), 1), c.matches())
	v := &CupView{ID: d.ID, Name: d.Name, Stars: d.Stars, Playoff: d.Playoff, Match: m, Matches: c.matches(), Stage: cupStage(m), Out: c.Out,
		Group: c.Groups[c.Mine].view(), Mine: c.Mine, Ties: append([]Tie{}, c.Ties...)}
	v.Group.Europe, v.Group.Relegate = CupThrough, 0
	if d.Playoff {
		v.Group.Europe = 1
	}
	for i, lg := range c.Groups {
		v.Groups = append(v.Groups, GroupView{Letter: string(rune('A' + i)), Table: lg.Table(), Last: lg.view().Last})
	}
	sort.SliceStable(v.Ties, func(a, b int) bool { return stageIndex(v.Ties[a].Stage) < stageIndex(v.Ties[b].Stage) })
	for _, t := range c.Ties {
		if t.Stage == "final" && t.Winner != "" {
			v.Winner = t.Winner
		}
	}
	return v
}
