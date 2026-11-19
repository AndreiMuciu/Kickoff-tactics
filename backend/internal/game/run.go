package game

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
)

const (
	MaxRounds     = 8
	BenchSize     = 8
	ShopSize      = 5
	MaxLevel      = 7
	RerollCost    = 2
	BaseIncome    = 5  // budget received after every match
	InterestStep  = 10 // every this much budget kept in the bank...
	InterestMax   = 5  // ...adds one to the income, up to this many
	XPCost        = 4
	XPPerBuy      = 4
	StartPatience = 20
)

// xpNeed[level] is the XP required to go from level to level+1.
var xpNeed = [MaxLevel + 1]int{0, 2, 2, 4, 6, 8, 10, 0}

// shopOdds[level] is the percentage chance per cost tier 1..5. Cost-5 legends
// only appear from stadium level 5, and stay rare.
var shopOdds = [MaxLevel + 1][5]int{{}, {100, 0, 0, 0, 0}, {80, 20, 0, 0, 0}, {60, 30, 10, 0, 0}, {45, 33, 20, 2, 0}, {30, 35, 28, 6, 1}, {20, 30, 35, 13, 2}, {12, 25, 35, 22, 6}}

// ErrRule is returned for any intent the rules reject. Its message is safe to show.
type ErrRule struct{ Msg string }

func (e ErrRule) Error() string { return e.Msg }

func rule(format string, a ...any) error { return ErrRule{fmt.Sprintf(format, a...)} }

type Unit struct {
	UID   string `json:"uid"`
	Def   string `json:"def"`
	Stars int    `json:"stars"`
	Line  int    `json:"line"` // -1 on the bench
	Lane  int    `json:"lane"`
	Ban   int    `json:"ban"` // matches of suspension left after a red card
}

type Opponent struct {
	Name  string `json:"name"`
	Units []Unit `json:"units"` // includes the keeper at GKLine
}

// MatchState lives only between kickoff and the final whistle. Seed is secret
// until the match ends; Commit (its SHA-256) is shown to the client up front.
type MatchState struct {
	Seed   string
	Commit string
	My     Lineup
	Opp    Lineup
}

type HistoryEntry struct {
	Round    int    `json:"round"`
	Opponent string `json:"opponent"`
	GF       int    `json:"gf"`
	GA       int    `json:"ga"`
	Pens     bool   `json:"pens"`
	Won      bool   `json:"won"`
	Damage   int    `json:"damage"`
	Income   int    `json:"income"`
	Seed     string `json:"seed"`
	Commit   string `json:"commit"`
}

// Run is the full server-side state of one season. It is never serialised to
// the client directly; View() builds the public projection.
type Run struct {
	SessionID string `bson:"_id"`
	Version   int64
	Phase     string // shop | halftime | ended
	Round     int
	Patience  int
	Budget    int
	Level     int
	XP        int
	Shop      []string
	Units     []Unit
	Opponent  Opponent
	Streak    int
	Wins      int
	Losses    int
	Match     *MatchState
	History   []HistoryEntry
	Result    string
	NextUID   int
	RNGState  int64
}

type Action struct {
	Type string `json:"type"`
	Slot int    `json:"slot"`
	Def  string `json:"def"` // buy: the player the manager saw in that slot
	UID  string `json:"uid"`
	Line int    `json:"line"`
	Lane int    `json:"lane"`
}

func randomSeed() (uint64, string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, "", err
	}
	return binary.BigEndian.Uint64(b[:8]) ^ binary.BigEndian.Uint64(b[8:]), hex.EncodeToString(b[:]), nil
}

func seedFromHex(h string) (uint64, error) {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 16 {
		return 0, errors.New("bad seed")
	}
	return binary.BigEndian.Uint64(b[:8]) ^ binary.BigEndian.Uint64(b[8:]), nil
}

func NewRun(sessionID string) (*Run, error) {
	seed, _, err := randomSeed()
	if err != nil {
		return nil, err
	}
	r := &Run{SessionID: sessionID, Phase: "shop", Round: 1, Patience: StartPatience, Budget: 6, Level: 1, RNGState: int64(seed), NextUID: 1}
	// Every club starts with an academy keeper already in goal.
	r.Units = []Unit{{UID: "u1", Def: AcademyKeeper, Stars: 1, Line: GKLine, Lane: 0}}
	r.withRNG(func(g *RNG) {
		r.rollShop(g)
		r.Opponent = genOpponent(g, 1)
	})
	return r, nil
}

func (r *Run) withRNG(f func(g *RNG)) {
	g := NewRNG(uint64(r.RNGState))
	f(g)
	r.RNGState = int64(g.S)
}

