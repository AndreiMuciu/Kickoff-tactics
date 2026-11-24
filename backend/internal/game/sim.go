package game

import (
	"math"
	"sort"
)

// The pitch is 100 x 60 units. Team 0 attacks towards x = 100, team 1 towards x = 0.
const (
	PitchL    = 100.0
	PitchW    = 60.0
	simDT     = 0.1
	HalfTicks = 300
)

type LineupUnit struct {
	DefID string
	Stars int
	Line  int // 0 defence, 1 midfield, 2 attack
	Lane  int // 0..4 across the pitch
}

type Lineup struct {
	Name     string
	GK       LineupUnit // only DefID and Stars are used
	Units    []LineupUnit
	Augments []string `bson:",omitempty"` // the club's augments: crests and match effects count
}

// Frame is [ballX, ballY, owner, then x, y, flags per player]; coordinates x10.
type Frame []int

type Event struct {
	T    int     `json:"t"`
	Type string  `json:"type"`
	Team int     `json:"team"`
	P    int     `json:"p"`
	Q    int     `json:"q"`
	XG   float64 `json:"xg,omitempty"`
}

type RosterEntry struct {
	Idx   int    `json:"idx"`
	Team  int    `json:"team"`
	Name  string `json:"name"`
	Def   string `json:"def"`
	Role  string `json:"role"`
	Stars int    `json:"stars"`
	GK    bool   `json:"gk"`
}

type TeamStats struct {
	Shots   int     `json:"shots"`
	XG      float64 `json:"xg"`
	Passes  int     `json:"passes"`
	Tackles int     `json:"tackles"`
	Poss    int     `json:"poss"`
}

type PenKick struct {
	Team   int  `json:"team"`
	Scored bool `json:"scored"`
}

// SentOff is a player dismissed during the match, with the suspension that
// follows: one match for two yellows, one to three for a straight red.
type SentOff struct {
	Team     int
	Def      string
	Line     int
	Lane     int
	Straight bool
	Matches  int
}

type SimResult struct {
	SentOff   []SentOff
	Roster    []RosterEntry
	Frames    []Frame
	Events    []Event
	Score     [2]int
	HalfScore [2]int
	Stats     [2]TeamStats
	Pens      []PenKick
	Winner    int
}

type vec struct{ x, y float64 }

func (a vec) add(b vec) vec           { return vec{a.x + b.x, a.y + b.y} }
func (a vec) sub(b vec) vec           { return vec{a.x - b.x, a.y - b.y} }
func (a vec) mul(f float64) vec       { return vec{a.x * f, a.y * f} }
func (a vec) length() float64         { return math.Hypot(a.x, a.y) }
func dist(a, b vec) float64           { return a.sub(b).length() }
func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// or1 treats an unset multiplier as neutral.
func or1(v float64) float64 {
	if v == 0 {
		return 1
	}
	return v
}

type simPlayer struct {
	idx, team int
	gk        bool
	role      string
	line      int
	fx        traitFx
	base      [7]float64
	ref       float64
	home, pos vec
	stam      float64
	stun      float64
	shield    float64
	turbo     float64
	moment    float64
	shotBoost float64
	surePass  bool
	sureBoost float64
	sureDuel  bool
	def       string
	lane      int
	yellow    bool
	off       bool // sent off: no longer part of the match
}

const (
	// interceptReach is how close to the passing line a defender must stand to cut a pass out.
	interceptReach = 5.0

	flightPass  = 1
	flightShot  = 2
	flightLoose = 3 // a misplaced pass rolling to a spot, for whoever gets there first
	outGoal     = 1
	outSave     = 2
	outMiss     = 3
)

type flight struct {
	active      bool
	kind        int
	from        vec
	target      int // receiving player for a pass
	to          vec // fixed point for a shot
	t, dur      float64
	outcome     int
	shooter     int
	team        int
	boost       float64
	extraMoment float64
}

type sim struct {
	draws   bool // a league match: level at full time stays a draw
	rng     *RNG
	pl      []*simPlayer
	teamIdx [2][]int
	gk      [2]int
	mods    [2]Mods
	tac     [2]Tactic
	ball    vec
	owner   int
	fl      flight
	decT    float64
	poss    int
	freeze  int
	tick    int
	half    int
	score   [2]int
	res     *SimResult
	// penTaker is the player about to take a penalty once the freeze ends, or -1.
	penTaker int
	// fkTaker is the player about to strike a direct free kick, or -1.
	fkTaker int
	// kicks each side takes in a shootout before sudden death.
	kicks int
}

func adv(team int, x float64) float64 {
	if team == 0 {
		return x
	}
	return PitchL - x
}

func goalX(team int) float64 {
	if team == 0 {
		return PitchL
	}
	return 0
}

var homeAdv = [3]float64{20, 33, 45}

// Simulate plays `halves` halves (1 or 2). The first half never depends on the
// second-half tactics, so a one-half run is an exact prefix of the full match.
func Simulate(seed uint64, lineups [2]Lineup, tactics [2][2]string, halves int) *SimResult {
	return simulate(seed, lineups, tactics, halves, [2]string{})
}

// SimulateLeague plays a full league match: level at full time is a draw
// (Winner -1), with no shootout. Otherwise identical to Simulate.
func SimulateLeague(seed uint64, lineups [2]Lineup, tactics [2][2]string) *SimResult {
	s := newSim(seed, lineups, [2]string{})
	s.draws = true
	s.play(tactics, 2)
	return s.res
}

// simulate plays the match. `without` names, per team, one combination to
// leave out: the balance reports use it to measure what a combination is
// worth with the very same players on the pitch.
func simulate(seed uint64, lineups [2]Lineup, tactics [2][2]string, halves int, without [2]string) *SimResult {
	s := newSim(seed, lineups, without)
	s.play(tactics, halves)
	return s.res
}

