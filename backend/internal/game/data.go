// Package game holds every rule of the game. Nothing here trusts the client:
// the HTTP layer only forwards intents, and this package decides what happens.
package game

// Stat indexes into PlayerDef.Stats.
const (
	SPac = iota
	SSho
	SPas
	SDri
	SDef
	SPhy
	SSta
)

// GKLine is the pitch line of the goalkeeper slot (lane 0). Lines 0..2 are outfield.
const GKLine = 3

// AcademyKeeper is the keeper every club starts with. It never appears in the market.
const AcademyKeeper = "academy_gk"

type PlayerDef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Cost      int    `json:"cost"`
	Style     string `json:"style"`
	Role      string `json:"role"`
	Stats     [7]int `json:"stats"`
	Ref       int    `json:"ref,omitempty"` // goalkeepers only
	Trait     string `json:"trait"`
	TraitDesc string `json:"traitDesc"`
	Active    bool   `json:"active"`         // trait fires when the Moment bar fills
	Wiki      string `json:"wiki,omitempty"` // Wikipedia page used for the portrait
}

// traitFx is the mechanical side of a player's unique trait. Zero values mean "no effect".
type traitFx struct {
	longShot, closeShot                float64 // xG multipliers by distance
	firstHalfShot, secondHalfShot      float64 // xG multipliers by half
	clutchShot                         float64 // xG multiplier while his team is not ahead
	wideShot, ignorePressure           bool
	duelAtk, duelDef, highDuelDef      float64 // added to duel ratings
	stunOnTackle                       float64 // seconds the tackled player stays down
	afterTackleTurbo                   float64 // sprint seconds after winning a tackle
	drain                              float64 // stamina drain multiplier
	defSpeed, ballSpeed                float64 // speed multipliers
	passAcc                            float64 // added to his own pass accuracy
	longPass                           bool    // distance does not hurt his passes
	recvBoost, recvBoostFinalThird     float64 // shot boost handed to whoever receives his pass
	recvMoment                         float64 // extra Moment for the receiver
	recvShield                         float64 // untouchable seconds after receiving in the final third
	laneBlock                          float64 // opponent pass penalty when he is near the lane
	afterDuelBoost, afterDuelFree      float64
	safeAfterTackle                    bool
	boxRunner                          float64
	teamMoment, teamPassAcc, teamOppXG float64 // auras
	teamDef, teamPress, teamDrain      float64 // auras
	active                             string  // turbo | laser | lock | ghost
	gkAll, gkLong, gkClose             float64 // xG-against multipliers
	gkSafe, gkSweeper                  bool
	gkPens, gkSecond                   float64
	foulDraw                           float64 // multiplies how often he is fouled
	penBonus                           float64 // added to the chance that his penalty scores
	freeKick                           float64 // multiplies the quality of his direct free kicks
}

type playerSpec struct {
	PlayerDef
	fx traitFx
}

// Stats come from a position template scaled by cost, so a cost-5 legend is
// clearly better than a cost-1 squad player, plus a few personal tweaks.
// Order: pace, shooting, passing, dribbling, defending, physical, stamina.
var roleStats = map[string][7]float64{
	"striker":   {72, 80, 58, 70, 28, 72, 66},
	"winger":    {84, 68, 62, 80, 30, 50, 66},
	"playmaker": {64, 68, 84, 80, 38, 48, 64},
	"anchor":    {62, 52, 72, 58, 78, 74, 80},
	"defender":  {62, 30, 54, 42, 82, 80, 68},
}

var costScale = [6]float64{0, 0.76, 0.84, 0.92, 1.0, 1.08}

type tw [7]int // personal stat tweaks

func outfield(id, name string, cost int, style, role, trait, desc, wiki string, fx traitFx, t tw) playerSpec {
	var st [7]int
	for k, v := range roleStats[role] {
		st[k] = int(v*costScale[cost]+0.5) + t[k]
	}
	return playerSpec{PlayerDef{ID: id, Name: name, Cost: cost, Style: style, Role: role, Stats: st, Trait: trait, TraitDesc: desc, Wiki: wiki}, fx}
}

func keeper(id, name string, cost, ref int, trait, desc, wiki string, fx traitFx) playerSpec {
	return playerSpec{PlayerDef{ID: id, Name: name, Cost: cost, Role: "gk", Ref: ref, Trait: trait, TraitDesc: desc, Wiki: wiki}, fx}
}