// Slots is the number of outfield players allowed: one per stadium level, plus the keeper.
func (r *Run) Slots() int { return r.Level }

func (r *Run) rollShop(g *RNG) {
	byCost := map[int][]string{}
	for _, p := range Players {
		byCost[p.Cost] = append(byCost[p.Cost], p.ID)
	}
	r.Shop = make([]string, ShopSize)
	for i := range r.Shop {
		roll, cost := g.Intn(100), 1
		for c, pct := range shopOdds[r.Level] {
			if roll < pct {
				cost = c + 1
				break
			}
			roll -= pct
		}
		pool := byCost[cost]
		r.Shop[i] = pool[g.Intn(len(pool))]
	}
}

var clubNames = [MaxRounds]string{"Sunday League FC", "Car Park Rovers", "Garage Athletic", "Kiosk United", "Stairwell Sporting", "Roundabout Olympic", "Valley Galacticos", "Imperial XI"}

func genOpponent(g *RNG, round int) Opponent {
	i := round - 1
	count := [MaxRounds]int{1, 2, 3, 3, 4, 5, 6, 7}[i]
	maxCost := [MaxRounds]int{1, 1, 2, 2, 3, 3, 4, 5}[i]
	star2 := [MaxRounds]int{0, 10, 20, 30, 45, 55, 70, 85}[i]
	var styles []string
	for _, sy := range Synergies {
		if sy.Kind == "style" {
			styles = append(styles, sy.ID)
		}
	}
	theme := styles[g.Intn(len(styles))]
	var themed, rest []string
	for _, p := range Players {
		if p.Cost > maxCost || p.Role == "gk" {
			continue
		}
		if p.Style == theme {
			themed = append(themed, p.ID)
		} else {
			rest = append(rest, p.ID)
		}
	}
	pick := func(pool *[]string) string {
		k := g.Intn(len(*pool))
		id := (*pool)[k]
		*pool = append((*pool)[:k], (*pool)[k+1:]...)
		return id
	}
	var ids []string
	for len(ids) < count {
		if len(themed) > 0 && (len(rest) == 0 || g.Intn(100) < 65) {
			ids = append(ids, pick(&themed))
		} else if len(rest) > 0 {
			ids = append(ids, pick(&rest))
		} else {
			break
		}
	}
	// Place by role: defenders deep, strikers high, wingers wide.
	taken := map[[2]int]bool{}
	place := func(line int, lanes []int) (int, int) {
		for _, l := range []int{line, 1, 0, 2} {
			for _, ln := range lanes {
				if !taken[[2]int{l, ln}] {
					taken[[2]int{l, ln}] = true
					return l, ln
				}
			}
		}
		return 0, 0
	}
	op := Opponent{Name: clubNames[i]}
	keeper := [MaxRounds]string{AcademyKeeper, AcademyKeeper, "pickford", "onana", "emi", "neuer", "courtois", "donnarumma"}[i]
	op.Units = append(op.Units, Unit{UID: "ogk", Def: keeper, Stars: 1, Line: GKLine, Lane: 0})
	for n, id := range ids {
		d := PlayerByID[id]
		var line, lane int
		switch d.Role {
		case "defender":
			line, lane = place(0, []int{2, 1, 3, 0, 4})
		case "anchor":
			line, lane = place(1, []int{2, 1, 3, 0, 4})
		case "playmaker":
			line, lane = place(1, []int{2, 3, 1, 4, 0})
		case "winger":
			line, lane = place(2, []int{0, 4, 1, 3, 2})
		default:
			line, lane = place(2, []int{2, 1, 3, 0, 4})
		}
		stars := 1
		if g.Intn(100) < star2 {
			stars = 2
		}
		if round >= 7 && d.Cost <= 2 && g.Intn(100) < 25 {
			stars = 3
		}
		op.Units = append(op.Units, Unit{UID: fmt.Sprintf("o%d", n), Def: id, Stars: stars, Line: line, Lane: lane})
	}
	return op
}

func (r *Run) find(uid string) *Unit {
	for i := range r.Units {
		if r.Units[i].UID == uid {
			return &r.Units[i]
		}
	}
	return nil
}

func (r *Run) counts() (bench, board int) {
	for _, u := range r.Units {
		switch {
		case u.Line < 0:
			bench++
		case u.Line <= 2:
			board++
		}
	}
	return
}

func (r *Run) freeBenchSlot() int {
	used := map[int]bool{}
	for _, u := range r.Units {
		if u.Line < 0 {
			used[u.Lane] = true
		}
	}
	for i := 0; i < BenchSize; i++ {
		if !used[i] {
			return i
		}
	}
	return -1
}