// SimulateShootout settles a meeting with penalties alone, five kicks each and
// then sudden death: the penalty rounds at the start of a lobby. Its frames
// are only the kicks; tick numbers continue from where a full match would
// end, so events read like those of a shootout after 90 minutes.
func SimulateShootout(seedHex string, lineups [2]Lineup) (*SimResult, error) {
	seed, err := seedFromHex(seedHex)
	if err != nil {
		return nil, err
	}
	s := newSim(seed, lineups, [2]string{})
	for t := 0; t < 2; t++ {
		s.tac[t] = TacticByID["balanced"]
	}
	s.tick = 2 * HalfTicks
	s.kicks = cupKicks
	s.shootout()
	s.res.Score = s.score
	return s.res, nil
}

// cupKicks is the length of a penalty round's shootout before sudden death.
const cupKicks = 5

func newSim(seed uint64, lineups [2]Lineup, without [2]string) *sim {
	s := &sim{rng: NewRNG(seed), owner: -1, penTaker: -1, fkTaker: -1, res: &SimResult{}, kicks: shootoutKicks}
	for t := 0; t < 2; t++ {
		lu := lineups[t]
		s.mods[t], _ = computeSynergies(lu, without[t])

		gd := PlayerByID[lu.GK.DefID]
		if gd == nil || gd.Role != "gk" {
			gd = PlayerByID[AcademyKeeper]
		}
		gs := lu.GK.Stars
		if gs < 1 || gs > 3 {
			gs = 1
		}
		gk := &simPlayer{idx: len(s.pl), team: t, gk: true, role: "gk", fx: fxByID[gd.ID], ref: float64(gd.Ref + (gs-1)*8), stam: 1, shotBoost: 1}
		gk.base = [7]float64{50, 20, 55, 30, 40, 60, 99}
		gk.home = vec{adv(t, 4), PitchW / 2}
		s.gk[t] = gk.idx
		s.pl = append(s.pl, gk)
		s.teamIdx[t] = append(s.teamIdx[t], gk.idx)
		s.res.Roster = append(s.res.Roster, RosterEntry{Idx: gk.idx, Team: t, Name: gd.Name, Def: gd.ID, Role: "gk", Stars: gs, GK: true})

		for _, u := range lu.Units {
			d := PlayerByID[u.DefID]
			if d == nil || d.Role == "gk" || u.Line < 0 || u.Line > 2 || u.Lane < 0 || u.Lane > 4 || u.Stars < 1 || u.Stars > 3 {
				continue
			}
			p := &simPlayer{idx: len(s.pl), team: t, role: d.Role, line: u.Line, lane: u.Lane, def: d.ID, fx: fxByID[d.ID], stam: 1, shotBoost: 1}
			for k := 0; k < 7; k++ {
				p.base[k] = float64(d.Stats[k]) * StarMul[u.Stars]
			}
			y := 6 + 12*float64(u.Lane)
			if t == 1 {
				y = PitchW - y
			}
			p.home = vec{adv(t, homeAdv[u.Line]), y}
			s.pl = append(s.pl, p)
			s.teamIdx[t] = append(s.teamIdx[t], p.idx)
			s.res.Roster = append(s.res.Roster, RosterEntry{Idx: p.idx, Team: t, Name: d.Name, Def: d.ID, Role: d.Role, Stars: u.Stars})
			// Aura traits change the whole team.
			s.mods[t].MomentMul *= or1(p.fx.teamMoment)
			s.mods[t].PassAcc += p.fx.teamPassAcc
			s.mods[t].OppXG *= or1(p.fx.teamOppXG)
			s.mods[t].Stat[SDef] *= or1(p.fx.teamDef)
			s.mods[t].PressSpeed *= or1(p.fx.teamPress)
			s.mods[t].Drain *= or1(p.fx.teamDrain)
		}
	}
	return s
}

func (s *sim) play(tactics [2][2]string, halves int) {
	for h := 0; h < halves && h < 2; h++ {
		s.half = h
		for t := 0; t < 2; t++ {
			tac, ok := TacticByID[tactics[t][h]]
			if !ok {
				tac = TacticByID["balanced"]
			}
			s.tac[t] = tac
		}
		if h == 1 {
			for _, p := range s.pl {
				p.stam = math.Min(1, p.stam+0.25)
			}
		}
		s.kickoff(h)
		for i := 0; i < HalfTicks; i++ {
			s.step()
			s.record()
		}
		if h == 0 {
			s.res.HalfScore = s.score
			s.event("half", 0, -1, -1, 0)
		} else {
			s.event("full", 0, -1, -1, 0)
		}
	}
	s.res.Score = s.score
	s.res.Winner = -1
	if halves >= 2 {
		switch {
		case s.score[0] > s.score[1]:
			s.res.Winner = 0
		case s.score[1] > s.score[0]:
			s.res.Winner = 1
		case s.draws:
		default:
			s.shootout()
		}
	}
}

func (s *sim) event(typ string, team, p, q int, xg float64) {
	s.res.Events = append(s.res.Events, Event{T: s.tick, Type: typ, Team: team, P: p, Q: q, XG: math.Round(xg*100) / 100})
}

func (s *sim) stat(p *simPlayer, k int) float64 {
	v := p.base[k] * s.mods[p.team].Stat[k]
	if k == SDef {
		v *= s.tac[p.team].DefMul
	}
	if k != SSta {
		v *= 0.75 + 0.25*p.stam
	}
	return v
}

func (s *sim) setOwner(i int) {
	s.owner = i
	s.poss = s.pl[i].team
	s.ball = s.pl[i].pos
	s.decT = 0.4
}

func (s *sim) kickoff(team int) {
	s.fl.active = false
	s.penTaker, s.fkTaker = -1, -1
	for _, p := range s.pl {
		if p.off {
			continue
		}
		p.pos = p.home
		p.stun, p.shield, p.turbo = 0, 0, 0
	}
	kicker := -1
	best := math.Inf(1)
	for _, i := range s.teamIdx[team] {
		p := s.pl[i]
		if p.gk {
			continue
		}
		if d := dist(p.home, vec{PitchL / 2, PitchW / 2}); d < best {
			best, kicker = d, i
		}
	}
	if kicker < 0 {
		kicker = s.gk[team]
	} else {
		s.pl[kicker].pos = vec{adv(team, 49), PitchW / 2}
	}
	s.setOwner(kicker)
	s.freeze = 10
	s.event("kickoff", team, kicker, -1, 0)
}