// The pool: eight styles with twelve outfield players and two keepers each,
// current stars next to all-time greats. Cost follows reputation: 1 is a good
// squad player, 5 a legend. A player sits in the style he actually stands for.
var specs = []playerSpec{
	// ---- Tiki-Taka: keep the ball, pass the opponent to death ----
	outfield("ferran", "Ferran Torres", 1, "tiki", "winger", "Ghost Run", "Drifts into the box whenever his team attacks.", "Ferran Torres", traitFx{boxRunner: 10}, tw{}),
	outfield("cucurella", "Marc Cucurella", 1, "tiki", "defender", "Terrier", "18% faster whenever his team does not have the ball.", "Marc Cucurella", traitFx{defSpeed: 1.18}, tw{}),
	outfield("zubimendi", "Martín Zubimendi", 2, "tiki", "anchor", "Screen", "Opponent passes that travel near him fail 10% more often.", "Martín Zubimendi", traitFx{laneBlock: 0.10}, tw{}),
	outfield("olmo", "Dani Olmo", 2, "tiki", "playmaker", "Half-Space", "Cannot be tackled for a moment after receiving the ball in the final third.", "Dani Olmo", traitFx{recvShield: 0.8}, tw{}),
	outfield("pedri", "Pedri", 3, "tiki", "playmaker", "La Pausa", "Moment: his next pass cannot fail and makes the receiver's shot 30% better.", "Pedri", traitFx{active: "laser"}, tw{}),
	outfield("saka", "Bukayo Saka", 3, "tiki", "winger", "Fast Start", "In the first half all his shots are 30% more dangerous.", "Bukayo Saka", traitFx{firstHalfShot: 1.3}, tw{}),
	outfield("odegaard", "Martin Ødegaard", 3, "tiki", "playmaker", "Conductor", "His own passes are 8% more accurate, and teammates who receive them gain extra Moment.", "Martin Ødegaard", traitFx{passAcc: 0.08, recvMoment: 14}, tw{}),
	outfield("rodri", "Rodri", 4, "tiki", "anchor", "Control", "While he is on the pitch, every teammate passes 8% more accurately.", "Rodri (footballer, born 1996)", traitFx{teamPassAcc: 0.08}, tw{}),
	outfield("kdb", "Kevin De Bruyne", 4, "tiki", "playmaker", "Killer Pass", "Passes into the final third make the receiver's next shot 45% better.", "Kevin De Bruyne", traitFx{recvBoostFinalThird: 1.45}, tw{2: 6}),
	outfield("xavi", "Xavi", 4, "tiki", "playmaker", "Carousel", "His passes are 15% more accurate and distance does not hurt them.", "Xavi (footballer, born 1980)", traitFx{passAcc: 0.15, longPass: true}, tw{2: 6}),
	outfield("messi", "Lionel Messi", 5, "tiki", "playmaker", "La Pulga", "Very hard to tackle (+10). Moment: untouchable for 3 seconds, and his next shot is 50% better.", "Lionel Messi", traitFx{active: "ghost", duelAtk: 10}, tw{1: 10, 3: 6}),
	outfield("iniesta", "Andrés Iniesta", 5, "tiki", "playmaker", "La Croqueta", "Very hard to tackle (+14). After beating a defender he is untouchable for 1.5 seconds, and his passes make the receiver's shot 30% better.", "Andrés Iniesta", traitFx{duelAtk: 14, afterDuelFree: 1.5, recvBoost: 1.3}, tw{}),

	// ---- Catenaccio: the door is bolted ----
	outfield("dragusin", "Radu Drăgușin", 1, "catenaccio", "defender", "Crunch", "Players he tackles stay down longer.", "Radu Drăgușin", traitFx{stunOnTackle: 1.6}, tw{}),
	outfield("amrabat", "Sofyan Amrabat", 1, "catenaccio", "anchor", "Atlas Lion", "20% faster whenever his team does not have the ball.", "Sofyan Amrabat", traitFx{defSpeed: 1.2}, tw{}),
	outfield("bastoni", "Alessandro Bastoni", 2, "catenaccio", "defender", "Ball-Playing", "After winning the ball, his next pass always reaches a teammate.", "Alessandro Bastoni", traitFx{safeAfterTackle: true}, tw{2: 8}),
	outfield("kante", "N'Golo Kanté", 2, "catenaccio", "anchor", "Everywhere", "25% faster whenever his team does not have the ball, and tires 30% slower.", "N'Golo Kanté", traitFx{defSpeed: 1.25, drain: 0.7}, tw{}),
	outfield("chiesa", "Federico Chiesa", 2, "catenaccio", "winger", "Direct Runner", "After beating a defender in the opponent half, his next shot is 30% more dangerous.", "Federico Chiesa", traitFx{afterDuelBoost: 1.3}, tw{}),
	outfield("dias", "Rúben Dias", 3, "catenaccio", "defender", "Organiser", "While he is on the pitch, the whole team defends 6% better.", "Rúben Dias", traitFx{teamDef: 1.06}, tw{}),
	outfield("griezmann", "Antoine Griezmann", 4, "catenaccio", "striker", "Tracks Back", "20% faster whenever his team does not have the ball, wins more duels up the pitch (+10), and pressure does not hurt his shots.", "Antoine Griezmann", traitFx{defSpeed: 1.2, highDuelDef: 10, ignorePressure: true}, tw{2: 10}),
	outfield("makelele", "Claude Makélélé", 3, "catenaccio", "anchor", "The Makélélé Role", "Opponent passes that travel near him fail 16% more often, and after winning the ball his next pass always arrives.", "Claude Makélélé", traitFx{laneBlock: 0.16, safeAfterTackle: true}, tw{}),
	outfield("chivu", "Cristian Chivu", 4, "catenaccio", "defender", "Captain's Tackle", "Strong in duels (+8). Moment: his next duel is won, guaranteed.", "Cristian Chivu", traitFx{active: "lock", duelDef: 8}, tw{2: 8}),
	outfield("vandijk", "Virgil van Dijk", 4, "catenaccio", "defender", "Colossus", "While he is on the pitch, every opponent shot is 12% less dangerous.", "Virgil van Dijk", traitFx{teamOppXG: 0.88}, tw{}),
	outfield("maldini", "Paolo Maldini", 5, "catenaccio", "defender", "Il Capitano", "Almost impossible to beat (+20 in duels), and every opponent shot is 8% less dangerous.", "Paolo Maldini", traitFx{duelDef: 20, teamOppXG: 0.92}, tw{0: 6}),
	outfield("baggio", "Roberto Baggio", 5, "catenaccio", "playmaker", "Il Divin Codino", "Very hard to tackle (+12); his direct free kicks are 50% more dangerous, and while his team is not ahead his shots are 30% better.", "Roberto Baggio", traitFx{duelAtk: 12, freeKick: 1.5, clutchShot: 1.3}, tw{1: 8}),

	// ---- Gegenpress: win it back high and at once ----
	outfield("nunez", "Darwin Núñez", 1, "press", "striker", "Chaos", "Runs 9% faster with the ball at his feet.", "Darwin Núñez", traitFx{ballSpeed: 1.09}, tw{0: 6}),
	outfield("endo", "Wataru Endo", 1, "press", "anchor", "Duel King", "Wins more duels in the opponent half (+10).", "Wataru Endo", traitFx{highDuelDef: 10}, tw{}),
	outfield("szoboszlai", "Dominik Szoboszlai", 2, "press", "playmaker", "Thunderbolt", "Shots from distance are 50% more dangerous.", "Dominik Szoboszlai", traitFx{longShot: 1.5}, tw{}),
	outfield("luisdiaz", "Luis Díaz", 2, "press", "winger", "Relentless", "Wins more duels in the opponent half (+12) and sprints for 1.5 seconds after a tackle.", "Luis Díaz (footballer, born 1997)", traitFx{highDuelDef: 12, afterTackleTurbo: 1.5}, tw{}),
	outfield("firmino", "Roberto Firmino", 2, "press", "striker", "False Nine", "Wins more duels in the opponent half (+10), and his passes make the receiver's shot 20% better.", "Roberto Firmino", traitFx{highDuelDef: 10, recvBoost: 1.2}, tw{2: 10}),
	outfield("rice", "Declan Rice", 3, "press", "anchor", "Ball Winner", "After winning a tackle he sprints for 2 seconds.", "Declan Rice", traitFx{afterTackleTurbo: 2}, tw{}),
	outfield("wirtz", "Florian Wirtz", 3, "press", "playmaker", "Between the Lines", "Hard to tackle (+6), and untouchable for a second after receiving the ball in the final third.", "Florian Wirtz", traitFx{recvShield: 1.0, duelAtk: 6}, tw{}),
	outfield("lautaro", "Lautaro Martínez", 3, "press", "striker", "Pressing Forward", "Wins far more duels in the opponent half (+18).", "Lautaro Martínez", traitFx{highDuelDef: 18}, tw{}),
	outfield("salah", "Mohamed Salah", 4, "press", "winger", "Inside Forward", "A wide angle does not reduce the quality of his shots.", "Mohamed Salah", traitFx{wideShot: true}, tw{1: 10}),
	outfield("dembele", "Ousmane Dembélé", 4, "press", "winger", "Two-Footed", "A wide angle does not hurt his shots; after beating a defender in the opponent half his next shot is 30% better.", "Ousmane Dembélé", traitFx{wideShot: true, afterDuelBoost: 1.3}, tw{}),
	outfield("matthaus", "Lothar Matthäus", 5, "press", "anchor", "Der Panzer", "Shots from distance are 60% more dangerous, he is strong in duels (+12) and tires 30% slower.", "Lothar Matthäus", traitFx{longShot: 1.6, duelDef: 12, drain: 0.7}, tw{1: 22, 0: 8}),
	outfield("gullit", "Ruud Gullit", 5, "press", "playmaker", "Black Tulip", "The team presses 10% faster; he is strong in duels (+10) and his close-range shots are 30% better.", "Ruud Gullit", traitFx{teamPress: 1.10, duelDef: 10, closeShot: 1.3}, tw{4: 10, 5: 16}),

	// ---- Street Football: beat your man ----
	outfield("man", "Dennis Man", 1, "street", "winger", "Cut Inside", "A wide angle does not reduce the quality of his shots.", "Dennis Man", traitFx{wideShot: true}, tw{}),
	outfield("kudus", "Mohammed Kudus", 1, "street", "playmaker", "Starboy", "After beating a defender in the opponent half, his next shot is 25% more dangerous.", "Mohammed Kudus", traitFx{afterDuelBoost: 1.25}, tw{}),
	outfield("leao", "Rafael Leão", 2, "street", "winger", "Open Field", "Runs 12% faster with the ball at his feet.", "Rafael Leão", traitFx{ballSpeed: 1.12}, tw{}),
	outfield("kvara", "Khvicha Kvaratskhelia", 2, "street", "winger", "Kvaradona", "Harder to tackle (+10), and a wide angle does not hurt his shots.", "Khvicha Kvaratskhelia", traitFx{duelAtk: 10, wideShot: true}, tw{}),
	outfield("mahrez", "Riyad Mahrez", 2, "street", "winger", "First Touch", "Cannot be tackled for a moment after receiving the ball in the final third; shots from distance are 20% better.", "Riyad Mahrez", traitFx{recvShield: 0.9, longShot: 1.2}, tw{}),
	outfield("musiala", "Jamal Musiala", 3, "street", "playmaker", "Slalom", "Much harder to tackle: +14 in every duel he attacks.", "Jamal Musiala", traitFx{duelAtk: 14}, tw{}),
	outfield("yamal", "Lamine Yamal", 3, "street", "winger", "Prodigy", "After beating a defender he is untouchable and sprinting for 1.2 seconds.", "Lamine Yamal", traitFx{afterDuelFree: 1.2}, tw{}),
	outfield("hazard", "Eden Hazard", 3, "street", "winger", "Low Centre", "Very hard to tackle (+14), and he is fouled 40% more often.", "Eden Hazard", traitFx{duelAtk: 14, foulDraw: 1.4}, tw{}),
	outfield("mutu", "Adrian Mutu", 4, "street", "striker", "Briliantul", "After beating a defender in the opponent half, his next shot is 60% more dangerous.", "Adrian Mutu", traitFx{afterDuelBoost: 1.6}, tw{3: 8}),
	outfield("neymar", "Neymar", 4, "street", "winger", "Joga Bonito", "Very hard to tackle (+16), and every pass he completes makes the receiver's shot 25% better.", "Neymar", traitFx{duelAtk: 16, recvBoost: 1.25}, tw{2: 10}),
	outfield("maradona", "Diego Maradona", 5, "street", "playmaker", "El Pibe de Oro", "After beating a defender in the opponent half his next shot is 50% better. Moment: untouchable for 3 seconds, and his next shot is 50% better.", "Diego Maradona", traitFx{active: "ghost", afterDuelBoost: 1.5}, tw{3: 8, 0: 6}),
	outfield("ronaldinho", "Ronaldinho", 5, "street", "playmaker", "Elástico", "Very hard to tackle (+14); shots from distance are 40% better. Moment: sprints 40% faster for 3 seconds.", "Ronaldinho", traitFx{duelAtk: 14, longShot: 1.4, active: "turbo"}, tw{3: 8}),

	// ---- Route One: the shortest way to goal, through a big number nine ----
	outfield("weghorst", "Wout Weghorst", 1, "route1", "striker", "Knock-Down", "Every pass he completes makes the receiver's next shot 15% better.", "Wout Weghorst", traitFx{recvBoost: 1.15}, tw{0: -8, 5: 8}),
	outfield("burn", "Dan Burn", 1, "route1", "defender", "Lighthouse", "Harder to dribble past: +10 in every duel he defends.", "Dan Burn", traitFx{duelDef: 10}, tw{0: -8, 5: 6}),
	outfield("koulibaly", "Kalidou Koulibaly", 2, "route1", "defender", "Long Clearance", "Strong in duels (+10), and distance does not hurt his passes.", "Kalidou Koulibaly", traitFx{duelDef: 10, longPass: true}, tw{}),
	outfield("osimhen", "Victor Osimhen", 2, "route1", "striker", "Masked Striker", "Wins more duels in the opponent half (+10), and close-range shots are 20% better.", "Victor Osimhen", traitFx{highDuelDef: 10, closeShot: 1.2}, tw{0: 6}),
	outfield("kane", "Harry Kane", 3, "route1", "striker", "Complete Forward", "Close-range shots are 25% better, and his passes boost the receiver's shot by 20%.", "Harry Kane", traitFx{closeShot: 1.25, recvBoost: 1.2}, tw{2: 12}),
	outfield("yaya", "Yaya Touré", 3, "route1", "anchor", "Freight Train", "Runs 12% faster with the ball, shrugs defenders off (+10), and shots from distance are 30% better.", "Yaya Touré", traitFx{ballSpeed: 1.12, duelAtk: 10, longShot: 1.3}, tw{1: 14, 5: 8}),
	outfield("haaland", "Erling Haaland", 4, "route1", "striker", "Terminator", "Shots from close range are 55% more dangerous.", "Erling Haaland", traitFx{closeShot: 1.55}, tw{0: 8, 5: 10}),
	outfield("ibra", "Zlatan Ibrahimović", 4, "route1", "striker", "Zlatan", "Shots from distance are 70% more dangerous, and he shrugs defenders off (+12).", "Zlatan Ibrahimović", traitFx{longShot: 1.7, duelAtk: 12}, tw{5: 12}),
	outfield("lewandowski", "Robert Lewandowski", 4, "route1", "striker", "Clinical", "Pressure does not hurt his shots, and close-range shots are 30% more dangerous.", "Robert Lewandowski", traitFx{ignorePressure: true, closeShot: 1.3}, tw{1: 6}),
	outfield("vidic", "Nemanja Vidić", 4, "route1", "defender", "Granite", "Very strong in duels (+14), players he tackles stay down twice as long, and his close-range shots are 30% better.", "Nemanja Vidić", traitFx{duelDef: 14, stunOnTackle: 2.0, closeShot: 1.3}, tw{1: 10}),
	outfield("ronaldo", "Cristiano Ronaldo", 5, "route1", "striker", "Big Game", "In the second half his shots are 40% more dangerous; close-range shots are 25% better all match.", "Cristiano Ronaldo", traitFx{secondHalfShot: 1.4, closeShot: 1.25}, tw{1: 6}),
	outfield("pele", "Pelé", 5, "route1", "striker", "O Rei", "Close-range shots are 45% better, long shots 30% better, and 20% more on top while his team is not ahead.", "Pelé", traitFx{closeShot: 1.45, longShot: 1.3, clutchShot: 1.2}, tw{3: 10, 0: 6}),

	// ---- Total Football: everybody can play everywhere ----
	outfield("timber", "Jurriën Timber", 1, "total", "defender", "Step In", "Runs 7% faster with the ball, and after winning the ball his next pass always arrives.", "Jurriën Timber", traitFx{ballSpeed: 1.07, safeAfterTackle: true}, tw{2: 6, 3: 8}),
	outfield("bernardo", "Bernardo Silva", 2, "total", "winger", "Tireless", "Tires 40% slower, and his own passes are 5% more accurate.", "Bernardo Silva", traitFx{drain: 0.6, passAcc: 0.05}, tw{2: 8}),
	outfield("alaba", "David Alaba", 2, "total", "defender", "Anywhere", "Distance does not hurt his passes, and his direct free kicks are 30% more dangerous.", "David Alaba", traitFx{longPass: true, freeKick: 1.3}, tw{1: 10, 2: 12}),
	outfield("gvardiol", "Joško Gvardiol", 2, "total", "defender", "Carrier", "Runs 11% faster with the ball; shots from distance are 30% better.", "Joško Gvardiol", traitFx{ballSpeed: 1.11, longShot: 1.3}, tw{1: 12, 3: 10}),
	outfield("dejong", "Frenkie de Jong", 3, "total", "anchor", "Press Resistant", "Very hard to tackle (+14) and runs 7% faster with the ball.", "Frenkie de Jong", traitFx{duelAtk: 14, ballSpeed: 1.07}, tw{3: 14}),
	outfield("kimmich", "Joshua Kimmich", 3, "total", "anchor", "Swiss Army Knife", "His passes are 8% more accurate, distance does not hurt them, and balls into the final third make the receiver's shot 20% better.", "Joshua Kimmich", traitFx{passAcc: 0.08, longPass: true, recvBoostFinalThird: 1.2}, tw{2: 10}),
	outfield("valverde", "Federico Valverde", 3, "total", "anchor", "Halcón", "Runs 12% faster with the ball, and his shots from distance are 40% more dangerous.", "Federico Valverde", traitFx{ballSpeed: 1.12, longShot: 1.4}, tw{0: 14, 1: 12}),
	outfield("bellingham", "Jude Bellingham", 4, "total", "playmaker", "Big Moments", "While his team is not ahead, his shots are 40% more dangerous.", "Jude Bellingham", traitFx{clutchShot: 1.4}, tw{4: 14, 5: 12}),
	outfield("modric", "Luka Modrić", 4, "total", "playmaker", "Metronome", "Teammates who receive his pass gain a large chunk of Moment; the team fills Moment 10% faster.", "Luka Modrić", traitFx{recvMoment: 22, teamMoment: 1.1}, tw{}),
	outfield("bergkamp", "Dennis Bergkamp", 4, "total", "striker", "Iceman", "Pressure does not hurt his shots; he cannot be tackled for a moment after receiving in the final third, and his passes make the receiver's shot 25% better.", "Dennis Bergkamp", traitFx{ignorePressure: true, recvShield: 1.0, recvBoost: 1.25}, tw{2: 14, 3: 8}),
	outfield("cruyff", "Johan Cruyff", 5, "total", "playmaker", "Total Football", "While he is on the pitch, the team presses 12% faster and tires 15% slower.", "Johan Cruyff", traitFx{teamPress: 1.12, teamDrain: 0.85}, tw{0: 8, 1: 6}),
	outfield("beckenbauer", "Franz Beckenbauer", 5, "total", "defender", "Der Kaiser", "The whole team defends 7% better; distance does not hurt his passes, and after winning the ball his pass always arrives.", "Franz Beckenbauer", traitFx{teamDef: 1.07, longPass: true, safeAfterTackle: true}, tw{2: 16, 3: 10}),

	// ---- Counter: win the ball and run ----
	outfield("pulisic", "Christian Pulisic", 1, "counter", "winger", "Captain America", "Runs 7% faster with the ball at his feet.", "Christian Pulisic", traitFx{ballSpeed: 1.07}, tw{}),
	outfield("werner", "Timo Werner", 1, "counter", "striker", "Channel Runner", "Runs 6% faster with the ball and drifts into the box when his team attacks.", "Timo Werner", traitFx{ballSpeed: 1.06, boxRunner: 6}, tw{0: 10, 1: -6}),
	outfield("vardy", "Jamie Vardy", 2, "counter", "striker", "On the Shoulder", "Runs 11% faster with the ball and drifts into the box when his team attacks.", "Jamie Vardy", traitFx{ballSpeed: 1.11, boxRunner: 8}, tw{0: 10}),
	outfield("hakimi", "Achraf Hakimi", 2, "counter", "defender", "Overlap", "15% faster whenever his team does not have the ball; after winning a tackle he sprints for 2.5 seconds.", "Achraf Hakimi", traitFx{defSpeed: 1.15, afterTackleTurbo: 2.5}, tw{0: 14, 3: 10}),
	outfield("davies", "Alphonso Davies", 2, "counter", "defender", "Roadrunner", "Runs 15% faster with the ball at his feet.", "Alphonso Davies", traitFx{ballSpeed: 1.15}, tw{0: 16, 3: 10}),
	outfield("son", "Son Heung-min", 3, "counter", "winger", "Both Feet", "A wide angle does not hurt his shots, and shots from distance are 35% more dangerous.", "Son Heung-min", traitFx{wideShot: true, longShot: 1.35}, tw{1: 12}),
	outfield("mane", "Sadio Mané", 3, "counter", "winger", "Lion of Teranga", "Wins more duels up the pitch (+10); after beating a defender in the opponent half his next shot is 35% better.", "Sadio Mané", traitFx{highDuelDef: 10, afterDuelBoost: 1.35}, tw{1: 8}),
	outfield("vini", "Vinícius Júnior", 4, "counter", "winger", "Samba", "After beating a defender he is untouchable and sprinting for 1.8 seconds.", "Vinícius Júnior", traitFx{afterDuelFree: 1.8}, tw{0: 6}),
	outfield("mbappe", "Kylian Mbappé", 4, "counter", "striker", "Afterburner", "Runs 18% faster with the ball at his feet.", "Kylian Mbappé", traitFx{ballSpeed: 1.18}, tw{0: 16, 3: 10}),
	outfield("bale", "Gareth Bale", 4, "counter", "winger", "Touchline Sprint", "After beating a defender he is untouchable and sprinting for 2 seconds; shots from distance are 40% better.", "Gareth Bale", traitFx{afterDuelFree: 2.0, longShot: 1.4}, tw{1: 10, 5: 10}),
	outfield("r9", "Ronaldo Nazário", 5, "counter", "striker", "O Fenômeno", "Shrugs defenders off (+16), runs 12% faster with the ball, and his close-range shots are 30% better.", "Ronaldo (Brazilian footballer)", traitFx{duelAtk: 16, ballSpeed: 1.12, closeShot: 1.3}, tw{0: 10, 3: 14}),
	outfield("henry", "Thierry Henry", 5, "counter", "striker", "Va-Va-Voom", "A wide angle does not hurt his shots, he runs 15% faster with the ball, and his passes make the receiver's shot 20% better.", "Thierry Henry", traitFx{wideShot: true, ballSpeed: 1.15, recvBoost: 1.2}, tw{0: 12}),

	// ---- Set Pieces: fouls, free kicks and penalties win matches ----
	outfield("wardprowse", "James Ward-Prowse", 1, "setpiece", "playmaker", "Dead Ball", "His direct free kicks are 60% more dangerous.", "James Ward-Prowse", traitFx{freeKick: 1.6}, tw{}),
	outfield("maguire", "Harry Maguire", 1, "setpiece", "defender", "Aerial Duel", "Harder to dribble past: +8 in every duel he defends.", "Harry Maguire", traitFx{duelDef: 8}, tw{0: -6}),
	outfield("taa", "Trent Alexander-Arnold", 2, "setpiece", "defender", "Quarterback", "Distance does not hurt his passes, balls into the final third boost the receiver's shot by 20%, and his direct free kicks are 30% more dangerous.", "Trent Alexander-Arnold", traitFx{longPass: true, recvBoostFinalThird: 1.2, freeKick: 1.3}, tw{2: 24, 4: -8}),
	outfield("casemiro", "Casemiro", 2, "setpiece", "anchor", "Enforcer", "Players he tackles stay down more than twice as long.", "Casemiro", traitFx{stunOnTackle: 2.2}, tw{}),
	outfield("eriksen", "Christian Eriksen", 2, "setpiece", "playmaker", "Delivery", "Direct free kicks are 30% more dangerous, and his passes make the receiver's shot 15% better.", "Christian Eriksen", traitFx{freeKick: 1.3, recvBoost: 1.15}, tw{}),
	outfield("bruno", "Bruno Fernandes", 3, "setpiece", "playmaker", "Risk Taker", "Every pass he completes makes the receiver's next shot 30% better, and his penalties are 8% more likely to score.", "Bruno Fernandes", traitFx{recvBoost: 1.3, penBonus: 0.08}, tw{}),
	outfield("beckham", "David Beckham", 4, "setpiece", "winger", "Bend It", "Direct free kicks are twice as dangerous; distance does not hurt his passes, and they make the receiver's shot 20% better.", "David Beckham", traitFx{freeKick: 2.0, longPass: true, recvBoost: 1.2}, tw{0: -8, 2: 14}),
	outfield("rcarlos", "Roberto Carlos", 4, "setpiece", "defender", "Banana Shot", "Shots from distance are 80% more dangerous, direct free kicks 70% more; runs 9% faster with the ball.", "Roberto Carlos", traitFx{longShot: 1.8, freeKick: 1.7, ballSpeed: 1.09}, tw{0: 12, 1: 26}),
	outfield("ramos", "Sergio Ramos", 4, "setpiece", "defender", "Last Minute", "Very strong in duels (+12), and his close-range shots are 30% more dangerous.", "Sergio Ramos", traitFx{duelDef: 12, closeShot: 1.3}, tw{1: 16}),
	outfield("pirlo", "Andrea Pirlo", 4, "setpiece", "playmaker", "Il Maestro", "Distance does not hurt his passes; direct free kicks are 60% more dangerous and his penalties 6% more likely to score.", "Andrea Pirlo", traitFx{longPass: true, freeKick: 1.6, penBonus: 0.06}, tw{2: 8}),
	outfield("ghagi", "Gheorghe Hagi", 5, "setpiece", "playmaker", "Regele", "Shots from distance are 90% more dangerous and his direct free kicks 60% more. Moment: his next pass cannot fail and boosts the receiver's shot.", "Gheorghe Hagi", traitFx{longShot: 1.9, freeKick: 1.6, active: "laser"}, tw{1: 8}),
	outfield("zidane", "Zinedine Zidane", 5, "setpiece", "playmaker", "Roulette", "Pressure does not hurt his shots, he is hard to tackle (+12), after beating a defender he is untouchable for 1.5 seconds, and his penalties are 8% more likely to score.", "Zinedine Zidane", traitFx{afterDuelFree: 1.5, ignorePressure: true, duelAtk: 12, penBonus: 0.08}, tw{5: 14}),

	// ---- goalkeepers ----
	keeper(AcademyKeeper, "Academy Keeper", 0, 46, "Youth Product", "Your starting keeper. Honest, nothing special.", "", traitFx{}),
	keeper("onana", "André Onana", 1, 54, "Good Feet", "His throws and kicks are never intercepted.", "André Onana", traitFx{gkSafe: true}),
	keeper("ederson", "Ederson", 3, 74, "Laser Feet", "Launches the ball straight to the most advanced free teammate, never intercepted.", "Ederson (footballer, born 1993)", traitFx{gkSafe: true, gkSweeper: true}),
	keeper("buffon", "Gianluigi Buffon", 5, 90, "Superman", "Every shot against him is 15% less dangerous, and one in seven certain goals is saved anyway.", "Gianluigi Buffon", traitFx{gkAll: 0.85, gkSecond: 0.15}),
	keeper("yashin", "Lev Yashin", 5, 91, "Black Spider", "Every shot against him is 14% less dangerous, and in a shootout every kick is 12% less likely to score.", "Lev Yashin", traitFx{gkAll: 0.86, gkPens: 0.12}),
	keeper("raya", "David Raya", 2, 68, "Claims Everything", "Shots from distance against him are 28% less dangerous.", "David Raya", traitFx{gkLong: 0.72}),
	keeper("kahn", "Oliver Kahn", 4, 84, "Der Titan", "Shots from close range against him are 22% less dangerous.", "Oliver Kahn", traitFx{gkClose: 0.78}),
	keeper("higuita", "René Higuita", 1, 53, "El Loco", "Launches the ball to the most advanced free teammate, and in a shootout every kick is 8% less likely to score.", "René Higuita", traitFx{gkSweeper: true, gkPens: 0.08}),
	keeper("donnarumma", "Gianluigi Donnarumma", 4, 83, "Second Chance", "One in five shots that would be a goal is clawed off the line.", "Gianluigi Donnarumma", traitFx{gkSecond: 0.2}),
	keeper("pickford", "Jordan Pickford", 1, 55, "Long Kick", "Launches the ball towards the most advanced free teammate.", "Jordan Pickford", traitFx{gkSweeper: true}),
	keeper("courtois", "Thibaut Courtois", 4, 84, "The Wall", "Every shot against him is 16% less dangerous.", "Thibaut Courtois", traitFx{gkAll: 0.84}),
	keeper("neuer", "Manuel Neuer", 3, 75, "Sweeper Keeper", "His distribution is never intercepted, and close-range shots against him are 15% less dangerous.", "Manuel Neuer", traitFx{gkSafe: true, gkClose: 0.85}),
	keeper("vandersar", "Edwin van der Sar", 4, 83, "Eleventh Outfielder", "His distribution is never intercepted, and shots from distance against him are 15% less dangerous.", "Edwin van der Sar", traitFx{gkSafe: true, gkLong: 0.85}),
	keeper("navas", "Keylor Navas", 2, 67, "Reflexes", "One in ten shots that would be a goal is clawed off the line.", "Keylor Navas", traitFx{gkSecond: 0.10}),
	keeper("alisson", "Alisson Becker", 3, 76, "One-on-One", "Shots from close range against him are 25% less dangerous.", "Alisson Becker", traitFx{gkClose: 0.75}),
	keeper("emi", "Emiliano Martínez", 2, 66, "Mind Games", "In a penalty shootout every opponent kick is 18% less likely to score.", "Emiliano Martínez", traitFx{gkPens: 0.18}),
	keeper("oblak", "Jan Oblak", 4, 85, "Shot Stopper", "Shots from distance against him are 30% less dangerous, and every shot 6% less.", "Jan Oblak", traitFx{gkLong: 0.7, gkAll: 0.94}),
}