// WarmupClub is the small club a manager meets in a warm-up round of a lobby,
// before the managers start playing each other.
func WarmupClub(g *RNG, round int) Lineup {
	o := genOpponent(g, min(max(round, 1), MaxRounds))
	return lineupOf(o.Name, o.Units)
}

// Interest is the extra income for budget kept in the bank, counted on what
// the club holds when the match ends.
func Interest(budget int) int { return min(max(budget, 0)/InterestStep, InterestMax) }

// merge combines three identical units into one of the next star level.
func (r *Run) merge() {
	for changed := true; changed; {
		changed = false
		groups := map[string][]int{}
		for i, u := range r.Units {
			if u.Stars < 3 {
				k := fmt.Sprintf("%s/%d", u.Def, u.Stars)
				groups[k] = append(groups[k], i)
			}
		}
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			idx := groups[k]
			if len(idx) < 3 {
				continue
			}
			idx = idx[:3]
			keep := idx[0]
			for _, i := range idx {
				if r.Units[i].Line >= 0 {
					keep = i
					break
				}
			}
			for _, i := range idx {
				r.Units[keep].Ban = max(r.Units[keep].Ban, r.Units[i].Ban)
			}
			r.Units[keep].Stars++
			var out []Unit
			for i, u := range r.Units {
				if i != keep && (i == idx[0] || i == idx[1] || i == idx[2]) {
					continue
				}
				out = append(out, u)
			}
			r.Units = out
			changed = true
			break
		}
	}
}

func sellValue(u Unit) int {
	c := PlayerByID[u.Def].Cost
	if c == 0 {
		return 0
	}
	switch u.Stars {
	case 2:
		return c*3 - 1
	case 3:
		return c*9 - 1
	}
	return c
}

func (r *Run) gainXP(n int) {
	r.XP += n
	for r.Level < MaxLevel && r.XP >= xpNeed[r.Level] {
		r.XP -= xpNeed[r.Level]
		r.Level++
	}
	if r.Level == MaxLevel {
		r.XP = 0
	}
}

// Apply validates and executes one management intent during the shop phase.
func (r *Run) Apply(a Action) error {
	if r.Phase != "shop" {
		return rule("You cannot do that right now.")
	}
	switch a.Type {
	case "buy":
		if a.Slot < 0 || a.Slot >= len(r.Shop) || r.Shop[a.Slot] == "" {
			return rule("Invalid market slot.")
		}
		// The client says who it thinks it is buying. If its market is out of
		// date (another tab rerolled, a new season began) nothing is bought.
		if a.Def != "" && a.Def != r.Shop[a.Slot] {
			return rule("The market has changed. Look again before you buy.")
		}
		d := PlayerByID[r.Shop[a.Slot]]
		if r.Budget < d.Cost {
			return rule("Not enough budget.")
		}
		slot := r.freeBenchSlot()
		if slot < 0 {
			// A full bench is fine when the purchase completes a merge.
			same := 0
			for _, u := range r.Units {
				if u.Def == d.ID && u.Stars == 1 {
					same++
				}
			}
			if same < 2 {
				return rule("Your bench is full.")
			}
			slot = BenchSize
		}
		r.Budget -= d.Cost
		r.Shop[a.Slot] = ""
		r.NextUID++
		r.Units = append(r.Units, Unit{UID: fmt.Sprintf("u%d", r.NextUID), Def: d.ID, Stars: 1, Line: -1, Lane: slot})
		r.merge()
	case "sell":
		u := r.find(a.UID)
		if u == nil {
			return rule("No such player.")
		}
		if u.Line == GKLine {
			return rule("You always need a goalkeeper. Swap another keeper into goal before selling this one.")
		}
		r.Budget += sellValue(*u)
		var out []Unit
		for _, x := range r.Units {
			if x.UID != a.UID {
				out = append(out, x)
			}
		}
		r.Units = out
	case "move":
		u := r.find(a.UID)
		if u == nil {
			return rule("No such player.")
		}
		switch {
		case a.Line == -1:
			if a.Lane < 0 || a.Lane >= BenchSize {
				return rule("Invalid position.")
			}
		case a.Line == GKLine:
			if a.Lane != 0 {
				return rule("Invalid position.")
			}
		case a.Line < 0 || a.Line > 2 || a.Lane < 0 || a.Lane > 4:
			return rule("Invalid position.")
		}
		var other *Unit
		for i := range r.Units {
			x := &r.Units[i]
			if x.UID != u.UID && x.Line == a.Line && x.Lane == a.Lane {
				other = x
			}
		}
		keeper := IsGK(u.Def)
		outfield := func(line int) bool { return line >= 0 && line <= 2 }
		// A suspended player may sit on the bench but cannot be brought on.
		if u.Ban > 0 && u.Line < 0 && a.Line >= 0 {
			return rule("%s is suspended for %d more match(es).", PlayerByID[u.Def].Name, u.Ban)
		}
		if other != nil && other.Ban > 0 && other.Line < 0 && u.Line >= 0 {
			return rule("%s is suspended for %d more match(es).", PlayerByID[other.Def].Name, other.Ban)
		}
		switch {
		case a.Line == GKLine && !keeper:
			return rule("Only a goalkeeper can stand in goal.")
		case outfield(a.Line) && keeper:
			return rule("A goalkeeper can only play in goal.")
		case u.Line == GKLine && a.Line != GKLine && (other == nil || !IsGK(other.Def)):
			// The goal is never left empty: a keeper only leaves by swapping with another keeper.
			return rule("You always need a goalkeeper. Drop another keeper onto the goal to swap.")
		case other != nil && outfield(u.Line) && IsGK(other.Def):
			return rule("A goalkeeper can only play in goal.")
		}
		if other == nil && u.Line < 0 && outfield(a.Line) {
			if _, board := r.counts(); board >= r.Slots() {
				return rule("Your stadium level allows %d outfield player(s). Upgrade the stadium to field more.", r.Slots())
			}
		}
		if other != nil {
			other.Line, other.Lane = u.Line, u.Lane
		}
		u.Line, u.Lane = a.Line, a.Lane
	case "reroll":
		if r.Budget < RerollCost {
			return rule("Not enough budget.")
		}
		r.Budget -= RerollCost
		r.withRNG(r.rollShop)
	case "levelup":
		if r.Level >= MaxLevel {
			return rule("The stadium is already at maximum level.")
		}
		if r.Budget < XPCost {
			return rule("Not enough budget.")
		}
		r.Budget -= XPCost
		r.gainXP(XPPerBuy)
	default:
		return rule("Unknown action.")
	}
	return nil
}