func (s *sim) step() {
	s.tick++
	if s.freeze > 0 {
		s.freeze--
		return
	}
	if s.penTaker >= 0 {
		s.takePenalty()
		return
	}
	if s.fkTaker >= 0 {
		s.takeFreeKick()
		return
	}
	for _, p := range s.pl {
		p.stun = math.Max(0, p.stun-simDT)
		p.shield = math.Max(0, p.shield-simDT)
		p.turbo = math.Max(0, p.turbo-simDT)
	}
	if s.fl.active {
		s.advanceFlight()
		if s.freeze > 0 {
			return
		}
	}
	s.movePlayers()
	if !s.fl.active && s.owner >= 0 {
		s.ball = s.pl[s.owner].pos
		s.duels()
	}
	if !s.fl.active && s.owner >= 0 {
		s.ball = s.pl[s.owner].pos
		s.decide()
	}
	s.res.Stats[s.poss].Poss++
}

func (s *sim) nearestOpp(p *simPlayer, at vec) float64 {
	best := 99.0
	for _, i := range s.teamIdx[1-p.team] {
		o := s.pl[i]
		if o.gk || o.stun > 0 {
			continue
		}
		if d := dist(o.pos, at); d < best {
			best = d
		}
	}
	return best
}

func (s *sim) movePlayers() {
	focus := s.ball
	switch {
	case s.fl.active && s.fl.kind == flightPass:
		focus = s.pl[s.fl.target].pos
	case s.fl.active && s.fl.kind == flightLoose:
		focus = s.fl.to
	}
	// Players move one team after the other, so the second team reacts to
	// slightly fresher positions. Alternating who goes first every tick keeps
	// that from favouring one side of the pitch.
	for k := 0; k < 2; k++ {
		team := (k + s.tick) % 2
		attacking := team == s.poss
		tac := s.tac[team]
		ballAdv := adv(team, s.ball.x)
		pressers := map[int]bool{}
		if !attacking {
			var cand []int
			for _, i := range s.teamIdx[team] {
				if p := s.pl[i]; !p.gk && p.stun == 0 && !(adv(team, focus.x) > deepPress && s.holds(p)) {
					cand = append(cand, i)
				}
			}
			sort.SliceStable(cand, func(a, b int) bool { return dist(s.pl[cand[a]].pos, focus) < dist(s.pl[cand[b]].pos, focus) })
			for k := 0; k < len(cand) && k < tac.Pressers; k++ {
				pressers[cand[k]] = true
			}
		}
		for _, i := range s.teamIdx[team] {
			p := s.pl[i]
			if p.stun > 0 {
				continue
			}
			var tgt vec
			speedMul := 1.0
			if !attacking {
				speedMul = or1(p.fx.defSpeed)
			}
			switch {
			case p.gk:
				if i == s.owner {
					continue
				}
				tgt = vec{p.home.x, clamp(30+(s.ball.y-30)*0.3, 24, 36)}
			case i == s.owner:
				a := adv(team, p.pos.x)
				if a >= deepCarry && s.holds(p) {
					tgt = p.pos // holds his line and looks for a pass
				} else if a < 78 {
					tgt = vec{adv(team, a+10), p.pos.y + (30-p.pos.y)*0.15}
				} else {
					tgt = vec{adv(team, 97), 30}
				}
				// Lean away from the closest defender ahead.
				for _, j := range s.teamIdx[1-team] {
					o := s.pl[j]
					if o.gk || o.stun > 0 {
						continue
					}
					if d := dist(o.pos, p.pos); d < 6 && adv(team, o.pos.x) > a {
						// Team 1 plays on a mirrored pitch, so the tie-break is mirrored too.
						if p.pos.y > o.pos.y || (p.pos.y == o.pos.y && team == 0) {
							tgt.y += 4
						} else {
							tgt.y -= 4
						}
						break
					}
				}
				speedMul = 0.9 * or1(p.fx.ballSpeed)
			case s.fl.active && s.fl.kind == flightPass && s.fl.target == i:
				continue
			case attacking:
				a := [3]float64{15, 28, 40}[p.line] + [3]float64{0.45, 0.6, 0.62}[p.line]*ballAdv + tac.Push*18
				if ballAdv > 45 {
					a += p.fx.boxRunner
				}
				a = clamp(a, 6, 93)
				if s.holds(p) {
					a = math.Min(15+0.4*ballAdv+tac.Push*14, deepSupport)
				}
				// A little off-the-ball movement, identical for both sides on their mirrored pitch.
				sway := 2.5 * math.Sin(float64(s.tick)*0.06+float64(i-s.gk[team])*1.7)
				if team == 1 {
					sway = -sway
				}
				y := p.home.y + (s.ball.y-p.home.y)*0.12 + sway
				tgt = vec{adv(team, a), y}
			case pressers[i]:
				tgt = focus
				speedMul *= s.mods[team].PressSpeed * tac.PressSpeed
			default:
				a := [3]float64{8, 16, 30}[p.line] + [3]float64{0.32, 0.5, 0.55}[p.line]*ballAdv + tac.Push*12
				a = clamp(a, 5, 80)
				tgt = vec{adv(team, a), p.home.y + (s.ball.y-30)*0.35}
			}
			tgt.x = clamp(tgt.x, 1, PitchL-1)
			tgt.y = clamp(tgt.y, 2, PitchW-2)
			v := (8.5 + 10*s.stat(p, SPac)/100) * speedMul
			if p.turbo > 0 {
				v *= 1.4
			}
			d := tgt.sub(p.pos)
			l := d.length()
			step := v * simDT
			if l <= step {
				p.pos = tgt
			} else {
				p.pos = p.pos.add(d.mul(step / l))
			}
			if l > 0.5 {
				drain := 0.011 * 100 / (40 + s.stat(p, SSta)) * s.mods[team].Drain * tac.Drain * or1(p.fx.drain)
				p.stam = math.Max(0, p.stam-drain*simDT)
			}
		}
	}
}

