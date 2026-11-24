package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Items are kit a club collects and hands to one player each: every third
// round's transfer window brings one, at random, into the kit bag. A player
// carries at most one item; a player who is sold, or merged into another copy,
// gives his item back to the bag.
//
// Six items are components, one stat each. Two components make a finished
// item, as in TFT: both stats plus a football effect, the kind of thing a
// player's trait does (a poacher drifts into the box, a regista makes the
// whole team pass better). 21 recipes, every component with every other and
// with itself. Grip Gloves make four keeper items with some components.
// Energy Drink and Captain's Armband are finished as they come. Combining
// happens in the bag (one item dropped on another) or on a player: a
// component handed to a player who wears one becomes the finished item.

type ItemDef struct {
	ID        string    `json:"id"`
	Icon      string    `json:"icon"`
	Name      string    `json:"name"`
	Desc      string    `json:"desc"`
	Keeper    bool      `json:"keeper,omitempty"`    // only a goalkeeper can wear it
	Component bool      `json:"component,omitempty"` // combines with another into a finished item
	Parts     [2]string `json:"parts,omitempty"`     // a finished item: what it is made of

	stat [7]float64 // multipliers on the wearer's stats (0 = unchanged)
	ref  int        // keeper reflexes added
	fx   traitFx    // a trait-like effect, added to the wearer's own
}

// ItemEvery is how often an item arrives: every this many rounds.
const ItemEvery = 3

var statNames = [7]string{"pace", "shooting", "passing", "dribbling", "defending", "physical", "stamina"}

// The items that drop: components and the three items finished as they come.
var baseItems = []ItemDef{
	{ID: "boots", Icon: "👟", Name: "Shooting Boots", Component: true, stat: [7]float64{SSho: 1.35}},
	{ID: "spikes", Icon: "⚡", Name: "Sprint Spikes", Component: true, stat: [7]float64{SPac: 1.12}},
	{ID: "compass", Icon: "🧭", Name: "Playmaker's Compass", Component: true, stat: [7]float64{SPas: 1.35}},
	{ID: "tape", Icon: "🩹", Name: "Ankle Tape", Component: true, stat: [7]float64{SDri: 1.35}},
	{ID: "shinpads", Icon: "🦵", Name: "Steel Shin Pads", Component: true, stat: [7]float64{SDef: 1.3}},
	{ID: "vest", Icon: "🏋", Name: "Weighted Vest", Component: true, stat: [7]float64{SPhy: 1.35}},
	{ID: "gloves", Icon: "🧤", Name: "Grip Gloves", Keeper: true, Component: true, ref: 6},
	{ID: "drink", Icon: "🧃", Name: "Energy Drink", Desc: "Stamina +30%: he tires much more slowly.", stat: [7]float64{SSta: 1.3}},
	{ID: "armband", Icon: "©️", Name: "Captain's Armband", Desc: "Every stat +6%.", stat: [7]float64{1.06, 1.06, 1.06, 1.06, 1.06, 1.06, 1.06}},
}

