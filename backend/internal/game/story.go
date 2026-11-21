package game

// The solo season tells a story: a manager hired by a tiny club climbs from
// the car park to the top flight in eight fixtures. Before fixtures 3 and 6
// something happens at the club and the manager decides how to answer; each
// answer has a real effect on the season. Some answers are promises whose
// outcome is only known after the next match.

// StoryRounds are the fixtures whose transfer window opens with a club event.
// They avoid the augment rounds (2, 5, 8) so a window never asks two questions.
var StoryRounds = map[int]bool{3: true, 6: true}

// StoryChoice is one answer to a club event.
type StoryChoice struct {
	Label  string `json:"label"`
	Effect string `json:"effect"`
	Cost   int    `json:"cost,omitempty"` // budget it takes; the answer is refused without it
}

// StoryEvent is something that happens at the club between two fixtures.
type StoryEvent struct {
	ID      string        `json:"id"`
	Title   string        `json:"title"`
	Text    string        `json:"text"`
	Choices []StoryChoice `json:"choices"`
}

// Promise outcomes, checked after the next match.
const (
	pledgeWin      = "win"     // promised a win
	pledgeBigWin   = "big_win" // promised the fans a win by two goals or more
	pledgeClean    = "clean"   // owner's bonus for a clean sheet
	pledgeWinBonus = 3         // patience a kept "win" promise gives back
	pledgeBigBonus = 4         // patience a kept "big win" promise gives back
	pledgeBroken   = 2         // extra patience a broken promise costs
	cleanBonus     = 5         // budget for a clean sheet when the bonus was accepted
	campCost       = 3         // training camp price
	campXP         = 8         // XP the camp gives
	sponsorCash    = 5         // budget the shirt sponsor pays at once
	scoutXP        = 4         // XP from scouting the youth cup
	homeXP         = 2         // XP from training at home
)

var StoryEvents = []StoryEvent{
	{ID: "sponsor", Title: "A sponsor at the door",
		Text: "The kebab shop next to the ground wants its logo on your shirts. The money is real; the owner thinks it looks cheap.",
		Choices: []StoryChoice{
			{Label: "Take the money", Effect: "+5 💰 now, owner's patience −1"},
			{Label: "Keep the shirt clean", Effect: "Owner's patience +2"},
		}},
	{ID: "press", Title: "Press conference",
		Text: "A reporter asks whether you will win on Saturday. Every camera in the room turns to you.",
		Choices: []StoryChoice{
			{Label: "Promise a win", Effect: "Win the next match: patience +3. Lose it: the defeat costs 2 more"},
			{Label: "One game at a time", Effect: "+1 💰 from a quiet week"},
		}},
	{ID: "youth", Title: "The youth cup",
		Text: "The academy plays a regional youth tournament this weekend. You can watch it with a notebook or let the kids enjoy it.",
		Choices: []StoryChoice{
			{Label: "Send the scouts", Effect: "+4 stadium XP"},
			{Label: "Let the kids enjoy it", Effect: "Owner's patience +2: the town loves it"},
		}},
	{ID: "camp", Title: "Training camp",
		Text: "A camp in the mountains has one week free. It is not cheap, but the players would come back sharper.",
		Choices: []StoryChoice{
			{Label: "Book the camp", Effect: "−3 💰, +8 stadium XP", Cost: campCost},
			{Label: "Train at home", Effect: "+2 stadium XP"},
		}},
	{ID: "derby", Title: "Derby week",
		Text: "The fans want more than a win this time: they want to humiliate the neighbours.",
		Choices: []StoryChoice{
			{Label: "Promise a big win", Effect: "Win by 2+: patience +4. Lose: the defeat costs 2 more"},
			{Label: "Calm everyone down", Effect: "Nothing changes"},
		}},
	{ID: "owner", Title: "The owner's bet",
		Text: "The owner offers a bonus if your team keeps a clean sheet next match. Or you can ask for something smaller, now.",
		Choices: []StoryChoice{
			{Label: "Accept the bet", Effect: "Clean sheet next match: +5 💰"},
			{Label: "Ask for it up front", Effect: "+2 💰 now"},
		}},
	{ID: "agent", Title: "An agent calls",
		Text: "A well-connected agent offers to show you some names nobody else has seen yet.",
		Choices: []StoryChoice{
			{Label: "Listen to him", Effect: "A fresh market now, free"},
			{Label: "Hang up", Effect: "+1 💰"},
		}},
}