// Where a manager places a player decides how far he goes. With team-mates
// in front of him, a player on the defence line holds his position: he stays
// in his own half in possession, hands the ball on instead of carrying it
// forward, and does not leave the back line to press high up the pitch.
// Alone at the back (nobody ahead of him) he has to play the whole pitch, and
// under All-Out Attack the back line joins in like everyone else.
const (
	deepCarry   = 50.0 // how far a holding defender takes the ball himself
	deepSupport = 52.0 // how far he follows an attack
	deepPress   = 60.0 // beyond this the back line leaves the pressing to others
	throughFrom = 62.0 // a break only counts from here on: closer to his own goal he still builds up
	throughLane = 10.0 // an opponent this close to the straight line to goal is still in the way
	allOutPush  = 0.3  // an order that pushes the shape this far up sends the back line forward too
)

// holds reports whether p is on the defence line with a team-mate ahead of him.
func (s *sim) holds(p *simPlayer) bool {
	if p.gk || p.line != 0 || s.tac[p.team].Push >= allOutPush {
		return false
	}
	for _, i := range s.teamIdx[p.team] {
		if m := s.pl[i]; !m.gk && m.line > 0 {
			return true
		}
	}
	return false
}

// through reports whether the player on the ball has broken clear: he is well
// inside the opponent's half and no outfield opponent stands between him and the
// goal. A defender chasing from behind does not count.
func (s *sim) through(o *simPlayer) bool {
	oa := adv(o.team, o.pos.x)
	if oa < throughFrom || s.holds(o) {
		return false
	}
	goal := vec{goalX(o.team), PitchW / 2}
	for _, j := range s.teamIdx[1-o.team] {
		d := s.pl[j]
		if d.gk || d.stun > 0 {
			continue
		}
		if adv(o.team, d.pos.x) > oa && segDist(o.pos, goal, d.pos) < throughLane {
			return false
		}
	}
	return true
}

// breakAway starts a counter-attack for a team under that order: once the
// ball is won, midfield and attack sprint forward for a moment.
func (s *sim) breakAway(team int) {
	c := s.tac[team].Counter + s.mods[team].Counter
	if c <= 0 {
		return
	}
	for _, i := range s.teamIdx[team] {
		if p := s.pl[i]; !p.gk && p.line >= 1 {
			p.turbo = math.Max(p.turbo, c)
		}
	}
}

// addMoment fills the Moment bar of players whose trait is an active one.
func (s *sim) addMoment(p *simPlayer, amt float64) {
	if p.gk || p.fx.active == "" {
		return
	}
	p.moment += amt * s.mods[p.team].MomentMul * s.tac[p.team].MomentMul
	if p.moment < 100 {
		return
	}
	p.moment = 0
	switch p.fx.active {
	case "turbo":
		p.turbo = 3
	case "laser":
		p.surePass, p.sureBoost = true, 1.3
	case "lock":
		p.sureDuel = true
	case "ghost":
		p.shield = 3
		p.shotBoost = math.Max(p.shotBoost, 1.5)
	}
	s.event("ability", p.team, p.idx, -1, 0)
}

func (s *sim) duels() {
	o := s.pl[s.owner]
	if o.gk || o.shield > 0 {
		return
	}
	for _, i := range s.teamIdx[1-o.team] {
		d := s.pl[i]
		if d.gk || d.stun > 0 || d.shield > 0 || dist(d.pos, o.pos) > 2.3 {
			continue
		}
		// Keeping the ball is about dribbling, not strength: a centre-back cannot
		// shoulder his way past a pressing forward. Winning it is mostly defending,
		// helped by strength and pace.
		atk := 0.9*s.stat(o, SDri) + 0.1*s.stat(o, SPac) + o.fx.duelAtk
		def := 0.65*s.stat(d, SDef) + 0.2*s.stat(d, SPhy) + 0.15*s.stat(d, SPac) + d.fx.duelDef
		if adv(d.team, d.pos.x) > 50 {
			def += d.fx.highDuelDef
		}
		pDef := 0.06 + 0.84/(1+math.Exp(-(def-atk)/10))
		if d.sureDuel {
			pDef = 1
			d.sureDuel = false
		}
		won := s.rng.Float() < pDef
		// Any duel can end in a foul. It is most likely when the defender is
		// beaten and brings the attacker down; a tackle that wins the ball is
		// only sometimes judged illegal. Both are far more likely in the
		// scramble of the penalty area. A side on Full Press dives in more.
		inBox := adv(o.team, o.pos.x) >= 85 && math.Abs(o.pos.y-PitchW/2) <= 16
		pFoul := 0.11 // beaten in open play
		switch {
		case inBox && !won:
			pFoul = 0.2
		case inBox:
			pFoul = 0.08
		case won:
			pFoul = 0.05
		}
		if s.tac[d.team].Pressers >= 3 {
			pFoul *= 1.3
		}
		// Some teams, and some players, know how to win a foul.
		pFoul = math.Min(0.6, pFoul*s.mods[o.team].FoulDraw*or1(o.fx.foulDraw))
		if s.rng.Float() < pFoul {
			s.foul(o, d, inBox)
			return
		}
		if won {
			s.res.Stats[d.team].Tackles++
			o.stun = 0.9
			if d.fx.stunOnTackle > 0 {
				o.stun = d.fx.stunOnTackle
			}
			d.shield = 0.8
			if d.fx.safeAfterTackle {
				d.surePass, d.sureBoost = true, 1
			}
			if d.fx.afterTackleTurbo > 0 {
				d.turbo = math.Max(d.turbo, d.fx.afterTackleTurbo)
			}
			s.setOwner(d.idx)
			s.breakAway(d.team)
			s.event("tackle", d.team, d.idx, o.idx, 0)
			s.addMoment(d, 25)
		} else {
			d.stun = 1.1
			o.shield = 0.5
			if o.fx.afterDuelBoost > 0 && adv(o.team, o.pos.x) > 50 {
				o.shotBoost = math.Max(o.shotBoost, o.fx.afterDuelBoost)
			}
			if o.fx.afterDuelFree > 0 {
				o.shield = o.fx.afterDuelFree
				o.turbo = math.Max(o.turbo, o.fx.afterDuelFree)
			}
			s.event("dribble", o.team, o.idx, d.idx, 0)
			s.addMoment(o, 20)
		}
		return
	}
}

