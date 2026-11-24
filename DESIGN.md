# Kickoff Tactics — design

Football auto-battler, server-authoritative. The game UI is in English.

Starting point was a claude.ai conversation from 2026-11-18. The author asked that nothing from it be
taken as-is, so the design below is rethought. Items marked **[in demo]** exist in code; the rest are ideas.

## Author decisions

- Important logic runs on the server; the frontend is visuals only.
- Backend in Go, security-conscious. Frontend in React + Vite.
- MongoDB (Atlas) as the database. No Supabase.
- Game language: English.
- Real footballer names, with a representative photo on cards and on the pitch.
- Traits are unique per player, not per position.
- You always have a goalkeeper: an academy keeper by default, never sold out of goal, only swapped.
- Market cards for players you already own show a different background, not a text label.
- The player's face is the background of his card, on the pitch and in the market.
- Stadium level N means N outfield players plus the keeper (level 1 = one outfield player).
- Pool: not mostly Romanians; current top players plus legends, balanced per style; cost by reputation,
  cost 5 hard to get; Chivu and Mutu cost 4, Gheorghe Hagi cost 5.
- Live play with timers, 1× speed only, everyone at the same point, state kept outside the client.
- Home page, accounts, friends, a way to talk to and invite another player.
- Name: "Kickoff Tactics", chosen by the author (2026-11-19; earlier working name "Auto Football").

## What makes it different from a classic auto-battler

1. **Half-time team talk [in demo].** Two halves. After the first you see score and stats and pick an
   order for the second: Balanced, All-Out Attack, Park the Bus, Full Press, Keep the Ball.
2. **Scouting [in demo].** You see the opponent's line-up and synergies before kick-off, on the same board.
3. **Position is role [in demo].** 3 lines (defence, midfield, attack) × 5 lanes, plus the goal.
   The line decides how far a player pushes up, the lane where he stays. A player on the defence
   line with a team-mate ahead of him holds: he follows an attack only to about the halfway line,
   passes the ball on instead of carrying it past there, and does not join the press when the ball
   is far up the pitch. Alone at the back, or under All-Out Attack, he plays the whole pitch.
   Midfielders pass a little more often; attackers stay highest. There are no per-player
   instructions: placement is the instruction.
4. **Luck in the open [in demo].** Every shot shows its xG; possession, shots and xG at the end.
5. **Verifiable match [in demo].** The server shows the SHA-256 of the seed before you choose your
   order and reveals the seed afterwards.

## Game loop [in demo]

A league of ten clubs over nine matchdays (2026-11-21): you and nine server-run clubs of rising
strength, everyone meeting everyone once (`game/league.go`). Win 3 points, draw 1: a solo match
level at full time is a draw, with no shootout (`SimulateLeague`); a draw costs no patience and
ends any streak. Your matches are simulated; the rivals' matches against each other are told, not
played: a Poisson scoreline from each club's strength (rival k = k plus a little form drawn per
season, `leagueEdge` 0.09 goals per point of strength), so the table moves every matchday at no
cost. You meet the rivals weakest first. The table (sidebar, after every match with the other
results, and at the end) marks the champion, Europe (2-4) and relegation (9-10). The season ends
by position: 1 champion, 2-4 European place, 5-8 mid-table, 9-10 relegated; sacked as before when
patience runs out. Seasons from before the league are treated as absent and start again.

**After the league** (2026-11-24, `game/cup.go`), with the same squad, budget and stadium:

- **Europe** for the top four: 1st the European Crown (★★★), 2nd the European Shield (★★), 3rd-4th
  the European Plate (★). Each cup draws 15 opponents at random from its own pool of invented clubs
  in real cities, shown with the country in brackets ("Racing Sevilla (Spain)"). The Crown pool is
  the ten strongest football countries; the Shield has a few of those and many mid-level ones
  (Poland, Romania, Hungary, Greece, Austria...); the Plate the smallest countries (Moldova,
  Andorra, San Marino, Gibraltar...) plus two clubs from big ones. Each club has a rating (0-10)
  that sets its line-up (`ratedTeam`) and its told results. Four groups of four over three
  matchdays, then quarter-finals (A1-B2, B1-A2, C1-D2, D1-C2), semi-finals and a final, penalties
  if level. Only your matches are played; every other group game and tie is told the same night,
  so all groups and the bracket are visible and move as you go ("All groups and the knockout
  bracket" under your group; the draw on the Chapter 2 screen; the whole cup in the epilogue,
  told to the end so it names the winner). Europe costs no patience: a group finish outside the
  top two, or a knockout defeat, ends the run.
- **Play-off** for the bottom two (his request: a story for relegation): a mini group with three
  of six second-division clubs (opponents as strong as league fixtures 6-8). Patience drops to 1,
  so any defeat is the sack; only first place goes back up (`promoted`), otherwise `stayed`.

History keeps the outcome (`GameRecord.Cup` = `<cup>:<group|quarter|semi|final|winner|promoted|stayed|sacked>`).
League achievements (titles, Invincibles, Perfect Season, Survivor) are counted on the last league
matchday; the run record (W/D/L) counts the later matches too. Bot play (1500 seasons): Plate won
12%, Shield 6%, Crown 1-3% (out in the group 46 / 57 / 76%); play-off promoted 21-34%, sacked 55-60%.

- **Market**: 5 players on offer, refreshed free each fixture; a new set costs 2. **Lock** keeps the
  current set for the next transfer window, once (as in TFT); a paid refresh unlocks it. The budget
  is shown next to the market buttons.
