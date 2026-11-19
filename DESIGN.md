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

Season of 8 fixtures against server-run clubs of rising strength.

- **Market**: 5 players on offer, refreshed free each fixture; a new set costs 2.
- **Budget**: 5 per fixture + interest (1 per 10 held when the match ends, max 5 at 50, as in TFT)
  + 1 for a win + streak bonus (+1 at 2 in a row, +2 at 4; wins or defeats). The transfer window
  shows the interest currently earned, how much more to bank for the next step, and the full rule.
- **Stadium** (level 1-7): one outfield player per level, plus the keeper, and better players on the market.
  2 free XP per fixture; 4 XP cost 4. XP per level: 2, 2, 4, 6, 8, 10.
- **Opponents** field 1, 2, 3, 3, 4, 5, 6, 7 outfield players across the eight fixtures.
- **Merge**: three identical players become one with an extra ★ (stats ×1.3, then ×1.65); three ★★
  make a ★★★. The market shows the stars you already own of a player and marks a card that would
  complete a merge.
- **Owner's patience**: 20. A defeat costs 2 + goal difference + fixture/3. At zero you are sacked.
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
L4 45/33/20/2, L5 30/35/28/6/1, L6 20/30/35/13/2, L7 12/25/35/22/6. Legends appear only from level 5.

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
- **Solo season** played the way the lobby bots play: 75% are sacked, mostly at fixtures 7 and 8;
  3% win the title. It is hard at the end; untouched so far.
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

- **Ranked**: moves your rating. **Never has bots**: it starts only when eight real managers are
  waiting, however long that takes. Matching is **by level**: a manager first accepts only groups
  whose spread is within 100 points, 50 more every 10 seconds waited, anyone after 2 minutes. Of
  the groups everyone in them accepts, the tightest starts; the rest keep waiting.
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
- **Warm-up**: in the first 2 rounds nobody meets another manager. Everyone plays the same small
  club (one, then two cheap players, as in the first fixtures of a solo season). Budget, experience,
  streaks and suspensions count as usual; a defeat costs no patience. From round 3 managers are
  paired with each other. The lobby lasts at most 2 + 14 rounds.
- **Each round** the survivors are paired at random and each pair plays its own match; everybody
  watches their own match at the same moment. With an odd number the last manager plays a **ghost**:
  a copy of another survivor's squad, who is not affected by that result.
- Losing costs patience: 8 × (2 + goal difference + n/3), where n counts competitive rounds only. At zero the manager is out and takes the
  next free finishing position; the last one in wins. Round 14 is the cap.
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

## Not built yet

- Rating by opponent strength (points depend on place and own division only), seasons and resets,
  spectating eliminated lobbies, a shared player pool, items or carousel rounds between matches.
- Sign-in by email, two-factor sign-in, account deletion, avatars.
- Cards, items, substitutions at half-time, set pieces, 3D graphics.

## Open questions

- Match length (60 s at 1× now) and season length.
- How strong the half-time order should be.
- Licensing route if the game is ever published.