// recipes: the two parts, the finished item and what it adds beyond their stats.
var recipes = []struct {
	a, b, id, icon, name, effect string
	fx                           traitFx
}{
	{"boots", "boots", "golden_boot", "🥇", "Golden Boot", "Close-range shots are 25% more dangerous.", traitFx{closeShot: 1.25}},
	{"boots", "spikes", "poacher", "🦊", "Poacher", "Drifts into the box when his team attacks; close-range shots +10%.", traitFx{boxRunner: 8, closeShot: 1.1}},
	{"boots", "compass", "freekick", "🎯", "Free-Kick Specialist", "Direct free kicks 60% more dangerous, shots from distance +50%.", traitFx{freeKick: 1.6, longShot: 1.5}},
	{"boots", "tape", "trickster", "🪄", "Trickster", "After beating a defender, his next shot is 40% better.", traitFx{afterDuelBoost: 1.4}},
	{"boots", "shinpads", "big_game", "💥", "Big-Game Player", "His shots are 35% better while his team is not ahead.", traitFx{clutchShot: 1.35}},
	{"boots", "vest", "target_man", "🗼", "Target Man", "Holds off defenders: +16 in duels when he has the ball.", traitFx{duelAtk: 16}},
	{"spikes", "spikes", "afterburner", "🚀", "Afterburner", "Runs 5% faster with the ball at his feet.", traitFx{ballSpeed: 1.05}},
	{"spikes", "compass", "launcher", "📡", "Counter Launcher", "Distance does not hurt his passes: he finds runners from deep.", traitFx{longPass: true}},
	{"spikes", "tape", "wing_wizard", "🌪", "Wing Wizard", "A wide angle does not hurt his shots.", traitFx{wideShot: true}},
	{"spikes", "shinpads", "recovery", "🏃", "Recovery Pace", "15% faster whenever his team does not have the ball.", traitFx{defSpeed: 1.15}},
	{"spikes", "vest", "box_to_box", "🔁", "Box-to-Box Engine", "Tires 25% more slowly.", traitFx{drain: 0.75}},
	{"compass", "compass", "regista", "🎼", "Regista", "Conducts the team: every teammate's passes are 6% more accurate.", traitFx{teamPassAcc: 0.06}},
	{"compass", "tape", "trequartista", "✨", "Trequartista", "Whoever receives his pass shoots 25% better.", traitFx{recvBoost: 1.25}},
	{"compass", "shinpads", "deep_playmaker", "⚓", "Deep-Lying Playmaker", "Calm on the ball: pressure does not hurt his shots, and his passes are 8% more accurate.", traitFx{ignorePressure: true, passAcc: 0.08}},
	{"compass", "vest", "general", "🎖", "Midfield General", "The whole team fills Moment 35% faster.", traitFx{teamMoment: 1.35}},
	{"tape", "tape", "nutmeg_king", "🥜", "Nutmeg King", "After beating a defender, nobody can touch him for 1.6 seconds.", traitFx{afterDuelFree: 1.6}},
	{"tape", "shinpads", "ball_player", "🧠", "Ball-Playing Defender", "Never loses the ball right after winning it; passes 8% more accurate.", traitFx{safeAfterTackle: true, passAcc: 0.08}},
	{"tape", "vest", "shield", "🛡", "Shield the Ball", "+14 in duels with the ball, and fouled 30% more often.", traitFx{duelAtk: 14, foulDraw: 1.3}},
	{"shinpads", "shinpads", "iron_curtain", "🧱", "Iron Curtain", "Organises the back line: opponents' chances are 12% worse.", traitFx{teamOppXG: 0.88}},
	{"shinpads", "vest", "enforcer", "🔨", "Enforcer", "+14 in tackles, and whoever he tackles stays down for half a second.", traitFx{duelDef: 14, stunOnTackle: 0.5}},
	{"vest", "vest", "tireless", "🔋", "Tireless", "Sets the pace for everyone: the whole team tires 15% more slowly.", traitFx{teamDrain: 0.85}},
	// Goalkeepers.
	{"gloves", "gloves", "safe_hands", "🙌", "Safe Hands", "Every shot against him is 5% less dangerous.", traitFx{gkAll: 0.95}},
	{"gloves", "compass", "sweeper_keeper", "🧹", "Sweeper Keeper", "Launches the ball to the most advanced free teammate, never intercepted, and shots from distance against him are 12% less dangerous.", traitFx{gkSafe: true, gkSweeper: true, gkLong: 0.88}},
	{"gloves", "spikes", "one_on_one", "🥅", "One-on-One Specialist", "Close-range shots against him are 15% less dangerous.", traitFx{gkClose: 0.85}},
	{"gloves", "boots", "penalty_saver", "🛑", "Penalty Saver", "In a shootout every kick against him is 5% less likely to score.", traitFx{gkPens: 0.05}},
}