func (s *sim) xg(o *simPlayer, d, press float64) float64 {
	def := 1 - o.team
	keeper := s.pl[s.gk[def]]
	v := 0.55 * math.Exp(-d/14)
	v *= 0.55 + s.stat(o, SSho)/100
	if !o.fx.wideShot {
		v *= 1 - math.Abs(o.pos.y-30)/30*0.6
	}
	if press < 3 && !o.fx.ignorePressure {
		v *= 0.75
	}
	v *= 1.25 - keeper.ref/100*0.6
	v *= or1(keeper.fx.gkAll)
	if d > 20 {
		v *= s.mods[o.team].LongShot * or1(o.fx.longShot) * or1(keeper.fx.gkLong)
		v *= or1(s.tac[o.team].LongShot) * or1(s.tac[def].GKOffLine)
	}
	if d <= 12 {
		v *= or1(o.fx.closeShot) * or1(keeper.fx.gkClose)
	}
	if s.half == 1 {
		v *= or1(o.fx.secondHalfShot)
	} else {
		v *= or1(o.fx.firstHalfShot)
	}
	if s.score[o.team] <= s.score[def] {
		v *= or1(o.fx.clutchShot)
	}
	v *= o.shotBoost * s.mods[def].OppXG
	concede := s.tac[def].Concede
	if d > 20 && s.tac[o.team].LongShot > 0 {
		// A packed box does not block shots from distance; it leaves room for them.
		concede = math.Max(concede, 1) * (1 + s.tac[o.team].Edge*math.Max(0, -s.tac[def].Push))
	}
	v *= s.tac[o.team].Attack * concede * (1 + s.tac[o.team].Behind*math.Max(0, s.tac[def].Push))
	return clamp(v, 0.02, 0.85)
}

func (s *sim) decide() {
	s.decT -= simDT
	if s.decT > 0 {
		return
	}
	s.decT = 0.4
	o := s.pl[s.owner]
	if o.gk {
		s.gkDistribute(o)
		return
	}
	tac := s.tac[o.team]
	goal := vec{goalX(o.team), PitchW / 2}
	d := dist(o.pos, goal)
	press := s.nearestOpp(o, o.pos)
	if d < 36 {
		xg := s.xg(o, d, press)
		p := 1.0
		if xg < 0.22 && d > 9 {
			p = clamp(xg*2.4+tac.ShootBias, 0.01, 1) * 0.7
		}
		if s.rng.Float() < p {
			s.shoot(o, xg, d)
			return
		}
	}
	best, bestScore := -1, -math.MaxFloat64
	oa := adv(o.team, o.pos.x)
	for _, i := range s.teamIdx[o.team] {
		m := s.pl[i]
		if m.gk || i == o.idx || m.stun > 0 {
			continue
		}
		dm := dist(m.pos, o.pos)
		sc := (adv(o.team, m.pos.x)-oa)*(0.05+tac.Forward) + math.Min(s.nearestOpp(m, m.pos), 10)*0.08 - math.Max(0, dm-30)*0.05
		if dm < 5 {
			sc--
		}
		if sc > bestScore {
			best, bestScore = i, sc
		}
	}
	if best < 0 {
		return
	}
	// Poor dribblers feel pressure from further away and get rid of the ball
	// instead of taking a man on.
	nervous := 4 + math.Max(0, 62-s.stat(o, SDri))/4
	pp := 0.08
	switch {
	case o.surePass:
		pp = 1
	case press < 4:
		pp = 0.8
	case press < nervous:
		pp = 0.9
	case bestScore > 0.9:
		pp = 0.35
	}
	if s.through(o) && !o.surePass {
		return // clean through on goal: he keeps running and shoots, he does not look for a pass
	}
	switch {
	case s.holds(o) && oa > deepCarry-12:
		pp = math.Max(pp, 0.85) // a holding defender moves the ball on
	case o.line == 1:
		pp += 0.08 // the midfield is where the passing goes through
	}
	if s.rng.Float() < pp+tac.PassBias {
		s.pass(o, best)
	}
}

func segDist(a, b, p vec) float64 {
	ab := b.sub(a)
	l2 := ab.x*ab.x + ab.y*ab.y
	if l2 == 0 {
		return dist(a, p)
	}
	t := clamp(((p.x-a.x)*ab.x+(p.y-a.y)*ab.y)/l2, 0, 1)
	return dist(p, a.add(ab.mul(t)))
}

func (s *sim) pass(o *simPlayer, target int) {
	m := s.pl[target]
	d := dist(o.pos, m.pos)
	p := 0.62 + s.stat(o, SPas)/100*0.33 + s.mods[o.team].PassAcc + s.tac[o.team].PassAcc + o.fx.passAcc
	if !o.fx.longPass {
		p -= d * 0.004
	}
	interceptor, bestD := -1, 99.0
	near := 0
	for _, i := range s.teamIdx[1-o.team] {
		x := s.pl[i]
		if x.gk || x.stun > 0 {
			continue
		}
		sd := segDist(o.pos, m.pos, x.pos)
		if sd < 4 {
			near++
		}
		if sd < 6 {
			p -= x.fx.laneBlock
		}
		if sd < bestD {
			bestD, interceptor = sd, i
		}
	}
	p -= 0.12 * math.Min(float64(near), 2)
	p = clamp(p, 0.2, 0.97)
	boost := or1(o.fx.recvBoost)
	if o.fx.recvBoostFinalThird > 0 && adv(o.team, m.pos.x) > 66 {
		boost = math.Max(boost, o.fx.recvBoostFinalThird)
	}
	if o.surePass {
		p = 1
		boost = math.Max(boost, o.sureBoost)
		o.surePass = false
	}
	extra := o.fx.recvMoment
	s.res.Stats[o.team].Passes++
	if s.rng.Float() >= p {
		if interceptor >= 0 && bestD <= interceptReach {
			// A defender standing on the passing line cuts it out.
			target, boost, extra = interceptor, 1, 0
		} else {
			// Nobody is close enough to intercept: the pass is simply misplaced.
			// It runs past the receiver and whoever is nearest collects it.
			dir := m.pos.sub(o.pos)
			if l := dir.length(); l > 0 {
				dir = dir.mul(1 / l)
			}
			side := vec{-dir.y, dir.x}.mul((s.rng.Float()*2 - 1) * 6)
			to := m.pos.add(dir.mul(4 + s.rng.Float()*6)).add(side)
			to.x, to.y = clamp(to.x, 2, PitchL-2), clamp(to.y, 2, PitchW-2)
			s.fl = flight{active: true, kind: flightLoose, from: o.pos, to: to, dur: math.Max(0.25, dist(o.pos, to)/38), team: o.team, shooter: o.idx}
			s.owner = -1
			s.event("pass", o.team, o.idx, -1, 0)
			return
		}
	}
	s.fl = flight{active: true, kind: flightPass, from: o.pos, target: target, dur: math.Max(0.25, d/38), team: o.team, boost: boost, extraMoment: extra}
	s.owner = -1
	s.event("pass", o.team, o.idx, target, 0)
}