// Players is the public catalogue; fxByID holds the mechanics, never sent to clients.
var (
	Players    []PlayerDef
	PlayerByID = map[string]*PlayerDef{}
	fxByID     = map[string]traitFx{}
)

// Keepers have a style of their own, so they count towards style combinations
// like everyone else.
var keeperStyle = map[string]string{
	"onana": "tiki", "ederson": "tiki",
	"buffon": "catenaccio", "yashin": "catenaccio",
	"raya": "press", "kahn": "press",
	"higuita": "street", "donnarumma": "street",
	"pickford": "route1", "courtois": "route1",
	"neuer": "total", "vandersar": "total",
	"navas": "counter", "alisson": "counter",
	"emi": "setpiece", "oblak": "setpiece",
}

func init() {
	Players = make([]PlayerDef, len(specs))
	for i, s := range specs {
		if st, ok := keeperStyle[s.ID]; ok {
			s.PlayerDef.Style = st
		}
		s.PlayerDef.Active = s.fx.active != ""
		Players[i] = s.PlayerDef
		PlayerByID[s.ID] = &Players[i]
		fxByID[s.ID] = s.fx
	}
}

func IsGK(id string) bool {
	d := PlayerByID[id]
	return d != nil && d.Role == "gk"
}

// StarMul scales a player's stats by star level (index = stars).
var StarMul = [4]float64{0, 1, 1.3, 1.65}