func lineupOf(name string, units []Unit) Lineup {
	lu := Lineup{Name: name, GK: LineupUnit{DefID: AcademyKeeper, Stars: 1}}
	for _, u := range units {
		switch {
		case u.Line == GKLine:
			lu.GK = LineupUnit{DefID: u.Def, Stars: u.Stars}
		case u.Line >= 0 && u.Ban == 0:
			lu.Units = append(lu.Units, LineupUnit{DefID: u.Def, Stars: u.Stars, Line: u.Line, Lane: u.Lane})
		}
	}
	return lu
}

func (r *Run) myLineup() Lineup { return lineupOf("Your Club", r.Units) }

// StartMatch freezes both lineups, draws a secret seed and plays the first half.
func (r *Run) StartMatch() (*SimResult, error) {
	if r.Phase != "shop" {
		return nil, rule("The match cannot start right now.")
	}
	r.AutoField()
	my := r.myLineup()
	if len(my.Units) == 0 && r.hasEligible() {
		return nil, rule("Put at least one outfield player who is not suspended on the pitch.")
	}
	// With nobody eligible in the whole squad (all suspended, or none signed)
	// the match still goes ahead with the keeper alone, so a season can never
	// get stuck.
	if len(my.Units) > r.Slots() {
		return nil, rule("Too many players on the pitch.")
	}
	_, seedHex, err := randomSeed()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(seedHex))
	r.Match = &MatchState{Seed: seedHex, Commit: hex.EncodeToString(sum[:]), My: my, Opp: lineupOf(r.Opponent.Name, r.Opponent.Units)}
	r.Phase = "halftime"
	return r.FirstHalf()
}

// FirstHalf replays the first half from the stored seed.
func (r *Run) FirstHalf() (*SimResult, error) {
	if r.Phase != "halftime" || r.Match == nil {
		return nil, rule("No match in progress.")
	}
	seed, err := seedFromHex(r.Match.Seed)
	if err != nil {
		return nil, err
	}
	t := [2][2]string{{"balanced", "balanced"}, {"balanced", "balanced"}}
	return Simulate(seed, [2]Lineup{r.Match.My, r.Match.Opp}, t, 1), nil
}

func botTactic(g *RNG, half [2]int) string {
	switch d := half[1] - half[0]; {
	case d < 0:
		return []string{"all_out", "press"}[g.Intn(2)]
	case d > 0:
		return []string{"park_bus", "keep_ball"}[g.Intn(2)]
	}
	return []string{"balanced", "press", "keep_ball"}[g.Intn(3)]
}