// Items is the public catalogue: what drops, then every finished item.
var Items, recipeOf = func() ([]ItemDef, map[[2]string]string) {
	list := append([]ItemDef(nil), baseItems...)
	byID := map[string]ItemDef{}
	for i, it := range list {
		if it.Desc == "" {
			list[i].Desc = statText(it.stat, it.ref)
			if it.Component {
				list[i].Desc += " A component: combine it with another."
			}
		}
		byID[it.ID] = list[i]
	}
	rec := map[[2]string]string{}
	for _, r := range recipes {
		a, b := byID[r.a], byID[r.b]
		it := ItemDef{ID: r.id, Icon: r.icon, Name: r.name, Parts: [2]string{r.a, r.b}, Keeper: a.Keeper || b.Keeper, fx: r.fx, ref: a.ref + b.ref}
		// A finished item also lifts every stat a little (the player feels the
		// part). A keeper's outfield stats do not play: his items give reflexes.
		if it.Keeper {
			it.ref = max(it.ref, keeperItemRef)
		} else {
			for k := range it.stat {
				it.stat[k] = finishedAll + max(a.stat[k]-1, 0) + max(b.stat[k]-1, 0)
			}
		}
		it.Desc = finishedText(it, a, b) + " " + r.effect
		list = append(list, it)
		rec[pair(r.a, r.b)] = r.id
	}
	return list, rec
}()