// Mods are the team-wide modifiers produced by synergies and aura traits.
type Mods struct {
	Stat       [7]float64
	PassAcc    float64 // added to pass success probability
	MomentMul  float64
	OppXG      float64 // multiplies the opponent's shot quality
	LongShot   float64 // multiplies xG for shots from distance
	PressSpeed float64
	Drain      float64 // stamina drain multiplier
	Counter    float64 // seconds midfield and attack sprint after the ball is won
	FoulDraw   float64 // multiplies how often the team's players are fouled
	PenBonus   float64 // added to the chance that the team's penalties score
	FreeKick   float64 // quality of a direct free kick near the box; 0 = the team plays it short
}

func baseMods() Mods {
	return Mods{Stat: [7]float64{1, 1, 1, 1, 1, 1, 1}, MomentMul: 1, OppXG: 1, LongShot: 1, PressSpeed: 1, Drain: 1, FoulDraw: 1}
}

type SynergyDef struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Kind  string   `json:"kind"` // "style" or "role"
	Tiers []int    `json:"tiers"`
	Desc  []string `json:"desc"`
	apply func(m *Mods, tier int)
}

// Styles have three steps (2, 4 and 6 players), positions two (2 and 4). The
// second step keeps the numbers the game had when two were all there was.
var Synergies = []SynergyDef{
	{"tiki", "Tiki-Taka", "style", []int{2, 4, 6}, []string{"Passing +8% accuracy, Dribbling +5%, Moment +15%", "Passing +18% accuracy, Dribbling +10%, Moment +35%", "Passing +28% accuracy, Dribbling +16%, Moment +60%"},
		func(m *Mods, t int) {
			m.PassAcc += []float64{0.08, 0.18, 0.28}[t]
			m.Stat[SDri] *= []float64{1.05, 1.10, 1.16}[t]
			m.MomentMul *= []float64{1.15, 1.35, 1.60}[t]
		}},
	{"catenaccio", "Catenaccio", "style", []int{2, 4, 6}, []string{"Defending +12%, opponent chances -10%", "Defending +28%, opponent chances -24%", "Defending +45%, opponent chances -38%"},
		func(m *Mods, t int) {
			m.Stat[SDef] *= []float64{1.12, 1.28, 1.45}[t]
			m.OppXG *= []float64{0.90, 0.76, 0.62}[t]
		}},
	{"press", "Gegenpress", "style", []int{2, 4, 6}, []string{"Pressing speed +10%, fatigue -15%", "Pressing speed +22%, fatigue -30%", "Pressing speed +35%, fatigue -45%"},
		func(m *Mods, t int) {
			m.PressSpeed *= []float64{1.10, 1.22, 1.35}[t]
			m.Drain *= []float64{0.85, 0.70, 0.55}[t]
		}},
	{"street", "Street Football", "style", []int{2, 4, 6}, []string{"Dribbling +14%", "Dribbling +30%, Pace +6%", "Dribbling +50%, Pace +14%"},
		func(m *Mods, t int) {
			m.Stat[SDri] *= []float64{1.14, 1.30, 1.50}[t]
			m.Stat[SPac] *= []float64{1, 1.06, 1.14}[t]
		}},
	{"route1", "Route One", "style", []int{2, 4, 6}, []string{"Physical +12%, Shooting +8%, long shots +20%", "Physical +25%, Shooting +15%, long shots +45%", "Physical +40%, Shooting +25%, long shots +75%"},
		func(m *Mods, t int) {
			m.Stat[SPhy] *= []float64{1.12, 1.25, 1.40}[t]
			m.Stat[SSho] *= []float64{1.08, 1.15, 1.25}[t]
			m.LongShot *= []float64{1.20, 1.45, 1.75}[t]
		}},
	{"total", "Total Football", "style", []int{2, 4, 6}, []string{"Every stat +2%", "Every stat +4%", "Every stat +6%"},
		func(m *Mods, t int) {
			for k := range m.Stat {
				m.Stat[k] *= []float64{1.02, 1.04, 1.06}[t]
			}
		}},
	{"counter", "Counter", "style", []int{2, 4, 6}, []string{"After winning the ball, midfield and attack sprint for 0.6 s", "After winning the ball, midfield and attack sprint for 1.1 s", "After winning the ball, midfield and attack sprint for 1.6 s"},
		func(m *Mods, t int) { m.Counter += []float64{0.6, 1.1, 1.6}[t] }},
	{"setpiece", "Set Pieces", "style", []int{2, 4, 6}, []string{
		"Fouled 30% more often, penalties +5%, free kicks near the box become shots",
		"Fouled 60% more often, penalties +10%, better free kicks",
		"Fouled twice as often, penalties +15%, dangerous free kicks"},
		func(m *Mods, t int) {
			m.FoulDraw *= []float64{1.3, 1.6, 2.0}[t]
			m.PenBonus += []float64{0.05, 0.10, 0.15}[t]
			m.FreeKick = []float64{0.07, 0.11, 0.16}[t]
		}},

	{"striker", "Strike Partnership", "role", []int{2, 4}, []string{"Shooting +18%", "Shooting +42%"},
		func(m *Mods, t int) { m.Stat[SSho] *= []float64{1.18, 1.42}[t] }},
	{"winger", "Wide Men", "role", []int{2, 4}, []string{"Pace +8%", "Pace +18%"},
		func(m *Mods, t int) { m.Stat[SPac] *= []float64{1.08, 1.18}[t] }},
	{"playmaker", "Brain Trust", "role", []int{2, 4}, []string{"Passing +15%, Moment +20%", "Passing +36%, Moment +45%"},
		func(m *Mods, t int) {
			m.Stat[SPas] *= []float64{1.15, 1.36}[t]
			m.MomentMul *= []float64{1.20, 1.45}[t]
		}},
	{"anchor", "Double Pivot", "role", []int{2, 4}, []string{"Defending +15%", "Defending +32%"},
		func(m *Mods, t int) { m.Stat[SDef] *= []float64{1.15, 1.32}[t] }},
	{"defender", "Back Line", "role", []int{2, 4}, []string{"Defending +10%, Physical +10%", "Defending +22%, Physical +22%"},
		func(m *Mods, t int) {
			m.Stat[SDef] *= []float64{1.10, 1.22}[t]
			m.Stat[SPhy] *= []float64{1.10, 1.22}[t]
		}},
}