- **Market during the match**: the line-up that plays is frozen at kick-off, so buying, selling,
  refreshing, locking and levelling stay open while the match is on and count from the next match.
  Live games: a drawer under the pitch, open from kick-off to the next window. Solo: open during the
  first half and the break only (after the break the season has already moved to the next fixture,
  whose market and budget would give the result away). A player sent off is found where he played
  or, if moved meanwhile, by name.
- **Augments** (rounds 2, 5 and 8, every mode): choose one of three for the rest of the season; if
  none is chosen when the window closes, the first is taken (bots always take the first). At most
  one crest per offer. Economy: Transfer Windfall (+10 now), Shrewd Investor (interest up to +7),
  Shirt Sponsor (+2 per match), Scouting Network (first refresh of each window free), Stadium
  Expansion (+8 XP now), Trusted by the Board (defeats cost 10% less patience, at least 1), Squad
  Rotation (one more outfield player). Team: Fitness Coach (drain -40%), Home Crowd (all stats +3%),
  Finishing School (shooting +18%), Defensive Coach (opponent chances -14%), Penalty Specialist
  (+10% to score penalties and shootout kicks). Crests: one per style, count as one more player of
  that style. Team effects and crests travel in the frozen line-up (`Lineup.Augments`), so a replay
  stays deterministic. Not balance-measured yet.
- **Budget**: 5 per fixture + interest (1 per 10 held when the match ends, max 5 at 50, as in TFT)
  + 1 for a win + streak bonus (+1 at 2 in a row, +2 at 4; wins or defeats). The transfer window
  shows the interest currently earned, how much more to bank for the next step, and the full rule.
- **Stadium** (level 1-10): one outfield player per level, plus the keeper, and better players on the market.
  Level 10 is a full eleven. 2 free XP per fixture; 4 XP cost 4. XP per level: 2, 2, 4, 6, 8, 10, 14, 18, 24.
  Squad Rotation adds one outfield player, but never past ten.
- **Opponents** field 1, 1, 2, 2, 3, 4, 4, 5, 6 outfield players across the nine matchdays, of cost
  up to 1, 1, 1, 2, 2, 3, 3, 4, 4, with a two-star chance of 0-60% (`soloCount`, `soloMaxCost`,
  `soloStar2` in `game/run.go`).
- **Story** (solo only): the season is told as a story. An opening chapter before fixture 1, one
  line about every rival, the owner's reaction after each match and an epilogue per ending
  (`frontend/src/story.ts`, words only). Before fixtures 3 and 6 a **club event** asks a question
  with two answers that change the season (`game/story.go`): a shirt sponsor (+5 budget and -1
  patience, or +2 patience), a press conference (promise a win: +3 patience if kept, the defeat
  costs 2 more if not), the youth cup (+4 XP or +2 patience), a training camp (-3 budget for +8 XP,
  or +2 XP), derby week (promise a win by two: +4 patience), the owner's bet (+5 budget for a clean
  sheet, or +2 now), an agent (a free fresh market, or +1 budget). Never the same event twice in a
  season; an event left unanswered at kick-off takes its first answer (the second if the first is
  not affordable). Augment rounds are 2, 5 and 8, so a window never asks both.
- **Merge**: three identical players become one with an extra ★ (stats ×1.3, then ×1.65); three ★★
  make a ★★★. The market shows the stars you already own of a player and marks a card that would
  complete a merge.
- **Owner's patience**: 20. A defeat costs 1 + goal difference (counted up to 3) + fixture/3. At zero
  you are sacked. The transfer window shows what the next defeat would cost.
- **End**: 6+ wins champion, 4-5 European place, otherwise you survived.

## Goalkeeper [in demo]

- Every club starts with an **Academy Keeper** in goal (reflexes 46, not on the market, sells for 0).
- Keepers are bought on the market like anyone else and merge to ★★/★★★ (+8 reflexes per star).
- The goal is never empty: the keeper in goal cannot be sold and cannot be moved out, except by
  dropping another keeper onto the goal, which swaps them. A benched keeper can be sold.
- Keepers only play in goal; outfield players cannot stand in goal.

## The match [in demo]

Continuous simulation on a 100 × 60 pitch, 0.1 s step, two halves of 30 s. The server computes the
whole match instantly and sends position frames plus events; the client only replays them.

- Duels (dribbling vs defending), passes (accuracy, distance, opponents near the lane), shots (xG from
  distance, angle, shooting, pressure, keeper reflexes), fatigue, penalty shootout on a draw.
- **Fouls and cards**: any duel can end in a foul. It is most likely when the defender is beaten
  and brings the attacker down (11%; 20% inside the box), less when he wins the ball (5%; 8% in
  the box). A side on Full Press fouls 30% more. A foul gives a free kick
  (the fouled player keeps the ball, untouchable for a moment) or, in the box, a penalty taken by
  the team's best shooter. The offender may be booked: yellow 34% (45% in the box), straight red 3%
  (6%). A second yellow is a red. Measured per match with five-a-side squads: 1.2 fouls, 0.4
  yellows, a sending-off about once in 18 matches, 0.14 penalties.
- **Sent off**: the player leaves the pitch for the rest of the match and is then **suspended**:
  one match for two yellows; for a straight red one match (65%), two (27%) or three (8%), by luck.
  A suspended player is shown greyed out with the matches left; he may stay where he is or sit on
  the bench, but he does not play and cannot be brought on until the ban is served. If nobody in the
  squad is eligible, the keeper plays alone, so a season never gets stuck. Same rules in a live series.
