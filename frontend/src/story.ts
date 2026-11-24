// The words of the solo season's story. What happens (club events, promises
// and their effects) is decided on the server; this file only tells it.

export const INTRO = {
  title: 'The phone rings',
  text: [
    "Nobody else wanted the job. The club plays on a pitch behind a supermarket, the kit is two sizes too big, and the owner hired you after a ten-minute chat in a car park.",
    'Ten clubs, nine matchdays, everyone plays everyone once. You meet the weakest first and every opponent after that is a little stronger. Three points for a win, one for a draw, and the owner is not a patient man: lose too often and you are clearing your locker before the summer.',
    'Finish top and they will build you a statue. The top four go to Europe; the bottom two go down. Sign players, find a style, and prove him right.',
  ],
}

// One line about each opponent, fixture by fixture (the server's club names, in order).
export const RIVALS: Record<string, string> = {
  'Sunday League FC': 'Half of them came straight from a barbecue. A gentle start, if you take it seriously.',
  'Bus Stop Wanderers': 'Nobody knows where they train. They turn up late, in two cars, and still expect to win.',
  'Car Park Rovers': 'They train between parked cars and hate losing on their own tarmac.',
  'Garage Athletic': 'Scrappy and loud. They have heard you are the new favourite and want to spoil the story.',
  'Kiosk United': 'Owned by a newspaper seller. Their striker reads every word written about him.',
  'Stairwell Sporting': 'The halfway point. They have won three in a row and smell a fourth.',
  'Roundabout Olympic': 'They pass you round in circles until you are dizzy. The owner is watching closely now.',
  'Valley Galacticos': 'Big names, big budget, bigger egos. This is where seasons are decided.',
  'Imperial XI': 'The champions-elect. Beat them and this town will never forget your name.',
}

export const PLEDGES: Record<string, string> = {
  win: '🤝 You promised a win',
  big_win: '📣 You promised the fans a big win',
  clean: '🧤 Owner bets on a clean sheet',
}

// What the owner says after a match.
export function reaction(won: boolean, gf: number, ga: number, round: number): string {
  const margin = Math.abs(gf - ga)
  if (gf === ga) return round >= 7 ? '“A point. Is a point enough, coach?” The owner is doing sums.' : '“A point is a point.” The owner shrugs.'
  if (won && margin >= 3) return '“That is what I pay you for.” The owner is buying drinks for the whole stand.'
  if (won && round >= 7) return '“Now we are talking.” The owner has started practising his trophy speech.'
  if (won) return '“Good. Do it again.” The owner almost smiles.'
  if (margin >= 3) return '“Embarrassing.” The owner leaves before the final whistle.'
  if (round >= 6) return '“We are running out of time, coach.” The owner’s patience is wearing thin.'
  return '“Unlucky. I don’t like unlucky.” The owner checks his phone.'
}

export const EPILOGUE: Record<string, { title: string; text: string }> = {
  champion: {
    title: '🏆 Champions!',
    text: 'From the car park to the top of the league. The owner is building you a statue outside the stadium, slightly taller than his own.',
  },
  europe: {
    title: '🎟 European football',
    text: 'A top-four finish: the town is planning its first trip abroad. The owner extends your contract and buys a bigger car.',
  },
  survived: {
    title: '😮‍💨 Mid-table',
    text: 'No trophy, no drama: safe in the middle of the table. The owner shakes your hand and says “next year”.',
  },
  relegated: {
    title: '⬇️ Relegated',
    text: 'You lasted the whole season, but the club goes down. The owner is already phoning the league to ask about appeals.',
  },
  sacked: {
    title: '📦 Sacked',
    text: "The owner's patience ran out. Somewhere a man in a tracksuit is already being interviewed for your job.",
  },
}

// ---- Europe ----
// A top-four finish carries the club, squad and budget into one of three
// European cups (decided on the server, see cup.go).

export const CUPS: Record<string, { name: string; stars: number; intro: string }> = {
  crown: {
    name: 'European Crown',
    stars: 3,
    intro: 'Champions get the big stage. The draw, in a hotel ballroom full of chandeliers, puts you among the giants of the continent: clubs with their own museums and stars on every shirt.',
  },
  shield: {
    name: 'European Shield',
    stars: 2,
    intro: 'Second or third is good enough for the Shield. Old European names with long histories and loud away ends are waiting, and none of them has heard of you yet.',
  },
  plate: {
    name: 'European Plate',
    stars: 1,
    intro: 'Fourth place, and a ticket to the Plate: small clubs from every corner of the continent, long bus rides and away fans who sing in languages you do not speak.',
  },
}

export const CUP_RULES =
  'Same squad, same budget. A group of four, three matchdays; the top two go through to a semi-final and a final, settled on the night, with penalties if level. The owner’s patience does not matter here: lose a knockout tie and the adventure is over.'

export const stars = (n: number) => '★'.repeat(n)

// The stage of European match m (1..5).
export const cupStage = (m: number) => (m <= 3 ? `Group, matchday ${m} of 3` : m === 4 ? 'Semi-final' : 'Final')

export function cupEnding(id: string, out: string): { title: string; text: string } {
  const name = CUPS[id]?.name ?? 'Europe'
  switch (out) {
    case 'winner':
      return { title: `🏆 ${name} winners!`, text: 'A European trophy for a club that trained behind a supermarket. The owner has ordered a second statue, bigger than the first.' }
    case 'final':
      return { title: `🥈 ${name} runners-up`, text: 'One match from glory. The town threw a parade anyway, and the owner cried through all of it.' }
    case 'semi':
      return { title: `${name}: semi-finalists`, text: 'Among the last four on the continent. Nobody laughs at the kit any more.' }
  }
  return { title: `${name}: out in the group`, text: 'Europe was a step too far this time, but the club has a passport now, and a taste for away trips.' }
}

// One line about each European club, in the order you meet them.
Object.assign(RIVALS, {
  'Borussia Elmhorst': 'A wall of yellow in the stands and a midfield that never stops running.',
  'Sporting Valmarés': 'Every one of their academy graduates seems to end up famous. This year’s batch is on the pitch tonight.',
  'Inter Valdoria': 'Three-time winners of the Crown, and they never let you forget it.',
  'Athletic Montemayor': 'They defend like their lives depend on it and score from every corner.',
  'Real Castellmar': 'The most decorated club on the continent. Their bench costs more than your town.',
  'Union Saint-Aubert': 'A cosy ground, a brass band in the stands and a coach who never sits down.',
  'Dinamo Kraljevac': 'Fierce at home, where the stands are close enough to touch the corner flag.',
  'SV Rheinau': 'Organised, patient and always in shape. They will not beat themselves.',
  'Atlético Montoro': 'They love a scrap. Expect fouls, time-wasting and a late winner.',
  'Olympique Varenne': 'Silky passers from the south, and the favourites to lift the Shield.',
  'FK Dravica': 'Part-timers from a river town. Their goalkeeper is also the club’s accountant.',
  'KS Wisłanka': 'Their supporters travelled two days by train to be here.',
  'Sporting Alvorada': 'Young, quick and fearless, with a striker the big clubs are already watching.',
  'Rapid Lindenau': 'A famous name that fell on hard times and wants the old days back.',
  'Hellas Kalamos': 'Flares, drums and a stadium on a hill above the sea. The final is in their backyard.',
})