func (s *sim) gkDistribute(o *simPlayer) {
	best, bestScore := -1, -math.MaxFloat64
	tac := s.tac[o.team]
	advW := 0.01
	if o.fx.gkSweeper || tac.GKLong {
		advW = 0.06
	}
	for _, i := range s.teamIdx[o.team] {
		m := s.pl[i]
		if m.gk || m.stun > 0 {
			continue
		}
		sc := math.Min(s.nearestOpp(m, m.pos), 12)*0.1 + adv(o.team, m.pos.x)*advW
		if sc > bestScore {
			best, bestScore = i, sc
		}
	}
	if best < 0 {
		// Nobody to give it to. With no outfield teammates at all the keeper
		// cannot hold the ball forever: he clears it, straight to the opposition.
		for _, i := range s.teamIdx[o.team] {
			if !s.pl[i].gk {
				return // teammates exist, they are only down for a moment
			}
		}
		target, bd := s.gk[1-o.team], 999.0
		for _, i := range s.teamIdx[1-o.team] {
			if x := s.pl[i]; !x.gk {
				if d := dist(x.pos, o.pos); d < bd {
					bd, target = d, i
				}
			}
		}
		s.res.Stats[o.team].Passes++
		s.fl = flight{active: true, kind: flightPass, from: o.pos, target: target, dur: math.Max(0.4, dist(o.pos, s.pl[target].pos)/38), team: o.team, boost: 1}
		s.owner = -1
		s.event("pass", o.team, o.idx, target, 0)
		return
	}
	target := best
	if !o.fx.gkSafe && s.rng.Float() >= 1-tac.GKIntercept {
		bd := 99.0
		for _, i := range s.teamIdx[1-o.team] {
			x := s.pl[i]
			if x.gk || x.stun > 0 {
				continue
			}
			if d := dist(x.pos, s.pl[best].pos); d < bd {
				bd, target = d, i
			}
		}
	}
	s.res.Stats[o.team].Passes++
	s.fl = flight{active: true, kind: flightPass, from: o.pos, target: target, dur: math.Max(0.3, dist(o.pos, s.pl[target].pos)/38), team: o.team, boost: 1}
	s.owner = -1
	s.event("pass", o.team, o.idx, target, 0)
}

func (s *sim) shoot(o *simPlayer, xg, d float64) {
	o.shotBoost = 1
	st := &s.res.Stats[o.team]
	st.Shots++
	st.XG += xg
	keeper := s.pl[s.gk[1-o.team]]
	gx := goalX(o.team)
	sign := 1.0
	if s.rng.Float() < 0.5 {
		sign = -1
	}
	r := s.rng.Float()
	f := flight{active: true, kind: flightShot, from: o.pos, dur: math.Max(0.3, d/45), shooter: o.idx, team: o.team}
	switch {
	case r < xg:
		f.outcome = outGoal
		f.to = vec{gx, 30 + sign*2.5}
		// Keeper trait: a share of certain goals is clawed away.
		if keeper.fx.gkSecond > 0 && s.rng.Float() < keeper.fx.gkSecond {
			f.outcome = outSave
			f.to = keeper.pos
		}
	case r < xg+(1-xg)*0.55:
		f.outcome = outSave
		f.to = keeper.pos
	default:
		f.outcome = outMiss
		f.to = vec{gx, 30 + sign*8}
	}
	s.fl = f
	s.owner = -1
	s.event("shot", o.team, o.idx, -1, xg)
	s.addMoment(o, 15)
}

func (s *sim) advanceFlight() {
	f := &s.fl
	f.t += simDT
	frac := math.Min(1, f.t/f.dur)
	to := f.to
	if f.kind == flightPass {
		to = s.pl[f.target].pos
	}
	s.ball = f.from.add(to.sub(f.from).mul(frac))
	if frac < 1 {
		return
	}
	f.active = false
	if f.kind == flightLoose {
		// The ball has stopped: the nearest player who is on his feet collects it.
		// It then rolls the last few steps to him, so it never jumps across the pitch.
		best, bd := -1, math.Inf(1)
		for _, p := range s.pl {
			if p.idx == f.shooter || p.stun > 0 || p.off {
				continue
			}
			if d := dist(p.pos, s.ball); d < bd {
				bd, best = d, p.idx
			}
		}
		if best < 0 {
			best = f.shooter
		}
		s.fl = flight{active: true, kind: flightPass, from: s.ball, target: best, dur: math.Max(0.2, dist(s.pl[best].pos, s.ball)/24), team: f.team, boost: 1}
		return
	}
	if f.kind == flightPass {
		recv := s.pl[f.target]
		s.setOwner(recv.idx)
		if recv.team == f.team {
			recv.shield = 0.35
			if recv.fx.recvShield > 0 && adv(recv.team, recv.pos.x) > 66 {
				recv.shield = recv.fx.recvShield
			}
			if f.boost > 1 {
				recv.shotBoost = math.Max(recv.shotBoost, f.boost)
			}
			s.addMoment(recv, 18+f.extraMoment)
		} else {
			recv.shield = 0.5
			s.breakAway(recv.team)
			s.event("intercept", recv.team, recv.idx, -1, 0)
			s.addMoment(recv, 20)
		}
		return
	}
	def := 1 - f.team
	switch f.outcome {
	case outGoal:
		s.score[f.team]++
		s.event("goal", f.team, f.shooter, -1, 0)
		s.kickoff(def)
		s.freeze = 18
	case outSave:
		s.event("save", def, s.gk[def], f.shooter, 0)
		s.setOwner(s.gk[def])
		s.decT = s.tac[def].GKHold
	default:
		s.event("miss", f.team, f.shooter, -1, 0)
		s.setOwner(s.gk[def])
		s.decT = s.tac[def].GKHold
	}
}