type SynergyView struct {
	ID     string `json:"id"`
	Count  int    `json:"count"`
	Active int    `json:"active"` // index of the active tier, -1 when none
}

// ComputeSynergies counts unique players per style and position. The
// goalkeeper counts for his style; there is no keeper position bonus.
func ComputeSynergies(lu Lineup) (Mods, []SynergyView) { return computeSynergies(lu, "") }

func computeSynergies(lu Lineup, without string) (Mods, []SynergyView) {
	seen := map[string]bool{}
	count := map[string]int{}
	add := func(id string) {
		d := PlayerByID[id]
		if d == nil || seen[id] {
			return
		}
		seen[id] = true
		if d.Style != "" {
			count[d.Style]++
		}
		if d.Role != "gk" {
			count[d.Role]++
		}
	}
	for _, u := range lu.Units {
		add(u.DefID)
	}
	add(lu.GK.DefID)
	m := baseMods()
	views := []SynergyView{}
	for _, s := range Synergies {
		c := count[s.ID]
		if c == 0 {
			continue
		}
		active := -1
		for i, need := range s.Tiers {
			if c >= need {
				active = i
			}
		}
		if active >= 0 && s.ID != without {
			s.apply(&m, active)
		}
		views = append(views, SynergyView{ID: s.ID, Count: c, Active: active})
	}
	return m, views
}