// FinishMatch plays the whole match with the chosen second-half order, applies
// the result and prepares the next round.
func (r *Run) FinishMatch(tactic string) (*SimResult, *HistoryEntry, error) {
	if r.Phase != "halftime" || r.Match == nil {
		return nil, nil, rule("No match in progress.")
	}
	if _, ok := TacticByID[tactic]; !ok {
		return nil, nil, rule("Unknown team order.")
	}
	seed, err := seedFromHex(r.Match.Seed)
	if err != nil {
		return nil, nil, err
	}
	lus := [2]Lineup{r.Match.My, r.Match.Opp}
	half := Simulate(seed, lus, [2][2]string{{"balanced", "balanced"}, {"balanced", "balanced"}}, 1)
	// The bot's order depends only on the half-time score, never on the player's choice.
	bot := botTactic(NewRNG(seed^0xA5A5A5A5), half.Score)
	res := Simulate(seed, lus, [2][2]string{{"balanced", tactic}, {"balanced", bot}}, 2)

	h := HistoryEntry{Round: r.Round, Opponent: r.Opponent.Name, GF: res.Score[0], GA: res.Score[1], Pens: len(res.Pens) > 0, Won: res.Winner == 0, Seed: r.Match.Seed, Commit: r.Match.Commit}
	if h.Won {
		r.Wins++
		if r.Streak < 0 {
			r.Streak = 0
		}
		r.Streak++
	} else {
		r.Losses++
		if r.Streak > 0 {
			r.Streak = 0
		}
		r.Streak--
		h.Damage = 2 + (res.Score[1] - res.Score[0]) + r.Round/3
		r.Patience -= h.Damage
	}
	r.Match = nil
	r.Discipline(res.SentOff, 0)
	switch {
	case r.Patience <= 0:
		r.Patience = 0
		r.Phase, r.Result = "ended", "sacked"
	case r.Round >= MaxRounds:
		r.Phase = "ended"
		switch {
		case r.Wins >= 6:
			r.Result = "champion"
		case r.Wins >= 4:
			r.Result = "europe"
		default:
			r.Result = "survived"
		}
	default:
		income := BaseIncome + Interest(r.Budget)
		if h.Won {
			income++
		}
		if s := max(r.Streak, -r.Streak); s >= 4 {
			income += 2
		} else if s >= 2 {
			income++
		}
		h.Income = income
		r.Budget += income
		r.gainXP(2)
		r.Round++
		r.Phase = "shop"
		r.withRNG(func(g *RNG) {
			r.rollShop(g)
			r.Opponent = genOpponent(g, r.Round)
		})
	}
	r.History = append(r.History, h)
	return res, &h, nil
}

// RunView is everything the client is allowed to know.
type RunView struct {
	Phase        string         `json:"phase"`
	Round        int            `json:"round"`
	MaxRounds    int            `json:"maxRounds"`
	Patience     int            `json:"patience"`
	MaxPatience  int            `json:"maxPatience"`
	Budget       int            `json:"budget"`
	Level        int            `json:"level"`
	MaxLevel     int            `json:"maxLevel"`
	XP           int            `json:"xp"`
	XPNeed       int            `json:"xpNeed"`
	Slots        int            `json:"slots"`
	Shop         []string       `json:"shop"`
	ShopOdds     [5]int         `json:"shopOdds"`
	Units        []Unit         `json:"units"`
	Synergies    []SynergyView  `json:"synergies"`
	Opponent     Opponent       `json:"opponent"`
	OppSynergies []SynergyView  `json:"oppSynergies"`
	Streak       int            `json:"streak"`
	Wins         int            `json:"wins"`
	Losses       int            `json:"losses"`
	History      []HistoryEntry `json:"history"`
	Result       string         `json:"result"`
	Commit       string         `json:"commit,omitempty"`
}

func (r *Run) View() RunView {
	_, syn := ComputeSynergies(r.myLineup())
	_, osyn := ComputeSynergies(lineupOf("", r.Opponent.Units))
	v := RunView{
		Phase: r.Phase, Round: r.Round, MaxRounds: MaxRounds, Patience: r.Patience, MaxPatience: StartPatience,
		Budget: r.Budget, Level: r.Level, MaxLevel: MaxLevel, XP: r.XP, XPNeed: xpNeed[r.Level], Slots: r.Slots(),
		Shop: r.Shop, ShopOdds: shopOdds[r.Level], Units: r.Units, Synergies: syn, Opponent: r.Opponent, OppSynergies: osyn,
		Streak: r.Streak, Wins: r.Wins, Losses: r.Losses, History: r.History, Result: r.Result,
	}
	if v.Units == nil {
		v.Units = []Unit{}
	}
	if v.History == nil {
		v.History = []HistoryEntry{}
	}
	if r.Match != nil {
		v.Commit = r.Match.Commit
	}
	return v
}

