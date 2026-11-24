package game

// Items are kit a club collects and hands to one player each: every third
// round's transfer window brings one, at random, into the kit bag. A player
// carries at most one item; equipping another swaps them, and a player who is
// sold, or merged into another copy, gives his item back to the bag.
// Goalkeeper items go only on keepers, the rest only on outfield players.

type ItemDef struct {
	ID     string `json:"id"`
	Icon   string `json:"icon"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Keeper bool   `json:"keeper,omitempty"` // only a goalkeeper can wear it

	stat [7]float64 // multipliers on the wearer's stats (0 = unchanged)
	ref  int        // keeper reflexes added
}

// ItemEvery is how often an item arrives: every this many rounds.
const ItemEvery = 3

var Items = []ItemDef{
	{ID: "boots", Icon: "👟", Name: "Golden Boots", Desc: "Shooting +15%.", stat: [7]float64{SSho: 1.15}},
	{ID: "spikes", Icon: "⚡", Name: "Sprint Spikes", Desc: "Pace +15%.", stat: [7]float64{SPac: 1.15}},
	{ID: "compass", Icon: "🧭", Name: "Playmaker's Compass", Desc: "Passing +15%.", stat: [7]float64{SPas: 1.15}},
	{ID: "tape", Icon: "🩹", Name: "Ankle Tape", Desc: "Dribbling +15%.", stat: [7]float64{SDri: 1.15}},
	{ID: "shinpads", Icon: "🛡", Name: "Steel Shin Pads", Desc: "Defending +15%.", stat: [7]float64{SDef: 1.15}},
	{ID: "vest", Icon: "🏋", Name: "Weighted Vest", Desc: "Physical +15%.", stat: [7]float64{SPhy: 1.15}},
	{ID: "drink", Icon: "🧃", Name: "Energy Drink", Desc: "Stamina +30%: he tires much more slowly.", stat: [7]float64{SSta: 1.3}},
	{ID: "armband", Icon: "©️", Name: "Captain's Armband", Desc: "Every stat +6%.", stat: [7]float64{1.06, 1.06, 1.06, 1.06, 1.06, 1.06, 1.06}},
	{ID: "gloves", Icon: "🧤", Name: "Grip Gloves", Keeper: true, Desc: "Goalkeepers only: reflexes +6.", ref: 6},
}

var ItemByID = func() map[string]*ItemDef {
	m := map[string]*ItemDef{}
	for i := range Items {
		m[Items[i].ID] = &Items[i]
	}
	return m
}()

// dropItem puts a random item in the bag when the round is an item round.
func (r *Run) dropItem(g *RNG) {
	if r.Round%ItemEvery == 0 {
		r.Items = append(r.Items, Items[g.Intn(len(Items))].ID)
	}
}

// equip hands item `slot` of the bag to a player; an item he already wore
// goes back to the bag.
func (r *Run) equip(slot int, uid string) error {
	if slot < 0 || slot >= len(r.Items) {
		return rule("No such item in your kit bag.")
	}
	u := r.find(uid)
	if u == nil {
		return rule("No such player.")
	}
	it := ItemByID[r.Items[slot]]
	switch {
	case it == nil:
		return rule("Unknown item.")
	case it.Keeper && !IsGK(u.Def):
		return rule("%s is for goalkeepers only.", it.Name)
	case !it.Keeper && IsGK(u.Def):
		return rule("%s is for outfield players.", it.Name)
	}
	old := u.Item
	u.Item = it.ID
	r.Items = append(r.Items[:slot], r.Items[slot+1:]...)
	if old != "" {
		r.Items = append(r.Items, old)
	}
	return nil
}

// unequip takes a player's item back to the bag.
func (r *Run) unequip(uid string) error {
	u := r.find(uid)
	if u == nil || u.Item == "" {
		return rule("He has no item.")
	}
	r.Items = append(r.Items, u.Item)
	u.Item = ""
	return nil
}

// botItems hands a bot's items to its most valuable players on the pitch
// (gloves to the keeper in goal) who have none yet.
func (r *Run) botItems(value func(Unit) float64) {
	for k := 0; k < len(r.Items); {
		it := ItemByID[r.Items[k]]
		best := -1
		for i, u := range r.Units {
			if it == nil || u.Item != "" || u.Line < 0 || IsGK(u.Def) != it.Keeper {
				continue
			}
			if best < 0 || value(u) > value(r.Units[best]) {
				best = i
			}
		}
		if best < 0 || r.equip(k, r.Units[best].UID) != nil {
			k++
		}
	}
}

// applyItem changes a player in the match by the item he wears.
func applyItem(p *simPlayer, id string) {
	it := ItemByID[id]
	if it == nil {
		return
	}
	for k, m := range it.stat {
		if m > 0 {
			p.base[k] *= m
		}
	}
	p.ref += float64(it.ref)
}