- **Shootout**: a draw is settled by three kicks each, then sudden death, played out on the pitch and
  recorded as frames (17 per kick), so it is watched like the match. In a live series the shootout is
  an extra phase on the shared clock, and the standings stay hidden until the last kick.
- **Half-time orders** are trades, measured in `tactic_test.go`: All-Out Attack raises both teams'
  chances (best when behind), Park the Bus lowers both (best when ahead), Full Press wins the ball
  more but tires the team and concedes better chances, Keep the Ball trades shots for possession.
  Both sides always choose: the server's club by the half-time score, a friend in secret.
  Three more orders: Counter-Attack (sit deep, sprint when the ball is won), Long Balls (every pass
  looks forward, less accurate), Shoot on Sight (many more shots, long shots 70% better).
  Measured on 2026-11-24, Long Balls and Shoot on Sight were never the best choice, so each became
  an answer to another order (`TestOrderCounters`, matrix in `matrix_test.go`): Long Balls gets
  better chances against a team that pushes up (`Behind` x its Push: All-Out, Full Press), Shoot on
  Sight's long shots ignore a deep defence and get better against it (`Edge` x how deep it sits:
  Park the Bus, Counter-Attack). Trailing against Park the Bus, Shoot on Sight wins 15.5% against
  11.1% for All-Out; level against All-Out, Long Balls wins 57%, the best of all.
  **Every order also instructs the goalkeeper**: All-Out makes him launch it at once and stand off
  his line (long shots against +30%), Park the Bus makes him waste time (holds the ball 2.4 s, almost
  never intercepted), Full Press has him play short and fast (cut out twice as often), Keep the Ball
  has him always find a free man, Counter-Attack and Long Balls make him launch it long.
- **Duels**: keeping the ball depends on Dribbling (and a little Pace), not strength, so defenders
  rarely dribble past a challenge (38%) while attackers usually do (70-78%); poor dribblers pass
  under pressure. **Interceptions** need a defender within 5 units of the passing line; otherwise a
  bad pass is simply misplaced and rolls loose to the nearest player.
- **Clean through**: a player on the ball from 62 m on, with no outfield opponent within 10 units of
  his straight line to goal, never passes: he keeps running and shoots. A defender chasing from
  behind does not count as in the way. (Before this rule he gave the ball away about four times a
  match in that situation.) Counter-Attack was toned down to keep the orders balanced: the sprint
  lasts 1.4 s and the chance bonus is 8%.
- **Moment** bar: only for players whose trait is an active one; it fills on touches and fires the trait.

Balance targets, checked in tests: a team one tier better wins about 70%, ~2 goals per match, no pitch
side favoured.

## Players and traits [in demo]

96 outfield players and 16 keepers (plus the academy keeper), cost 1-5. Cost follows general
reputation: cost 1 is a good squad player, cost 5 an all-time great. **Eight styles, each with twelve
outfield players and two keepers**, so no style is easier to complete than another. A player sits in
the style he actually stands for (Vinícius and Mbappé run in behind: Counter; Beckham and Roberto
Carlos strike dead balls: Set Pieces; Cruyff and Beckenbauer: Total Football), and the pool covers
many more countries than before, but nobody is in it only for his passport. Every player has
**one trait nobody else has**; styles and positions give the team combinations.
The full table with effects is the single source of truth: `backend/internal/game/data.go`.

| Style | Cost 1 | Cost 2 | Cost 3 | Cost 4 | Cost 5 | Keepers |
| --- | --- | --- | --- | --- | --- | --- |
| Tiki-Taka | Ferran Torres, Marc Cucurella | Martín Zubimendi, Dani Olmo | Pedri, Bukayo Saka, Martin Ødegaard | Rodri, Kevin De Bruyne, Xavi | Lionel Messi, Andrés Iniesta | André Onana (1), Ederson (3) |
| Catenaccio | Radu Drăgușin, Sofyan Amrabat | Alessandro Bastoni, N'Golo Kanté, Federico Chiesa | Rúben Dias, Claude Makélélé | Antoine Griezmann, **Cristian Chivu**, Virgil van Dijk | Paolo Maldini, Roberto Baggio | Gianluigi Buffon (5), Lev Yashin (5) |
| Gegenpress | Darwin Núñez, Wataru Endo | Dominik Szoboszlai, Luis Díaz, Roberto Firmino | Declan Rice, Florian Wirtz, Lautaro Martínez | Mohamed Salah, Ousmane Dembélé | Lothar Matthäus, Ruud Gullit | David Raya (2), Oliver Kahn (4) |
| Street Football | Dennis Man, Mohammed Kudus | Rafael Leão, Khvicha Kvaratskhelia, Riyad Mahrez | Jamal Musiala, Lamine Yamal, Eden Hazard | **Adrian Mutu**, Neymar | Diego Maradona, Ronaldinho | René Higuita (1), Gianluigi Donnarumma (4) |
| Route One | Wout Weghorst, Dan Burn | Kalidou Koulibaly, Victor Osimhen | Harry Kane, Yaya Touré | Erling Haaland, Zlatan Ibrahimović, Robert Lewandowski, Nemanja Vidić | Cristiano Ronaldo, Pelé | Jordan Pickford (1), Thibaut Courtois (4) |
| Total Football | Jurriën Timber | Bernardo Silva, David Alaba, Joško Gvardiol | Frenkie de Jong, Joshua Kimmich, Federico Valverde | Jude Bellingham, Luka Modrić, Dennis Bergkamp | Johan Cruyff, Franz Beckenbauer | Manuel Neuer (3), Edwin van der Sar (4) |
| Counter | Christian Pulisic, Timo Werner | Jamie Vardy, Achraf Hakimi, Alphonso Davies | Son Heung-min, Sadio Mané | Vinícius Júnior, Kylian Mbappé, Gareth Bale | Ronaldo Nazário, Thierry Henry | Keylor Navas (2), Alisson Becker (3) |
| Set Pieces | James Ward-Prowse, Harry Maguire | Trent Alexander-Arnold, Casemiro, Christian Eriksen | Bruno Fernandes | David Beckham, Roberto Carlos, Sergio Ramos, Andrea Pirlo | **Gheorghe Hagi**, Zinedine Zidane | Emiliano Martínez (2), Jan Oblak (4) |