var StoryEventByID = func() map[string]StoryEvent {
	m := map[string]StoryEvent{}
	for _, e := range StoryEvents {
		m[e.ID] = e
	}
	return m
}()

// offerStory picks this window's club event, never one already seen this season.
func (r *Run) offerStory(g *RNG) {
	r.Event = ""
	if !StoryRounds[r.Round] {
		return
	}
	seen := map[string]bool{}
	for _, id := range r.EventsSeen {
		seen[id] = true
	}
	var pool []string
	for _, e := range StoryEvents {
		if !seen[e.ID] {
			pool = append(pool, e.ID)
		}
	}
	if len(pool) == 0 {
		return
	}
	r.Event = pool[g.Intn(len(pool))]
	r.EventsSeen = append(r.EventsSeen, r.Event)
}

// chooseStory answers the pending club event.
func (r *Run) chooseStory(choice int) error {
	ev, ok := StoryEventByID[r.Event]
	if !ok {
		return rule("There is nothing to answer.")
	}
	if choice < 0 || choice >= len(ev.Choices) {
		return rule("Invalid choice.")
	}
	if c := ev.Choices[choice]; r.Budget < c.Cost {
		return rule("Not enough budget.")
	}
	switch ev.ID + "/" + string(rune('0'+choice)) {
	case "sponsor/0":
		r.Budget += sponsorCash
		r.Patience = max(1, r.Patience-1)
	case "sponsor/1", "youth/1":
		r.Patience = min(StartPatience, r.Patience+2)
	case "press/0":
		r.Pledge = pledgeWin
	case "press/1", "agent/1":
		r.Budget++
	case "youth/0":
		r.gainXP(scoutXP)
	case "camp/0":
		r.Budget -= campCost
		r.gainXP(campXP)
	case "camp/1":
		r.gainXP(homeXP)
	case "derby/0":
		r.Pledge = pledgeBigWin
	case "derby/1":
	case "owner/0":
		r.Pledge = pledgeClean
	case "owner/1":
		r.Budget += 2
	case "agent/0":
		r.Locked = false
		r.withRNG(r.rollShop)
	}
	r.Event = ""
	r.EventChoice = append(r.EventChoice, choice)
	return nil
}

// settlePledge applies a promise made before this match and says what came of it.
// It returns the extra patience a broken promise costs and a line for the history.
func (r *Run) settlePledge(won, draw bool, gf, ga int) (extraDamage int, note string) {
	p := r.Pledge
	r.Pledge = ""
	switch p {
	case pledgeWin:
		if won {
			r.Patience = min(StartPatience, r.Patience+pledgeWinBonus)
			return 0, "Promise kept: owner's patience +3"
		}
		if draw {
			return 0, "Only a draw: the promise was not kept, but no harm done"
		}
		return pledgeBroken, "Promise broken: the defeat cost 2 more"
	case pledgeBigWin:
		if won && gf-ga >= 2 {
			r.Patience = min(StartPatience, r.Patience+pledgeBigBonus)
			return 0, "The fans got their big win: patience +4"
		}
		if draw {
			return 0, "A draw: not what the fans were promised, but no harm done"
		}
		if !won {
			return pledgeBroken, "Derby promise broken: the defeat cost 2 more"
		}
		return 0, "A win, but not the thrashing the fans wanted"
	case pledgeClean:
		if ga == 0 {
			r.Budget += cleanBonus
			return 0, "Clean sheet: the owner pays +5 💰"
		}
		return 0, "No clean sheet: no bonus"
	}
	return 0, ""
}

// StoryView is the pending club event as the client sees it.
func (r *Run) storyView() *StoryEvent {
	ev, ok := StoryEventByID[r.Event]
	if !ok {
		return nil
	}
	return &ev
}
