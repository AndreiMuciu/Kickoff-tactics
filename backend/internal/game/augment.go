package game

import "strings"

// Augments are one-off boosts for a club, chosen from three offers at fixed
// points of a season. Some pay out at once, some change the economy for the
// rest of the season, and some change how the team plays. A "crest" counts as
// one more player of a style.
type AugmentDef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
	Kind string `json:"kind"` // economy | team | crest

	team func(m *Mods) // match effect, applied with the combinations
}

// AugmentRounds are the rounds whose transfer window opens with an offer.
var AugmentRounds = []int{2, 5, 8}

const (
	augmentOffers   = 3
	windfallBudget  = 10
	expansionXP     = 8
	investorMax     = InterestMax + 2
	sponsorIncome   = 2
	trustedDiscount = 0.7 // defeats cost this share of the usual patience
)

var Augments = func() []AugmentDef {
	list := []AugmentDef{
		{ID: "windfall", Name: "Transfer Windfall", Kind: "economy", Desc: "+10 💰 right now."},
		{ID: "investor", Name: "Shrewd Investor", Kind: "economy", Desc: "Interest goes up to +7 instead of +5 (at 70 💰 in the bank)."},
		{ID: "sponsor", Name: "Shirt Sponsor", Kind: "economy", Desc: "+2 💰 after every match."},
		{ID: "scout", Name: "Scouting Network", Kind: "economy", Desc: "The first new-players refresh of every transfer window is free."},
		{ID: "expansion", Name: "Stadium Expansion", Kind: "economy", Desc: "+8 stadium experience right now."},
		{ID: "trusted", Name: "Trusted by the Board", Kind: "economy", Desc: "Defeats cost 30% less of the owner's patience."},
		{ID: "extraman", Name: "Squad Rotation", Kind: "economy", Desc: "Field one more outfield player than your stadium level allows."},
		{ID: "fitness", Name: "Fitness Coach", Kind: "team", Desc: "Your players tire 25% more slowly.",
			team: func(m *Mods) { m.Drain *= 0.75 }},
		{ID: "crowd", Name: "Home Crowd", Kind: "team", Desc: "Every stat +4%.",
			team: func(m *Mods) {
				for k := range m.Stat {
					m.Stat[k] *= 1.04
				}
			}},
		{ID: "finishing", Name: "Finishing School", Kind: "team", Desc: "Shooting +12%.",
			team: func(m *Mods) { m.Stat[SSho] *= 1.12 }},
		{ID: "backline", Name: "Defensive Coach", Kind: "team", Desc: "Opponent chances -10%.",
			team: func(m *Mods) { m.OppXG *= 0.9 }},
		{ID: "penalties", Name: "Penalty Specialist", Kind: "team", Desc: "Your penalties, in a match or a shootout, score 10% more often.",
			team: func(m *Mods) { m.PenBonus += 0.10 }},
	}
	for _, s := range Synergies {
		if s.Kind == "style" {
			list = append(list, AugmentDef{ID: "crest_" + s.ID, Name: s.Name + " Crest", Kind: "crest", Desc: "Counts as one more " + s.Name + " player on the pitch."})
		}
	}
	return list
}()

var AugmentByID = func() map[string]*AugmentDef {
	m := map[string]*AugmentDef{}
	for i := range Augments {
		m[Augments[i].ID] = &Augments[i]
	}
	return m
}()

// crestStyle returns the style a crest augment stands for, or "".
func crestStyle(id string) string {
	s, ok := strings.CutPrefix(id, "crest_")
	if !ok {
		return ""
	}
	return s
}

func (r *Run) has(aug string) bool {
	for _, a := range r.Augments {
		if a == aug {
			return true
		}
	}
	return false
}

// offerAugments opens a choice of three augments the club does not have yet,
// at most one of them a crest, when the current round is an augment round.
func (r *Run) offerAugments(g *RNG) {
	r.AugmentOffer = nil
	due := false
	for _, n := range AugmentRounds {
		due = due || n == r.Round
	}
	if !due {
		return
	}
	var plain, crests []string
	for _, a := range Augments {
		if r.has(a.ID) {
			continue
		}
		if a.Kind == "crest" {
			crests = append(crests, a.ID)
		} else {
			plain = append(plain, a.ID)
		}
	}
	take := func(pool *[]string) {
		k := g.Intn(len(*pool))
		r.AugmentOffer = append(r.AugmentOffer, (*pool)[k])
		*pool = append((*pool)[:k], (*pool)[k+1:]...)
	}
	crest := len(crests) > 0 && g.Intn(100) < 70
	for len(r.AugmentOffer) < augmentOffers && (len(plain) > 0 || crest) {
		if crest && (len(r.AugmentOffer) == augmentOffers-1 || len(plain) == 0) {
			take(&crests)
			crest = false
			continue
		}
		take(&plain)
	}
	// The crest is not always last on screen.
	for i := len(r.AugmentOffer) - 1; i > 0; i-- {
		j := g.Intn(i + 1)
		r.AugmentOffer[i], r.AugmentOffer[j] = r.AugmentOffer[j], r.AugmentOffer[i]
	}
}

// chooseAugment takes one of the offered augments and applies what it does at once.
func (r *Run) chooseAugment(slot int) error {
	if len(r.AugmentOffer) == 0 {
		return rule("There is no augment to choose right now.")
	}
	if slot < 0 || slot >= len(r.AugmentOffer) {
		return rule("Invalid augment.")
	}
	id := r.AugmentOffer[slot]
	r.Augments = append(r.Augments, id)
	r.AugmentOffer = nil
	switch id {
	case "windfall":
		r.Budget += windfallBudget
	case "expansion":
		r.gainXP(expansionXP)
	case "scout":
		r.FreeReroll = true
	}
	return nil
}

// interest is the club's interest after a match, with the investor's higher cap.
func (r *Run) interest() int {
	if r.has("investor") {
		return min(max(r.Budget, 0)/InterestStep, investorMax)
	}
	return Interest(r.Budget)
}

func (r *Run) interestMax() int {
	if r.has("investor") {
		return investorMax
	}
	return InterestMax
}

// income is what the club is paid after a match.
func (r *Run) income(won bool) int {
	n := BaseIncome + r.interest()
	if won {
		n++
	}
	if s := max(r.Streak, -r.Streak); s >= 4 {
		n += 2
	} else if s >= 2 {
		n++
	}
	if r.has("sponsor") {
		n += sponsorIncome
	}
	return n
}

// lessened applies the board's trust to a defeat's patience cost.
func (r *Run) lessened(damage int) int {
	if r.has("trusted") && damage > 0 {
		return max(1, int(float64(damage)*trustedDiscount+0.5))
	}
	return damage
}

// rerollPrice is what the next market refresh costs.
func (r *Run) rerollPrice() int {
	if r.FreeReroll {
		return 0
	}
	return RerollCost
}

// nextWindow prepares the market and augments of a new transfer window. A
// locked market is kept as it was, once, and then unlocks.
func (r *Run) nextWindow(g *RNG) {
	if r.Locked {
		r.Locked = false
	} else {
		r.rollShop(g)
	}
	r.FreeReroll = r.has("scout")
	r.offerAugments(g)
}

// LossFor is the patience a defeat of the given base cost takes from this club.
func (r *Run) LossFor(damage int) int { return r.lessened(damage) }
