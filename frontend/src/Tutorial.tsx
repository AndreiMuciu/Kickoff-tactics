import { useEffect, useState } from 'react'

// How to play, in eight short steps. Shown once to a manager who has not
// played a match yet (see Home), skippable at any step, and always available
// again from the "How to play" button.

const STEPS: { icon: string; title: string; text: string[] }[] = [
  {
    icon: '⚽',
    title: 'Welcome, manager',
    text: [
      'Kickoff Tactics is a football auto-battler. You build the club: sign players, decide where they stand and what you tell them at half-time. The matches play themselves, and the better-built club wins.',
      'This takes about a minute. You can skip it at any time and open it again from “How to play”.',
    ],
  },
  {
    icon: '🛒',
    title: 'The transfer market',
    text: [
      'Every transfer window offers five players. Their price, 1 to 5 💰, follows their reputation: the legends cost 5 and turn up rarely, and more often as your stadium grows.',
      'Refresh the offers for 2 💰, or 🔒 lock them to keep the same five for the next window. To sell a player, select him and press Sell, or drag him onto the market.',
    ],
  },
  {
    icon: '⭐',
    title: 'Three of a kind',
    text: [
      'Sign the same player three times and the copies merge into one ★★ player, much stronger. Three ★★ copies make a ★★★.',
      'A card in the market with a different background is a player you already own: one more step towards the merge.',
    ],
  },
  {
    icon: '🏟',
    title: 'The stadium',
    text: [
      'Your stadium level is the number of outfield players allowed on the pitch. The goalkeeper always plays on top of that, and you start with an academy keeper in goal.',
      'Level 1 fits one outfield player; level 10 is a full eleven. Buy stadium experience for 4 💰. Everyone else waits on the bench, and at the deadline free places are filled from the bench for you.',
    ],
  },
  {
    icon: '🧩',
    title: 'The pitch and the combinations',
    text: [
      'Drag players onto the three lines: the defence holds the back, the midfield links the play, the attack stays high. Every player also has his own trait, shown on his card.',
      'Players of the same style (Tiki-Taka, Catenaccio, Gegenpress, Counter and more) unlock a combination bonus at 2, 4 and 6 on the pitch, listed on the left. Building around one or two styles is how clubs get strong.',
    ],
  },
  {
    icon: '⏱',
    title: 'Match day',
    text: [
      'Press Kick off and watch. At half-time you pick the order for the second half: All-Out Attack when you are behind, Park the Bus when you are ahead.',
      'Some orders answer the other side: Long Balls against a team that pushes up, Shoot on Sight against one that sits deep. Red cards suspend a player for the next matches.',
    ],
  },
  {
    icon: '💰',
    title: 'Money',
    text: [
      'After every match you receive 5 💰, plus 1 for a win and a little more for a streak, good or bad. Money in the bank earns interest: +1 for every 10 💰 kept, up to +5. Saving pays.',
      'In rounds 2, 5 and 8 you also choose an augment: a boost for the rest of the game, from extra money to a crest that counts as one more player of a style.',
    ],
  },
  {
    icon: '🏆',
    title: 'Ways to play',
    text: [
      'Solo season: a league of ten clubs over nine matchdays. Keep the owner patient, finish in the top four and the club plays on in Europe.',
      'Lobby of eight: seven other managers, the last club standing wins. Ranked lobbies give points and divisions; normal ones are for practice. Live series: you and a friend, both from scratch, meeting every round.',
    ],
  },
]

export default function Tutorial({ onClose }: { onClose: () => void }) {
  const [i, setI] = useState(0)
  const last = i === STEPS.length - 1
  const step = STEPS[i]

  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      if (e.key === 'ArrowRight') setI((n) => Math.min(n + 1, STEPS.length - 1))
      if (e.key === 'ArrowLeft') setI((n) => Math.max(n - 1, 0))
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [onClose])

  return (
    <div className="overlay story-overlay tutorial-overlay">
      <div className="dialog story tutorial" role="dialog" aria-modal="true" aria-labelledby="tutorial-title">
        <div className="tutorial-head">
          <small className="story-kicker">
            How to play · {i + 1} of {STEPS.length}
          </small>
          {!last && (
            <button className="link" onClick={onClose}>
              Skip tutorial
            </button>
          )}
        </div>
        <div className="tutorial-icon" aria-hidden="true">
          {step.icon}
        </div>
        <h2 id="tutorial-title">{step.title}</h2>
        {step.text.map((t, n) => (
          <p key={n}>{t}</p>
        ))}
        <div className="tutorial-dots" role="tablist" aria-label="Steps">
          {STEPS.map((s, n) => (
            <button key={n} role="tab" aria-selected={n === i} aria-label={s.title} className={n === i ? 'on' : ''} onClick={() => setI(n)} />
          ))}
        </div>
        <div className="row center-row">
          <button disabled={i === 0} onClick={() => setI(i - 1)}>
            ← Back
          </button>
          {last ? (
            <button className="primary" onClick={onClose}>
              Let’s play ▶
            </button>
          ) : (
            <button className="primary" onClick={() => setI(i + 1)}>
              Next →
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