// Squad returns the club's current line-up under the given name. ok is false
// when there is no outfield player to field.
func (r *Run) Squad(name string) (lu Lineup, ok bool) {
	lu = lineupOf(name, r.Units)
	return lu, len(lu.Units) > 0
}

// Stale reports whether the season refers to players that no longer exist in
// the pool (the catalogue changed). Such a season cannot continue.
func (r *Run) Stale() bool {
	known := func(id string) bool { return id == "" || PlayerByID[id] != nil }
	for _, id := range r.Shop {
		if !known(id) {
			return true
		}
	}
	for _, u := range r.Units {
		if !known(u.Def) {
			return true
		}
	}
	for _, u := range r.Opponent.Units {
		if !known(u.Def) {
			return true
		}
	}
	if r.Match != nil {
		for _, lu := range []Lineup{r.Match.My, r.Match.Opp} {
			if !known(lu.GK.DefID) {
				return true
			}
			for _, u := range lu.Units {
				if !known(u.DefID) {
					return true
				}
			}
		}
	}
	return false
}

// NewSeed draws a secret match seed and returns it with its public commitment.
func NewSeed() (seedHex, commit string, err error) {
	_, seedHex, err = randomSeed()
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(seedHex))
	return seedHex, hex.EncodeToString(sum[:]), nil
}

// SimulateSeed plays `halves` halves between two squads from a stored seed.
// Both sides play the first half balanced; orders are their second-half orders.
func SimulateSeed(seedHex string, squads [2]Lineup, orders [2]string, halves int) (*SimResult, error) {
	seed, err := seedFromHex(seedHex)
	if err != nil {
		return nil, err
	}
	for i, o := range orders {
		if _, ok := TacticByID[o]; !ok {
			orders[i] = "balanced"
		}
	}
	return Simulate(seed, squads, [2][2]string{{"balanced", orders[0]}, {"balanced", orders[1]}}, halves), nil
}

// ---- head-to-head series ----

const (
	VersusPatience  = 100 // live games count patience on a scale of 100; a defeat costs VersusDamage per point
	VersusDamage    = 8
	MaxVersusRounds = 10
)

// NewVersusRun is a fresh club for a series against a friend: same economy as
// a season, but the opponent is the other manager and patience is shorter.
func NewVersusRun(key string) (*Run, error) {
	r, err := NewRun(key)
	if err != nil {
		return nil, err
	}
	r.Patience = VersusPatience
	r.Opponent = Opponent{}
	return r, nil
}

// fieldLines is the order in which the lines are tried for a player brought
// on automatically: his own line first (a defender at the back, a striker up
// front), then the nearest ones. Within a line the centre fills first, except
// for wingers, who go wide.
var fieldLines = map[string][3]int{"defender": {0, 1, 2}, "anchor": {1, 0, 2}, "playmaker": {1, 2, 0}, "winger": {2, 1, 0}, "striker": {2, 1, 0}}

func fieldCells(role string) [][2]int {
	lanes := []int{2, 1, 3, 0, 4}
	if role == "winger" {
		lanes = []int{0, 4, 1, 3, 2}
	}
	lines, ok := fieldLines[role]
	if !ok {
		lines = [3]int{1, 2, 0}
	}
	var cells [][2]int
	for _, line := range lines {
		for _, lane := range lanes {
			cells = append(cells, [2]int{line, lane})
		}
	}
	return cells
}

// fillPitch brings bench players on, in bench order, while the stadium level
// allows more outfield players than are on the pitch, each on the line of his
// position. Keepers and suspended
// players stay where they are; an empty bench changes nothing.
func (r *Run) fillPitch() bool {
	_, board := r.counts()
	if board >= r.Slots() {
		return false
	}
	taken := map[[2]int]bool{}
	bench := []int{}
	for i, u := range r.Units {
		switch {
		case u.Line >= 0:
			taken[[2]int{u.Line, u.Lane}] = true
		case !IsGK(u.Def) && u.Ban == 0:
			bench = append(bench, i)
		}
	}
	sort.Slice(bench, func(a, b int) bool { return r.Units[bench[a]].Lane < r.Units[bench[b]].Lane })
	changed := false
	for _, i := range bench {
		if board >= r.Slots() {
			break
		}
		for _, c := range fieldCells(PlayerByID[r.Units[i].Def].Role) {
			if !taken[c] {
				r.Units[i].Line, r.Units[i].Lane = c[0], c[1]
				taken[c] = true
				board++
				changed = true
				break
			}
		}
	}
	return changed
}

