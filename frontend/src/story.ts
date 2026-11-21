// The words of the solo season's story. What happens (club events, promises
// and their effects) is decided on the server; this file only tells it.

export const INTRO = {
  title: 'The phone rings',
  text: [
    "Nobody else wanted the job. The club plays on a pitch behind a supermarket, the kit is two sizes too big, and the owner hired you after a ten-minute chat in a car park.",
    'Eight fixtures. Every club in this league is a little stronger than the last, and the owner is not a patient man. Lose too often and you are clearing your locker before the summer.',
    'Win six and they will build you a statue. Sign players, find a style, and prove him right.',
  ],
}

// One line about each opponent, fixture by fixture (the server's club names, in order).
export const RIVALS: Record<string, string> = {
  'Sunday League FC': 'Half of them came straight from a barbecue. A gentle start, if you take it seriously.',
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
    text: 'Not the title, but the town is planning its first trip abroad. The owner extends your contract and buys a bigger car.',
  },
  survived: {
    title: '😮‍💨 You lasted the season',
    text: 'Not pretty, but you were still in the dugout at the end. The owner shakes your hand and says “next year”.',
  },
  sacked: {
    title: '📦 Sacked',
    text: "The owner's patience ran out. Somewhere a man in a tracksuit is already being interviewed for your job.",
  },
}