func (s *sim) record() {
	fr := make(Frame, 0, 3+3*len(s.pl))
	fr = append(fr, int(math.Round(s.ball.x*10)), int(math.Round(s.ball.y*10)), s.owner)
	for _, p := range s.pl {
		flags := 0
		if p.off {
			flags |= 8
		}
		if p.stun > 0 {
			flags |= 1
		}
		if p.turbo > 0 {
			flags |= 2
		}
		if p.shotBoost > 1 || p.surePass || p.sureDuel {
			flags |= 4
		}
		fr = append(fr, int(math.Round(p.pos.x*10)), int(math.Round(p.pos.y*10)), flags)
	}
	s.res.Frames = append(s.res.Frames, fr)
}

// foul stops play after an illegal challenge: possibly a card for the
// offender, then a penalty in the box or a free kick anywhere else.
func (s *sim) foul(fouled, by *simPlayer, inBox bool) {
	s.event("foul", by.team, by.idx, fouled.idx, 0)
	// Cards: a straight red is rare, a yellow common; fouls in the box are judged harder.
	pRed, pYellow := 0.03, 0.34
	if inBox {
		pRed, pYellow = 0.06, 0.45
	}
	switch r := s.rng.Float(); {
	case r < pRed:
		// The length of the ban is luck: usually one match, seldom three.
		matches := 1
		if q := s.rng.Float(); q > 0.92 {
			matches = 3
		} else if q > 0.65 {
			matches = 2
		}
		s.sendOff(by, true, matches)
	case r < pRed+pYellow:
		if by.yellow {
			s.sendOff(by, false, 1)
		} else {
			by.yellow = true
			s.event("yellow", by.team, by.idx, -1, 0)
		}
	}
	if inBox {
		s.awardPenalty(fouled)
		return
	}
	if s.directFreeKick(fouled) {
		return
	}
	// Free kick: the fouled player keeps the ball and gets room to play it.
	fouled.stun, fouled.shield = 0, 1.2
	if !by.off {
		by.stun = 0.8
	}
	s.setOwner(fouled.idx)
	s.freeze = 8
}

// freeKickRange is how far from goal a free kick is still struck at goal.
const freeKickRange = 30.0

// specialistFreeKick is the quality of a direct free kick for a team that has
// a free-kick specialist on the pitch but not the Set Pieces style.
const specialistFreeKick = 0.05

// directFreeKick sets up a shot from a free kick near the box. A team goes for
// goal if it has worked on set pieces (Mods.FreeKick) or has a specialist on
// the pitch; everyone else plays the free kick short. The best taker strikes it.
func (s *sim) directFreeKick(fouled *simPlayer) bool {
	att := fouled.team
	goal := vec{goalX(att), PitchW / 2}
	if dist(fouled.pos, goal) > freeKickRange {
		return false
	}
	var taker *simPlayer
	for _, i := range s.teamIdx[att] {
		p := s.pl[i]
		if p.gk {
			continue
		}
		if taker == nil || or1(p.fx.freeKick)*p.base[SSho] > or1(taker.fx.freeKick)*taker.base[SSho] {
			taker = p
		}
	}
	if taker == nil || (s.mods[att].FreeKick <= 0 && taker.fx.freeKick <= 1) {
		return false
	}
	spot := fouled.pos
	for _, p := range s.pl {
		if !p.off {
			p.stun, p.shield, p.turbo = 0, 0, 0
		}
	}
	taker.pos = spot
	s.fl.active = false
	s.owner, s.poss, s.ball = taker.idx, att, spot
	s.fkTaker = taker.idx
	s.freeze = 10
	s.event("freekick", att, taker.idx, fouled.idx, 0)
	return true
}

// takeFreeKick strikes the free kick set up by directFreeKick.
func (s *sim) takeFreeKick() {
	o := s.pl[s.fkTaker]
	s.fkTaker = -1
	def := 1 - o.team
	keeper := s.pl[s.gk[def]]
	d := dist(o.pos, vec{goalX(o.team), PitchW / 2})
	xg := math.Max(s.mods[o.team].FreeKick, specialistFreeKick) * or1(o.fx.freeKick) * (0.8 + 0.4*o.base[SSho]*s.mods[o.team].Stat[SSho]/100)
	if d > 24 {
		xg *= 0.75
	}
	xg *= s.mods[def].OppXG * or1(keeper.fx.gkAll) * or1(keeper.fx.gkLong)
	s.shoot(o, clamp(xg, 0.02, 0.5), d)
}

// sendOff removes a player from the match and records his suspension.
func (s *sim) sendOff(p *simPlayer, straight bool, matches int) {
	p.off = true
	p.pos = vec{-20, -20}
	var keep []int
	for _, i := range s.teamIdx[p.team] {
		if i != p.idx {
			keep = append(keep, i)
		}
	}
	s.teamIdx[p.team] = keep
	typ := "red2"
	if straight {
		typ = "red"
	}
	s.event(typ, p.team, p.idx, matches, 0)
	s.res.SentOff = append(s.res.SentOff, SentOff{Team: p.team, Def: p.def, Line: p.line, Lane: p.lane, Straight: straight, Matches: matches})
}