Chivu and Mutu at cost 4 and Hagi at cost 5 are the author's explicit choices; Romanians are otherwise
only Drăgușin and Dennis Man at cost 1.

Stats come from a position template scaled by cost (×0.76 at cost 1 up to ×1.08 at cost 5) plus a few
personal tweaks. Market odds per stadium level (cost 1-5): L1 100/0/0/0/0, L2 80/20, L3 60/30/10,
L4 45/33/20/2, L5 30/35/28/6/1, L6 20/30/35/13/2, L7 12/25/35/22/6, L8 8/20/33/29/10,
L9 5/15/30/35/15, L10 3/10/25/37/25. Legends appear only from level 5.

**Combinations** (the left panel) come in two kinds, and a player counts for one of each. Nation
combinations were tried and rejected by the author.

- **Style, three steps (2 / 4 / 6 players).**
  - Tiki-Taka: pass accuracy +8 / +18 / +28%, Dribbling +5 / +10 / +16%, Moment +15 / +35 / +60%.
  - Catenaccio: Defending +12 / +28 / +45%, opponent chances -10 / -24 / -38%.
  - Gegenpress: pressing speed +10 / +22 / +35%, fatigue -15 / -30 / -45%.
  - Street Football: Dribbling +14 / +30 / +50%, Pace - / +6 / +14%.
  - Route One: Physical +12 / +25 / +40%, Shooting +8 / +15 / +25%, long shots +20 / +45 / +75%.
  - Total Football: every stat +2 / +4 / +6%.
  - Counter: after the ball is won, midfield and attack sprint for 0.6 / 1.1 / 1.6 s. It adds to the
    Counter-Attack order.
  - Set Pieces: the team's players are fouled 30 / 60 / 100% more often, its penalties are 5 / 10 /
    15 points more likely to score, and a free kick within 30 units of goal is struck at goal
    (base chance 7 / 11 / 16%, scaled by the taker's Shooting and his own free-kick trait). Without
    the style a team goes for goal only if it fields a free-kick specialist (base 5%).
- **Position, two steps (2 / 4):** Strike Partnership (Shooting +18 / +42%), Wide Men (Pace +8 /
  +18%), Brain Trust (Passing +15 / +36%, Moment +20 / +45%), Double Pivot (Defending +15 / +32%),
  Back Line (Defending and Physical +10 / +22%).

**Goalkeepers count for style combinations** (Pickford: Route One; Buffon: Catenaccio). There is no
keeper position bonus.

Three trait dimensions came with Set Pieces: some players are fouled more often (Hazard), some take
better penalties (Bruno Fernandes, Pirlo, Zidane) and some strike better free kicks (Beckham, Roberto
Carlos, Pirlo, Hagi, Ward-Prowse, Eriksen, Alexander-Arnold, Alaba).

Measured (1500 matches, a Set Pieces team at step 4 against the same opponent as a team with no
style): fouled 1.62 times a match instead of 0.71, 0.15 free kicks at goal a match of which one in
five scores, penalties 0.050 a match instead of 0.015.

### Balance, measured (2026-11-19)

`REPORT=1 go test -run Balance -v ./internal/game/` prints the reports
(`balance_report_test.go`); they check nothing, they are for tuning.

- **What a combination is worth**: the same eight players against themselves, one side without
  it. Styles at 2 / 4 / 6: Tiki-Taka 55 / 62 / 67%, Catenaccio 53 / 57 / 59, Gegenpress 54 / 61 /
  66, Street 54 / 57 / 57, Route One 52 / 56 / 61, Total Football 54 / 58 / 63, Counter about 53 at
  every step in a mixed team (worth more in a team of counter players), Set Pieces 52 / 55 / 58.
  Positions at 2 / 4: between 52 and 60%.
- **Pure style teams** (six of a style, lined up 2-3-2, round robin): every style averages
  between 44% (Route One) and 56% (Total Football, Counter). Before tuning the range was 36% to
  70%: Total Football gave +13% to every stat at 6, the Counter sprint lasted 2 s, and Catenaccio
  and Route One were nearly all defenders and all strikers, which is why three players were
  swapped into each for the other half of the pitch.
- **Players**: in a team of five of his own price against random teams of that price, nobody wins
  less than 43% or more than 61%. Defenders rate highest, playmakers lowest.
- **Price and stars**: five players beat five who cost one less in 59 to 69%; a star is worth
  about 80% against the same player without it.
- **Solo season** played the way the lobby bots play (always the first story answer): before
  2026-11-21, 42% sacked and 4% champions, winning 16% and 12% of fixtures 7 and 8. Softer
  opponents since: 19% sacked, 14% champions, 50% European place; win rate per fixture
  54 / 74 / 61 / 59 / 44 / 47 / 31 / 26% (the last two stay the hard finale). A person plays
  better than the bot.
- **League** (2026-11-21, bot play, 400 seasons): 10% champions, 42% Europe, 35% mid-table,
  3.5% relegated, 8% sacked; the champion ends on about 20 points of 27, the bot on 14.5.
- **Augments** (2026-11-24, `TestBalanceAugments`: 900 lobbies of eight bots, each club handed a
  different augment at round 2, average final place, 4.5 neutral): Trusted by the Board was far too
  strong (2.84, defeats cost 30% less) and is now 10% less (about 4.1); Home Crowd 4% -> 3%, Fitness
  Coach and Finishing School and Defensive Coach were raised. After tuning everything sits between
  3.8 and 4.7 (noise about ±0.12). Scouting Network and Shrewd Investor look weak only because bots
  never refresh the market or save money.
- `TestMatchInvariants` plays 4000 random matches and checks that score, goal events, shootouts,
  sendings-off and positions tell one story.

## Real names and photos

Names are those of real footballers; photos are loaded in the browser from Wikipedia's public API,
limited to freely licensed images (Wikimedia Commons), with a credit line in the footer. A player
without a free image shows initials (today all 80 have one). Portraits are cached per player in the
browser session and only missing ones are requested, so adding players never leaves gaps.

Cards are cropped onto the face using a hand-measured face position per player
(`frontend/src/photos.ts`, `FOCUS`). If Wikipedia replaces a lead image, that player's crop can drift
until his entry is re-measured.

This is fine for a private prototype. **Publishing or selling the game with real names and likenesses
needs licences** (player unions such as FIFPRO, clubs, leagues), and Commons images carry attribution
duties per image. Names and photo sources live in one table (`backend/internal/game/data.go`), so
switching to licensed or invented players is a data change.

## Architecture [in demo]

- `backend/internal/game` — pure rules, no HTTP, no database; deterministic seeded generator.
  Trait mechanics stay server-side; the client only receives names and descriptions.
- `backend/internal/store` — memory or MongoDB, optimistic versioning.
- `backend/internal/api` — session, rate limiting, validation; computes nothing about the game.
- `frontend` — React + Vite; 2D canvas for the match. No game calculations.

Security is described in README.md.

## Accounts and social [in demo]

- **Sign-up / login** with username and password (Argon2id hashes, session cookie rotated on login).
- **Home page**: solo season status (continue / new), friends, chat.
- **Friends**: invite by username or by link (`/?add=<name>`), accept / decline / remove, online dot.
- **Chat** between friends: plain text, polled every 3 seconds, kept 30 days.

## Live series [in demo]

A new season against a friend, played live. This replaced two earlier attempts the author rejected:
an offline "friendly" between solo squads, and a one-off live match between solo squads.

- Invite a friend who is online; the invite expires after 60 seconds. On accept **both managers start
  from scratch**: budget 6, stadium level 1, academy keeper, patience 100. Solo seasons are untouched.
- Every round runs on the server's clock: transfer window (50 s first round, then 40 s), 4 s countdown,
  30 s first half, 20 s break, 30 s second half, 9 s result. 1x speed only, no skipping.
- Transfer window: the usual market, bench and pitch. Whatever is on the pitch when the timer ends
  plays. Free places are filled automatically at that moment (and at kick-off in a solo season):
  while the stadium level allows more outfield players than are on the pitch, bench players come
  on in bench order, skipping keepers and suspended players; an empty bench changes nothing. A
  pitch full of suspended players gets the best eligible benched player in place of one of them;
  a manager who bought nobody plays with the keeper alone (who then clears the ball to the opponent rather than holding it).
- Scouting shows the opponent's line-up from the previous match, never the current window.
- Break: each manager picks a team talk; hidden from the opponent, changeable until the timer ends.
- After each match the loser's owner loses patience, 8 × (2 + goal difference + round/3); both get income
  and XP as in a season. **The stake is the series itself**: first owner to run out of patience loses,
  or the higher patience after 10 rounds. Forfeit hands the series to the friend. The result is posted
  in the chat. There is no rating or points table yet.
- **State lives in the database, not in the server process or the browser.** The series document holds
  the round clock, seed, frozen squads and orders; each manager's club is its own document. Phase
  changes are applied by whichever poll arrives first, guarded by version checks and idempotent per
  round. Clients poll once a second, get only frames already played (plus a 3 s buffer) and align
  to the server's clock.

## Lobbies: ranked, normal and with friends [in demo]

The auto-battler format proper: up to eight managers in one lobby, on the same shared clock as a
live series. The interface says "points", not "LP" (the author's wording). Three ways in:

- **Ranked**: moves your rating. Matching is **by level**: a manager first accepts only groups
  whose spread is within 100 points, 50 more every 10 seconds waited, anyone after 2 minutes. Of
  the groups everyone in them accepts, the tightest starts; the rest keep waiting. Once the
  longest-waiting manager has waited **2 minutes**, he starts anyway with the queued managers
  closest to his points (up to eight) and **bots take the empty seats**. Bots earn no points;
  humans are rated on their finishing place as usual.
- **Normal**: same game, nothing at stake. The queue looks for people for one minute, then bots take
  the empty seats.
- **Friends lobby**: one manager creates it; **anyone already in it** can invite his own friends;
  invited managers join or decline; only the host starts it **with whoever is in**, two to
  eight managers. No bots, no points. The host leaving closes it; an unstarted lobby expires after
  15 minutes.

A manager waits in one queue or one friends lobby at a time.

- While searching, the home page shows how many managers have been found ("found 3 of 8"), eight
  seat markers, the time until bots step in, and a rotating quote attributed to a footballer
  (`frontend/src/quotes.ts`).
- **Start**: everyone from scratch (budget 6, stadium level 1, academy keeper, patience 100, the
  scale TFT players know).
- **Penalty rounds** (replaced the warm-up friendlies, 2026-11-20): the first 2 rounds are paired
  like any other, but each meeting is a penalty shootout only: the keeper and the best shooters on
  the pitch, five kicks each, then sudden death (`SimulateShootout`; the clock has no halves, the
  shootout starts at kick-off). The winner gets +2 💰 on top of the usual income; nobody loses
  patience. From round 3 full matches. The lobby lasts at most 2 + 16 rounds.
- **Each round** the survivors are paired at random and each pair plays its own match; everybody
  watches their own match at the same moment. With an odd number the last manager plays a **ghost**:
  a copy of another survivor's squad, who is not affected by that result.
- Losing costs patience like TFT's damage: a stage base that climbs every three match rounds (2, 4,
  7, 10, 13, then 17) plus the margin, counted up to four goals, times 1 + n/5 (n = match rounds
  played, this one included). A 1-0 defeat costs 3 in the first match rounds, 9 around round 9,
  17 around 15, 21 from 18; 3-0 costs 5, 13, 25, 29. Simulated: a lobby ends around match round
  20 (p10 19, p90 22), first manager out around 13, fourth around 16. Earlier rules ended it around
  round 10 (8 × (2 + goal difference + n/3)) and 12 (3 × (2 + margin) + 2n): he found both far too
  quick. At zero the manager is out and takes the next free finishing position; the last one in
  wins. Round 26 of matches is the cap. A friends series (10 rounds) now usually runs to the cap and
  is decided on patience.
- **Streaks**: five wins in a row set a manager's name on fire (animated flame gradient and a
  "5W" badge, stronger from eight); five defeats in a row make it reel (a woozy, blurred name and a
  "5L" badge). Shown on the scoreboard, the patience bars, the countdown screen and the lobby list,
  and on your own name in solo matches. Penalty rounds count. While a shootout is still being
  watched, the streak shown is the one from before the round, so it cannot give the result away.