// Tactic is a team order. The first half is always "balanced"; the manager
// picks the second-half order at the break.
type Tactic struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Desc       string  `json:"desc"`
	Keeper     string  `json:"keeper"` // what the goalkeeper does under this order
	Attack     float64 `json:"-"`      // multiplies the quality of this team's shots
	Concede    float64 `json:"-"`      // multiplies the quality of shots against this team
	Push       float64 `json:"-"`      // how far up the pitch the shape sits
	ShootBias  float64 `json:"-"`
	LongShot   float64 `json:"-"` // multiplies this team's shots from distance (0 = unchanged)
	Pressers   int     `json:"-"`
	Drain      float64 `json:"-"`
	DefMul     float64 `json:"-"`
	PassAcc    float64 `json:"-"`
	MomentMul  float64 `json:"-"`
	PressSpeed float64 `json:"-"`
	PassBias   float64 `json:"-"`
	Forward    float64 `json:"-"` // extra weight on passing forward
	Counter    float64 `json:"-"` // seconds the forwards sprint after the ball is won

	// The goalkeeper's part in the order.
	GKHold      float64 `json:"-"` // seconds he keeps the ball before releasing it
	GKIntercept float64 `json:"-"` // chance his distribution is cut out
	GKLong      bool    `json:"-"` // he launches it to the most advanced free teammate
	GKOffLine   float64 `json:"-"` // multiplies long shots against him (he stands off his line); 0 = unchanged
}