// penaltyChance is the probability that a spot kick goes in.
func (s *sim) penaltyChance(taker, keeper *simPlayer) float64 {
	sho := taker.base[SSho] * s.mods[taker.team].Stat[SSho]
	p := clamp(0.76+(sho-keeper.ref)/250, 0.55, 0.92) + s.mods[taker.team].PenBonus + taker.fx.penBonus
	return math.Min(p, 0.96) * or1(keeper.fx.gkAll)
}

// bestTaker is the outfield player with the best shot on a team (the keeper if alone).
func (s *sim) bestTaker(team int) *simPlayer {
	best := s.pl[s.gk[team]]
	for _, i := range s.teamIdx[team] {
		if p := s.pl[i]; !p.gk && (best.gk || p.base[SSho] > best.base[SSho]) {
			best = p
		}
	}
	return best
}

// awardPenalty sets up the spot kick after a foul in the box.
func (s *sim) awardPenalty(fouled *simPlayer) {
	att := fouled.team
	taker := s.bestTaker(att)
	spot := vec{adv(att, 89), PitchW / 2}
	for _, p := range s.pl {
		if p.off {
			continue
		}
		p.stun, p.shield, p.turbo = 0, 0, 0
		switch {
		case p.gk:
			p.pos = p.home
		case adv(att, p.pos.x) > 80:
			// Everyone else leaves the area.
			p.pos.x = adv(att, 79-float64(p.idx%4))
		}
	}
	taker.pos = vec{adv(att, 87.5), PitchW / 2}
	s.fl.active = false
	s.owner, s.poss, s.ball = taker.idx, att, spot
	s.penTaker = taker.idx
	s.freeze = 14
	s.event("penalty", att, taker.idx, fouled.idx, 0)
}

// takePenalty strikes the spot kick set up by awardPenalty.
func (s *sim) takePenalty() {
	o := s.pl[s.penTaker]
	s.penTaker = -1
	keeper := s.pl[s.gk[1-o.team]]
	p := s.penaltyChance(o, keeper)
	st := &s.res.Stats[o.team]
	st.Shots++
	st.XG += p
	gx := goalX(o.team)
	sign := 1.0
	if s.rng.Float() < 0.5 {
		sign = -1
	}
	r := s.rng.Float()
	f := flight{active: true, kind: flightShot, from: s.ball, dur: 0.4, shooter: o.idx, team: o.team}
	switch {
	case r < p:
		f.outcome, f.to = outGoal, vec{gx, PitchW/2 + sign*3}
	case r < p+(1-p)*0.75:
		f.outcome = outSave
		keeper.pos.y = PitchW/2 + sign*3
		f.to = keeper.pos
	default:
		f.outcome, f.to = outMiss, vec{gx, PitchW/2 + sign*8}
	}
	s.fl = f
	s.owner = -1
	s.event("shot", o.team, o.idx, -1, p)
}

// shootoutKicks is how many kicks each side takes before sudden death. Three
// keeps a shootout short enough to watch after every drawn match.
const shootoutKicks = 3

// shootout settles a draw: three kicks each, then sudden death. Every kick is
// played out on the pitch and recorded as frames, so it can be watched.
func (s *sim) shootout() {
	var order [2][]*simPlayer
	for t := 0; t < 2; t++ {
		for _, i := range s.teamIdx[t] {
			if !s.pl[i].gk {
				order[t] = append(order[t], s.pl[i])
			}
		}
		sort.SliceStable(order[t], func(a, b int) bool { return order[t][a].base[SSho] > order[t][b].base[SSho] })
	}
	// All kicks are taken at the right-hand goal. The others wait at the halfway line.
	s.fl.active = false
	spot := vec{89, PitchW / 2}
	line := func() {
		var n [2]int
		for _, p := range s.pl {
			if p.off {
				continue
			}
			p.stun, p.shield, p.turbo = 0, 0, 0
			x := 46.0
			if p.team == 1 {
				x = 54
			}
			p.pos = vec{x, 14 + 5*float64(n[p.team])}
			n[p.team]++
		}
	}
	hold := func(ticks int) {
		for i := 0; i < ticks; i++ {
			s.tick++
			s.record()
		}
	}
	var pens [2]int
	kick := func(t, n int) {
		taker := s.pl[s.gk[t]]
		if len(order[t]) > 0 {
			taker = order[t][n%len(order[t])]
		}
		keeper := s.pl[s.gk[1-t]]
		line()
		keeper.pos = vec{99, PitchW / 2}
		taker.pos = vec{87.5, PitchW / 2}
		s.owner, s.poss, s.ball = taker.idx, t, spot
		hold(6)
		p := clamp(s.penaltyChance(taker, keeper)-keeper.fx.gkPens, 0.4, 0.92)
		sign := 1.0
		if s.rng.Float() < 0.5 {
			sign = -1
		}
		r := s.rng.Float()
		ok := r < p
		to := vec{100, PitchW/2 + sign*3}
		switch {
		case ok:
		case r < p+(1-p)*0.75:
			keeper.pos.y = PitchW/2 + sign*3
			to = keeper.pos
		default:
			to = vec{100.5, PitchW/2 + sign*8}
		}
		s.owner = -1
		for i := 1; i <= 4; i++ {
			s.ball = spot.add(to.sub(spot).mul(float64(i) / 4))
			s.tick++
			s.record()
		}
		if ok {
			pens[t]++
			s.event("pengoal", t, taker.idx, -1, 0)
		} else {
			s.event("penmiss", t, taker.idx, keeper.idx, 0)
		}
		s.res.Pens = append(s.res.Pens, PenKick{Team: t, Scored: ok})
		hold(7)
	}
	s.event("shootout", 0, -1, -1, 0)
	for n := 0; n < s.kicks; n++ {
		kick(0, n)
		kick(1, n)
	}
	for n := s.kicks; pens[0] == pens[1] && n < 30; n++ {
		kick(0, n)
		kick(1, n)
	}
	if pens[1] > pens[0] {
		s.res.Winner = 1
	} else {
		s.res.Winner = 0
	}
}