- **Spectating**: click a manager in the lobby list to see their board, combinations, augments,
  level and budget during the window, and their match while it is played (`/api/lobby?watch=seat`);
  a selector over the match switches between matches. A manager who is out can keep watching until
  the lobby ends ("Keep watching", or later "Watch it to the end" on the home page; it then follows
  the leader automatically). Spectators are visible: the lobby list shows 👁N next to a watched
  manager, the match lists its spectators by name and a manager sees who is watching him. Presence
  lives in the bus (Redis hash per lobby, 5 s freshness; in memory on a single instance). A lobby
  whose humans are all out is no longer closed at once: the bots play on while anyone watches, and
  the usual 3-minute abandonment closes it otherwise. The final screen names the winner.
- You see this round's opponent only once the transfer window has closed.
- **Bots** manage a real club with the same rules: they level the stadium when rich, sign pairs
  first, field their best eligible players and pick a half-time order from the score.
- **Ranked points**: everyone starts at **0** and can never go below 0. Each division has its own
  table for places 1 to 8, and the table of the division the manager started the lobby in applies.
  Low divisions pay a lot and cost little, so a manager better than his division leaves it fast
  (`tiers` in `backend/internal/api/lobby.go`, shown in the game under "Divisions & points"):

  | Division | Points | 1st | 2nd | 3rd | 4th | 5th | 6th | 7th | 8th |
  | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
  | Bronze | 0-99 | +50 | +38 | +26 | +10 | +4 | -2 | -6 | -10 |
  | Silver | 100-249 | +46 | +34 | +22 | +10 | -2 | -8 | -14 | -20 |
  | Gold | 250-449 | +42 | +30 | +18 | +10 | -6 | -14 | -22 | -30 |
  | Platinum | 450-699 | +38 | +26 | +14 | +10 | -10 | -20 | -30 | -40 |
  | Diamond | 700+ | +34 | +22 | +12 | +10 | -12 | -24 | -36 | -48 |

  Fourth place is **always +10**, in every division (the author's rule).
- **Ranking**: the home page shows your points and a top ten, switchable between **Everyone** and
  **Friends** (you and your friends). "Full ranking" has the same two tabs and lists at most 100
  rows; a manager further down still gets his own row with his real rank. Equal points share a rank.
  Other lists are bounded too: 100 friends, 60 chat messages per page.
- Forfeit places you last of those still in. A lobby with no human left, or with a single manager
  left, ends at once.
- State: one lobby document (seats, pairs, seeds, orders, round clock) plus one club document per
  seat; the queue is a document too. Nothing lives in server memory.
- `GAME_SPEED=15` on the server runs live games fifteen times faster, for testing whole lobbies.

## Accounts: email, password reset, profile [in demo]

- **Sign-up needs an email address** (unique, case-insensitive). A confirmation link is mailed;
  playing does not wait for it, the home page just keeps reminding. Accounts created before this
  have no address and can add one from the profile.
- **Mailed links** carry a 32-byte random secret in the URL *fragment* (`/#verify=…`, `/#reset=…`),
  which browsers never send to a server; the page reads it and removes it from the address bar.
  The server stores only the SHA-256, each link works once, a newer link of the same kind cancels
  the older one. Confirmation lasts 24 hours, reset 30 minutes.
- **Forgot password** always answers the same, account or not, and mails in the background, so
  neither the reply nor its timing reveals who has an account. Choosing the new password signs the
  account out everywhere and confirms the address.
- **Profile** (the manager chip on the home page): statistics, change email, change password. Both
  changes ask for the current password; changing the password signs out every other device and
  cancels pending reset links; a confirmed address gets a "this was changed" mail.
- **Limits**: sign-up, forgot, reset and confirm share the per-IP limit (10, then 10 a minute);
  at most 3 mails per address, then one every 5 minutes.
- **Sending**: `SMTP_HOST`, `SMTP_PORT` (587, STARTTLS), `SMTP_USER`, `SMTP_PASS`, `MAIL_FROM`,
  `PUBLIC_URL` (base of the links). Without `SMTP_HOST` mails are **printed in the server log**
  instead of sent: fine for a local demo, never for a public server.
- **Statistics** are counters on the user (`stats`): matches, wins, goals for/against (solo
  matches when played; live matches when the manager is placed or the series ends), solo seasons
  and titles, lobbies, lobby wins, top-4 finishes, average place, friend series and series won.
  Counting starts when the feature shipped.
- **Cookie notice**: one bar until dismissed. The game sets a single strictly necessary cookie
  (the session) and keeps the photo cache and the dismissal in local storage; nothing else.
- Known trade-off: sign-up says when an address already has an account (rate-limited).

## Live updates and running several instances [in demo]

- **WebSocket** at `/api/ws` (`backend/internal/api/ws.go`, `frontend/src/live.ts`). The client
  *watches* GET routes (`/api/ranked`, `/api/series`, `/api/lobby`, `/api/friends`, `/api/chat`,
  `/api/ranking`); the server re-runs the route's own handler and pushes the answer when it
  differs from the last one. It re-runs on a wake-up (someone did something that concerns this
  user) and on the route's interval (live games move with the clock: once a second). Nothing new
  is exposed and no game logic is duplicated: a watch returns exactly what a poll would.
- **Fallback**: while the socket is down, every watch polls the same route over HTTP, so a proxy
  without WebSocket support or a restarting server only costs latency.
- **Security of the socket**: the handshake is a GET, which browsers do not protect with the
  same-origin policy, so the handler checks `Origin` against the allow-list itself and requires
  the session cookie. Six sockets per user, eight watches per socket, 2 KB and 30 messages per ten
  seconds from the client, only whitelisted routes. The session is re-checked every 30 seconds, so
  logging out or changing the password closes sockets elsewhere.
- **Several instances** share MongoDB (all state) and, with `REDIS_URL`, Redis
  (`backend/internal/bus`): a pub/sub channel carries wake-ups between instances, and the
  account limits (per IP, per account, per mail address) are token buckets in Redis, one counter
  for all instances. If Redis is unreachable a limit falls back to the instance's own counter.
  No sticky sessions are needed. Clocks must be in sync (NTP). `TRUST_PROXY=true` reads the
  client address from the last `X-Forwarded-For` entry, for use behind a load balancer only.
- **Verified** with `backend/cmd/twopods` (2026-11-19): two instances on one database and Redis,
  eight managers split across them, four on WebSocket and four polling, one full ranked lobby.
  Same lobby for all, same final places, identical match frames for opponents on different
  instances, everyone rated exactly once, a change on one instance pushed to a socket on the
  other in about 5 ms, login attempts alternated between instances blocked by one common counter.
- Still per instance: the general flood limit (15 requests a second per IP) and the simulation,
  which is recomputed for every answer during a match (a per-seed cache is the next step).

## Addresses

`/` is the game, `/login` the sign-in page. Signed out at `/` goes to `/login`; signed in at
`/login` goes to `/`; any other path shows a 404 page.

## Team plan [in demo]

The equivalent of TFT's team planner. A manager chooses the team he is building towards: up to 7
outfield players (the biggest stadium) and one keeper. The plan is a list of player ids on the
account (`plan`), so it is the same in a solo season, a live series and every lobby.

- `GET /api/plan`, `POST /api/plan {players}`. The server validates the list and answers with the
  combinations that team would have, computed by the same code as a real line-up.
- In the transfer window a strip shows "n/m signed · in the market now · missing"; planned players
  in the market carry a "plan" mark.
- The planner gives advice from the plan and the club as it is: planned players on sale and what
  they cost together, a copy of an owned planned player on sale (merge), stadium levels still
  needed, players whose price the market does not offer at this level, and combinations one player
  short of the next tier.
- The plan never buys, sells or moves anyone.

## Items [in demo]

`game/item.go`, 2026-11-24 (his request, TFT-like kit): every third round's transfer window
(`ItemEvery`, rounds 3, 6, 9...; every mode) drops one random item into the club's kit bag
(`Run.Items`). An item goes on one player (`Unit.Item`, actions `equip` {slot, uid} and `unequip`
{uid}); equipping another swaps them, and a player who is sold or merged into another copy gives his
item back (a merged player keeps one). Outfield items multiply one of the wearer's stats in the
match (`applyItem` on the sim player): Golden Boots shooting +15%, Sprint Spikes pace +15%,
Playmaker's Compass passing +15%, Ankle Tape dribbling +15%, Steel Shin Pads defending +15%,
Weighted Vest physical +15%, Energy Drink stamina +30%, Captain's Armband every stat +6%. Grip Gloves
(+6 reflexes) go only on keepers. Bots hand items to their most valuable players on the pitch. UI:
the "Kit bag" panel under the augments; click an item then a player, or drag it onto him; a badge on
the card; "Take off" next to Sell. Opponents' items show on their cards. Solo balance barely moved
(bot play: 12% champions, Plate 12%, Shield 8%, Crown 2%); generated opponents carry no items.

## Tutorial [in demo]

`frontend/src/Tutorial.tsx`, 2026-11-24: eight short steps (market, merging, stadium, pitch and
combinations, match day and orders, money and augments, ways to play). It opens by itself on Home
for a manager who has not played a match yet (`/api/me` `newPlayer` = no `matches` stat) and has
not closed it on this browser (localStorage `autofootball-tutorial-<name>`); Skip on every step,
Esc and arrow keys work, and "📖 How to play" in the Home header opens it again.

## Profile, achievements, history, feedback [in demo]

- **Achievements** (24, `api/achievements.go`; five added on 2026-11-24 for the run after the league: European Nights = reach the quarter-finals, Conquerors of Europe = win any cup, Kings of the Continent = win the Crown, The Double = league + Crown in one season, Bounce Back = win the play-off), each with five levels (2026-11-23): Bronze the first
  time, then Silver, Gold, Platinum and Diamond as the same feat repeats (e.g. wins 1/10/50/150/500,
  league titles 1/3/10/25/50, Invincibles and Perfect Season 1/2/5/10/25, Climber = ranked points
  100/250/450/700/1000). A level is stored with its date under `<id>` (Bronze) or `<id>@<n>`;
  achievements from before levels (wins_50, champion_5, top4_10, lobby_win_5, silver/gold/
  platinum/diamond) became levels of the merged ones and keep their dates (`oldKeys`). Decided on
  the server whenever counters change, never taken back. Unseen levels are counted on Home (🏅 n)
  and marked new in the profile, which marks them seen. A solo match result and a feedback message
  name the levels they unlocked.
- **Match history** (`games` collection, one document per finished game: solo season, lobby or
  series, with its matches): profile tab, 10 per page, newest first (`GET /api/history?page=`).
- **Friend profiles**: clicking a friend's name opens his profile (overview and achievements,
  without email or history). `GET /api/profile?name=` answers only for accepted friends.