func pair(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

// statText describes an item's stat changes, e.g. "Shooting +15%, pace +15%."
func statText(stat [7]float64, ref int) string {
	var parts []string
	for k, m := range stat {
		if m > 0 {
			parts = append(parts, fmt.Sprintf("%s +%d%%", statNames[k], int((m-1)*100+0.5)))
		}
	}
	if ref > 0 {
		parts = append(parts, fmt.Sprintf("reflexes +%d", ref))
	}
	if len(parts) == 0 {
		return ""
	}
	s := strings.Join(parts, ", ") + "."
	return strings.ToUpper(s[:1]) + s[1:]
}

// Tuned with TestBalanceItems: what a finished item adds to every stat, and
// the least reflexes a finished keeper item gives.
const (
	finishedAll   = 1.08
	keeperItemRef = 10
)

// finishedText describes a finished item's stats: its parts' stats, then the rest.
func finishedText(it ItemDef, a, b ItemDef) string {
	if it.Keeper {
		return statText([7]float64{}, it.ref)
	}
	var parts []string
	for k, m := range it.stat {
		if a.stat[k] > 0 || b.stat[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s +%d%%", statNames[k], int((m-1)*100+0.5)))
		}
	}
	s := strings.Join(parts, ", ") + fmt.Sprintf(", every other stat +%d%%.", int(math.Round((finishedAll-1)*100)))
	return strings.ToUpper(s[:1]) + s[1:]
}

var ItemByID = func() map[string]*ItemDef {
	m := map[string]*ItemDef{}
	for i := range Items {
		m[Items[i].ID] = &Items[i]
	}
	return m
}()

// dropItem puts a random item that drops in the bag when the round is an item round.
func (r *Run) dropItem(g *RNG) {
	if r.Round%ItemEvery == 0 {
		r.Items = append(r.Items, baseItems[g.Intn(len(baseItems))].ID)
	}
}

// Recipe is the finished item two items make, or "".
func Recipe(a, b string) string { return recipeOf[pair(a, b)] }

// combine makes a finished item of bag items `slot` and `other`.
func (r *Run) combine(slot, other int) error {
	if slot < 0 || other < 0 || slot >= len(r.Items) || other >= len(r.Items) || slot == other {
		return rule("No such items in your kit bag.")
	}
	id := Recipe(r.Items[slot], r.Items[other])
	if id == "" {
		return rule("%s and %s do not combine.", ItemByID[r.Items[slot]].Name, ItemByID[r.Items[other]].Name)
	}
	var bag []string
	for i, it := range r.Items {
		if i != slot && i != other {
			bag = append(bag, it)
		}
	}
	r.Items = append(bag, id)
	return nil
}

// equip hands item `slot` of the bag to a player. If he already wears a
// component that combines with it, the two become the finished item;
// otherwise the item he wore goes back to the bag.
func (r *Run) equip(slot int, uid string) error {
	if slot < 0 || slot >= len(r.Items) {
		return rule("No such item in your kit bag.")
	}
	u := r.find(uid)
	if u == nil {
		return rule("No such player.")
	}
	it := ItemByID[r.Items[slot]]
	if it == nil {
		return rule("Unknown item.")
	}
	id := it.ID
	if made := Recipe(u.Item, id); u.Item != "" && made != "" {
		id, it = made, ItemByID[made]
	}
	switch {
	case it.Keeper && !IsGK(u.Def):
		return rule("%s is for goalkeepers only.", it.Name)
	case !it.Keeper && IsGK(u.Def) && !it.Component:
		return rule("%s is for outfield players.", it.Name)
	case !it.Keeper && IsGK(u.Def):
		return rule("%s is for outfield players (a keeper can use it combined with Grip Gloves).", it.Name)
	}
	old := u.Item
	if id != r.Items[slot] {
		old = "" // the two became one
	}
	u.Item = id
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

// botItems combines what a bot can, then hands its items to its most
// valuable players on the pitch (keeper items to the keeper in goal) who
// have none yet.
func (r *Run) botItems(value func(Unit) float64) {
	for again := true; again; {
		again = false
		for i := 0; i < len(r.Items) && !again; i++ {
			for j := i + 1; j < len(r.Items); j++ {
				if Recipe(r.Items[i], r.Items[j]) != "" && r.combine(i, j) == nil {
					again = true
					break
				}
			}
		}
	}
	sort.SliceStable(r.Items, func(a, b int) bool { return !ItemByID[r.Items[a]].Component && ItemByID[r.Items[b]].Component })
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
	p.fx = withFx(p.fx, it.fx)
}

// withFx adds an item's effect to a player's own: multipliers multiply,
// bonuses add up, switches turn on.
func withFx(a, b traitFx) traitFx {
	mul := func(x, y float64) float64 {
		if x == 0 || y == 0 {
			return x + y
		}
		return x * y
	}
	a.longShot, a.closeShot = mul(a.longShot, b.longShot), mul(a.closeShot, b.closeShot)
	a.clutchShot = mul(a.clutchShot, b.clutchShot)
	a.wideShot, a.ignorePressure = a.wideShot || b.wideShot, a.ignorePressure || b.ignorePressure
	a.duelAtk, a.duelDef = a.duelAtk+b.duelAtk, a.duelDef+b.duelDef
	a.stunOnTackle += b.stunOnTackle
	a.drain = mul(a.drain, b.drain)
	a.defSpeed, a.ballSpeed = mul(a.defSpeed, b.defSpeed), mul(a.ballSpeed, b.ballSpeed)
	a.passAcc += b.passAcc
	a.longPass = a.longPass || b.longPass
	a.recvBoost = mul(a.recvBoost, b.recvBoost)
	a.afterDuelBoost = mul(a.afterDuelBoost, b.afterDuelBoost)
	a.afterDuelFree += b.afterDuelFree
	a.safeAfterTackle = a.safeAfterTackle || b.safeAfterTackle
	a.boxRunner += b.boxRunner
	a.teamMoment, a.teamOppXG = mul(a.teamMoment, b.teamMoment), mul(a.teamOppXG, b.teamOppXG)
	a.teamPassAcc += b.teamPassAcc
	a.teamDrain = mul(a.teamDrain, b.teamDrain)
	a.gkAll, a.gkClose, a.gkLong = mul(a.gkAll, b.gkAll), mul(a.gkClose, b.gkClose), mul(a.gkLong, b.gkLong)
	a.gkSafe, a.gkSweeper = a.gkSafe || b.gkSafe, a.gkSweeper || b.gkSweeper
	a.gkPens += b.gkPens
	a.foulDraw = mul(a.foulDraw, b.foulDraw)
	a.freeKick = mul(a.freeKick, b.freeKick)
	return a
}