// Each order is a trade, not an upgrade (see tactic_test.go for the measured
// effect): All-Out Attack and Park the Bus move both teams' chances up or
// down together, which helps the side that is behind or ahead; the others
// swap one strength for one weakness. Every order also tells the keeper what
// to do with the ball and where to stand.
var Tactics = []Tactic{
	{ID: "balanced", Name: "Balanced", Desc: "Play the same game. No risks.", Keeper: "Normal distribution.",
		Attack: 1, Concede: 1, Pressers: 2, Drain: 1, DefMul: 1, MomentMul: 1, PressSpeed: 1, GKHold: 1, GKIntercept: 0.08},
	{ID: "all_out", Name: "All-Out Attack", Desc: "Chasing the game: far more chances for you, and for them. Best when you are behind.", Keeper: "Releases the ball at once, launches it long and stands off his line: long shots against him are 30% more dangerous.",
		Attack: 2.2, Concede: 1.5, Push: 0.65, ShootBias: 0.2, Pressers: 2, Drain: 1.15, DefMul: 0.88, Forward: 0.06, MomentMul: 1, PressSpeed: 1, GKHold: 0.5, GKIntercept: 0.1, GKLong: true, GKOffLine: 1.3},
	{ID: "park_bus", Name: "Park the Bus", Desc: "Protecting a lead: far fewer chances for them, and for you. Best when you are ahead.", Keeper: "Wastes time: holds the ball more than twice as long, and his throws are almost never cut out.",
		Attack: 0.5, Concede: 0.5, Push: -0.4, ShootBias: -0.06, Pressers: 2, Drain: 0.9, DefMul: 1.2, MomentMul: 1, PressSpeed: 1, GKHold: 2.4, GKIntercept: 0.02},
	{ID: "press", Name: "Full Press", Desc: "Three men hunt the ball: you win it more often, but your team tires twice as fast and gives up better chances.", Keeper: "Plays it short and quickly to keep the tempo: his passes are cut out twice as often.",
		Attack: 1, Concede: 1.38, Push: 0.15, Pressers: 3, Drain: 2.0, DefMul: 1.1, MomentMul: 1, PressSpeed: 1.15, GKHold: 0.7, GKIntercept: 0.16},
	{ID: "keep_ball", Name: "Keep the Ball", Desc: "Safe passes and long possession: Moment fills faster, but you shoot much less and from worse positions.", Keeper: "Takes his time and always finds a free man: his distribution is never cut out.",
		Attack: 0.6, Concede: 1.0, ShootBias: -0.14, Pressers: 2, Drain: 0.9, DefMul: 1, PassAcc: 0.12, MomentMul: 1.35, PressSpeed: 1, PassBias: 0.18, GKHold: 1.3, GKIntercept: 0},
	{ID: "counter", Name: "Counter-Attack", Desc: "Sit deep with one man pressing. When you win the ball, midfield and attack sprint for a second and a half and the next chances are better. You will see little of the ball.", Keeper: "Launches it long the moment he has it.",
		Attack: 1.0, Concede: 1.0, Push: -0.3, Pressers: 1, Drain: 0.95, DefMul: 1.05, MomentMul: 1, PressSpeed: 1, Forward: 0.05, Counter: 1.4, GKHold: 0.5, GKIntercept: 0.12, GKLong: true},
	{ID: "long_balls", Name: "Long Balls", Desc: "Skip the midfield: every pass looks forward. You reach their box faster, but passes are 8% less accurate.", Keeper: "Goal kicks go straight to the most advanced free teammate.",
		Attack: 1.28, Concede: 1.05, Push: 0.1, Pressers: 2, Drain: 1, DefMul: 1, PassAcc: -0.08, MomentMul: 1, PressSpeed: 1, Forward: 0.08, GKHold: 0.8, GKIntercept: 0.12, GKLong: true},
	{ID: "shoot_sight", Name: "Shoot on Sight", Desc: "Let fly from anywhere: many more shots and long shots are 70% better, but shooting early wastes good positions.", Keeper: "Normal distribution.",
		Attack: 1.15, Concede: 1, ShootBias: 0.2, LongShot: 1.7, Pressers: 2, Drain: 1, DefMul: 1, MomentMul: 1, PressSpeed: 1, GKHold: 1, GKIntercept: 0.08},
}

var TacticByID = func() map[string]Tactic {
	m := map[string]Tactic{}
	for _, t := range Tactics {
		m[t.ID] = t
	}
	return m
}()