- **Achievements for the league**: Invincibles (win the league without losing a match) and Perfect
  Season (win every match).
- **Weekly question**: a week after sign-up, and a week after every answer or message, Home asks
  whether the manager has anything to say ("Nothing this week" / "Yes, I have something", which
  opens the form). Decided on the server (`askFeedback`, `feedbackasked`), so it is the same on
  every device.
- **Feedback**: a button on every screen opens a form (idea / bug / other, 10-1500 characters).
  Stored in the `feedback` collection and mailed to `FEEDBACK_TO`, or to the address of
  `MAIL_FROM` when unset. Five messages, then one per 10 minutes per manager.

## Server performance notes (2026-11-21)

- A match is simulated once per server: `api/simcache.go` keeps the last 96 results keyed by seed,
  squads, orders and halves, and concurrent requests for a match being simulated wait for that one
  run. Before, every poll of every manager and spectator re-simulated it (~0.4 ms CPU and ~350 KB
  allocated per match).
- A lobby bot shops at most once per round (`Run.BotRound`): a server that lost the race to save
  the lobby used to make the bot shop a second time on retry.
- Home's top ten is read at most every 5 s per server; mail delivery has a 30 s deadline; the
  Mongo client keeps 4 connections warm and compresses traffic (zstd/snappy).