// AutoField is the manager's assistant at the deadline. Free places on the
// pitch are filled from the bench (see fillPitch). If nobody eligible is out
// there after that, because the pitch is full of suspended players, the best
// eligible player on the bench takes a suspended player's place. It reports
// whether anything changed.
func (r *Run) AutoField() bool {
	if r.fillPitch() {
		return true
	}
	best, banned, board := -1, -1, 0
	taken := map[[2]int]bool{}
	for i, u := range r.Units {
		switch {
		case IsGK(u.Def):
		case u.Line >= 0:
			if u.Ban == 0 {
				return false // someone eligible is already out there
			}
			board++
			banned = i
			taken[[2]int{u.Line, u.Lane}] = true
		case u.Ban == 0 && (best < 0 || sellValue(u) > sellValue(r.Units[best])):
			best = i
		}
	}
	if best < 0 {
		return false
	}
	if board >= r.Slots() && banned >= 0 {
		// Swap places with a suspended player.
		b, n := &r.Units[banned], &r.Units[best]
		b.Line, b.Lane, n.Line, n.Lane = n.Line, n.Lane, b.Line, b.Lane
		return true
	}
	return false
}

// SettleVersus applies one series result to the club and opens the next
// transfer window. It is idempotent per round: a second call for the same
// round does nothing, so a retried settlement cannot pay or punish twice.
func (r *Run) SettleVersus(round int, opponent string, gf, ga int, won, pens bool, sent []SentOff, team int, stake int) bool {
	if r.Round != round {
		return false
	}
	r.Discipline(sent, team)
	h := HistoryEntry{Round: round, Opponent: opponent, GF: gf, GA: ga, Pens: pens, Won: won}
	if won {
		r.Wins++
		if r.Streak < 0 {
			r.Streak = 0
		}
		r.Streak++
	} else {
		r.Losses++
		if r.Streak > 0 {
			r.Streak = 0
		}
		r.Streak--
		// `stake` is how many competitive rounds have been played, this one
		// included: 0 for a warm-up match, which costs nothing.
		if stake > 0 {
			h.Damage = VersusDamage * (2 + max(0, ga-gf) + stake/3)
			r.Patience = max(0, r.Patience-h.Damage)
		}
	}
	income := BaseIncome + Interest(r.Budget)
	if won {
		income++
	}
	if s := max(r.Streak, -r.Streak); s >= 4 {
		income += 2
	} else if s >= 2 {
		income++
	}
	h.Income = income
	r.Budget += income
	r.gainXP(2)
	r.Round++
	r.withRNG(r.rollShop)
	r.History = append(r.History, h)
	return true
}

// LineupUnits turns a frozen line-up back into board units, for scouting views.
func LineupUnits(lu Lineup) []Unit {
	out := []Unit{}
	if lu.GK.DefID != "" {
		out = append(out, Unit{UID: "ogk", Def: lu.GK.DefID, Stars: lu.GK.Stars, Line: GKLine})
	}
	for i, u := range lu.Units {
		out = append(out, Unit{UID: fmt.Sprintf("o%d", i), Def: u.DefID, Stars: u.Stars, Line: u.Line, Lane: u.Lane})
	}
	return out
}

// Synergies returns the active team bonuses of a line-up.
func (lu Lineup) Synergies() []SynergyView {
	_, v := ComputeSynergies(lu)
	return v
}

// Discipline is applied once after every match: suspensions already running
// lose one match, then players sent off in this match (those of `team`) start
// theirs. A suspended player stays where he is, greyed out, and simply does
// not play until his ban is over or he is replaced.
func (r *Run) Discipline(sent []SentOff, team int) {
	for i := range r.Units {
		if r.Units[i].Ban > 0 {
			r.Units[i].Ban--
		}
	}
	for _, so := range sent {
		if so.Team != team {
			continue
		}
		for i := range r.Units {
			u := &r.Units[i]
			if u.Def == so.Def && u.Line == so.Line && u.Lane == so.Lane {
				u.Ban = so.Matches
				break
			}
		}
	}
}

// hasEligible reports whether the squad has any outfield player who may play.
func (r *Run) hasEligible() bool {
	for _, u := range r.Units {
		if !IsGK(u.Def) && u.Ban == 0 {
			return true
		}
	}
	return false
}

// ---- bots for ranked lobbies ----