- Not done yet (measured as worth it later): one `$in` read for a lobby's runs, a per-entry queue
  update instead of rewriting the queue document on every poll, a per-lobby lease so only one
  server advances a lobby at a phase boundary, `GOMEMLIMIT` with a container memory limit.
  Splitting one lobby's four simulations across cores is not worth it: they cost ~1.6 ms against
  ~16 sequential Mongo round trips.

## Account deletion [in demo]

Profile → Account → Delete account, confirmed with the password (`api/delete.go`). Refused while
the manager is in a live lobby, a live series or a friends lobby; a pending series invitation is
dropped and he leaves any queue.

- **At once**: the account is marked `deletedat`, every session and mailed link stops working, a
  mail gives the erase date. For 30 days it is hidden (rankings, friend lists, friend profiles,
  searches, invites) but kept whole, and the **username and email stay taken**. Signing in within
  the 30 days restores everything.
- **After 30 days** a janitor on every server (hourly, safe to repeat) erases it
  (`store.PurgeUser`): own match history, solo and series clubs, friendships and their chats,
  sessions and tokens go; his name in other managers' history becomes "Former manager #xxxxxx";
  feedback stays without user id and name; the user document becomes a stub (id, that name,
  dates). The username and email are then free.
- Outside the app: mails already sent (feedback in the contact inbox) and server logs, which rotate.

## Ranked seasons [in demo]

A season is a calendar month: season 1 is November 2026 (`api/seasons.go`). When it ends every
manager keeps half his points (rounded down), and whoever finished a ranked lobby in it gets a
badge with the division of his final points (`badges`, shown in the profile). The reset is lazy:
the first server that needs the ranking in a new month runs one `UpdateMany` that only touches
managers still in an older season, so it is safe to run twice or on two servers at once. Home
shows the season and when it ends; history rows carry their season.

## Not built yet

- Rating by opponent strength (points depend on place and own division only),
  a shared player pool, items or carousel rounds between matches, balance measurement of augments.
- Sign-in by email, two-factor sign-in, avatars.
- Cards, items, substitutions at half-time, set pieces, 3D graphics.

## Open questions

- Match length (60 s at 1× now) and season length.
- How strong the half-time order should be.
- Licensing route if the game is ever published.