// BotManage runs one transfer window for a bot club: level the stadium when
// rich, sign players (pairs first, then the most valuable it can afford), put
// its best keeper in goal and its best eligible players on the pitch.
func (r *Run) BotManage(g *RNG) {
	for i := 0; i < 3 && r.Level < MaxLevel && r.Budget >= XPCost+3+r.Level; i++ {
		_ = r.Apply(Action{Type: "levelup"})
	}
	for pass := 0; pass < 8; pass++ {
		best, bestScore := -1, -1
		for i, id := range r.Shop {
			d := PlayerByID[id]
			if d == nil || d.Cost > r.Budget {
				continue
			}
			score := d.Cost * 10
			for _, u := range r.Units {
				if u.Def == id {
					score += 25 // completing a merge is worth more than a new face
				}
			}
			if d.Role == "gk" {
				score -= 12
			}
			score += g.Intn(6)
			if score > bestScore {
				best, bestScore = i, score
			}
		}
		if best < 0 || r.Apply(Action{Type: "buy", Slot: best}) != nil {
			break
		}
	}
	value := func(u Unit) float64 {
		d := PlayerByID[u.Def]
		if d.Role == "gk" {
			return float64(d.Ref + (u.Stars-1)*8)
		}
		return float64(d.Cost+1) * StarMul[u.Stars]
	}
	// Best keeper in goal.
	gk, inGoal := -1, -1
	for i, u := range r.Units {
		if !IsGK(u.Def) {
			continue
		}
		if u.Line == GKLine {
			inGoal = i
		}
		if gk < 0 || value(u) > value(r.Units[gk]) {
			gk = i
		}
	}
	if gk >= 0 && inGoal >= 0 && gk != inGoal {
		_ = r.Apply(Action{Type: "move", UID: r.Units[gk].UID, Line: GKLine, Lane: 0})
	}
	// Best eligible outfield players on the pitch, everyone else on the bench.
	var out []int
	for i, u := range r.Units {
		if !IsGK(u.Def) {
			out = append(out, i)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		ua, ub := r.Units[out[a]], r.Units[out[b]]
		if (ua.Ban == 0) != (ub.Ban == 0) {
			return ua.Ban == 0
		}
		return value(ua) > value(ub)
	})
	taken := map[[2]int]bool{}
	cell := func(role string) (int, int) {
		line, lanes := 2, []int{2, 1, 3, 0, 4}
		switch role {
		case "defender":
			line = 0
		case "anchor", "playmaker":
			line = 1
		case "winger":
			lanes = []int{0, 4, 1, 3, 2}
		}
		for _, l := range []int{line, 1, 0, 2} {
			for _, ln := range lanes {
				if !taken[[2]int{l, ln}] {
					taken[[2]int{l, ln}] = true
					return l, ln
				}
			}
		}
		return 0, 0
	}
	bench := 0
	// Keepers that are not in goal take the first bench seats.
	for i := range r.Units {
		if IsGK(r.Units[i].Def) && r.Units[i].Line != GKLine {
			r.Units[i].Line, r.Units[i].Lane = -1, bench
			bench++
		}
	}
	for n, i := range out {
		u := &r.Units[i]
		if n < r.Slots() && u.Ban == 0 {
			u.Line, u.Lane = cell(PlayerByID[u.Def].Role)
		} else {
			u.Line, u.Lane = -1, bench
			bench++
		}
	}
	// A bench can hold only so many: let the weakest go.
	for bench > BenchSize {
		worst := -1
		for i, u := range r.Units {
			if u.Line == -1 && (worst < 0 || value(u) < value(r.Units[worst])) {
				worst = i
			}
		}
		if worst < 0 {
			break
		}
		r.Budget += sellValue(r.Units[worst])
		r.Units = append(r.Units[:worst], r.Units[worst+1:]...)
		bench = 0
		for i := range r.Units {
			if r.Units[i].Line == -1 {
				r.Units[i].Lane = bench
				bench++
			}
		}
	}
}

// BotOrder is the half-time order of a bot (or a ghost) on the given side. It
// depends only on the seed and the half-time score, never on the opponent's choice.
func BotOrder(seedHex string, squads [2]Lineup, side int) string {
	seed, err := seedFromHex(seedHex)
	if err != nil {
		return "balanced"
	}
	half := Simulate(seed, squads, [2][2]string{{"balanced", "balanced"}, {"balanced", "balanced"}}, 1)
	sc := half.Score
	if side == 0 {
		sc = [2]int{sc[1], sc[0]} // botTactic reads the score from the second team's side
	}
	return botTactic(NewRNG(seed^0xA5A5A5A5+uint64(side)), sc)
}
